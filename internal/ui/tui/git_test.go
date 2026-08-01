package tui

// Runtime tests for the git display and the cache behind it. Nothing here
// spawns git: the App is wired with fakeGitReader, so every state below is a
// literal the test chose.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

// gitModel is a Model on the target list of "alpha", with the git cache in
// whatever state the caller passes. A nil entry pointer leaves the cache
// empty, which is the never-asked state.
func gitModel(entry *gitEntry) Model {
	m := testModel()
	if entry != nil {
		m = m.setGitEntry("alpha", *entry)
	}
	return m
}

func okEntry(state domain.RepoState) *gitEntry {
	return &gitEntry{status: gitOK, state: state}
}

// TestGitSegmentRendersEveryState is the AC's core display check: unknown,
// absent and clean must be distinguishable from each other, and the absence of
// a marker must never read as clean.
func TestGitSegmentRendersEveryState(t *testing.T) {
	tests := []struct {
		name  string
		entry *gitEntry
		want  string
	}{
		{name: "never asked", entry: nil, want: "…"},
		{name: "loading", entry: &gitEntry{status: gitLoading}, want: "…"},
		{name: "absent", entry: &gitEntry{status: gitAbsent}, want: "no repo"},
		{name: "unknown", entry: &gitEntry{status: gitUnknown}, want: "git unknown"},
		{
			name:  "no commits yet",
			entry: okEntry(domain.RepoState{Head: domain.HeadUnborn}),
			want:  "no commits",
		},
		{
			name:  "on a branch, clean",
			entry: okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}),
			want:  "main ✓ clean",
		},
		{
			name:  "on a branch, dirty",
			entry: okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main", Dirty: true}),
			want:  "main ● dirty",
		},
		{
			name:  "detached, clean",
			entry: okEntry(domain.RepoState{Head: domain.HeadDetached}),
			want:  "detached ✓ clean",
		},
		{
			name:  "detached, dirty",
			entry: okEntry(domain.RepoState{Head: domain.HeadDetached, Dirty: true}),
			want:  "detached ● dirty",
		},
		{
			// Not a state the adapter produces — classifyHead never reports a
			// present head as unknown. Pinned anyway, because the one thing
			// this must never degrade to is "clean".
			name:  "a gitOK entry with an unfilled RepoState",
			entry: okEntry(domain.RepoState{}),
			want:  "git unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ansi.Strip(gitModel(tt.entry).gitSegment("alpha"))

			if strings.TrimSpace(got) != tt.want {
				t.Errorf("gitSegment = %q, want %q", strings.TrimSpace(got), tt.want)
			}
			// Every state renders something. A blank segment is the failure
			// mode the AC names: it reads as "nothing to report", which for a
			// dirty or unreadable repository is a lie.
			if strings.TrimSpace(got) == "" {
				t.Error("gitSegment rendered blank")
			}
			if strings.Contains(got, "HEAD") {
				t.Errorf("gitSegment leaked the literal HEAD: %q", got)
			}
		})
	}
}

// TestGitSegmentDistinguishesUnknownAbsentAndClean checks the AC's
// must-distinguish trio differ in wording.
func TestGitSegmentDistinguishesUnknownAbsentAndClean(t *testing.T) {
	rendered := map[string]string{
		"unknown": ansi.Strip(gitModel(&gitEntry{status: gitUnknown}).gitSegment("alpha")),
		"absent":  ansi.Strip(gitModel(&gitEntry{status: gitAbsent}).gitSegment("alpha")),
		"clean": ansi.Strip(gitModel(
			okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"})).gitSegment("alpha")),
	}

	for aName, a := range rendered {
		for bName, b := range rendered {
			if aName < bName && a == b {
				t.Errorf("%s and %s render the same text %q", aName, bName, a)
			}
		}
	}
}

// TestGitStylesAreThreeDifferentColours is the other half of "distinguishable":
// three different words in one colour would pass the test above and still fail
// the user at a glance.
//
// It asserts on the style declarations rather than on rendered output, and the
// reason is a real limitation rather than a preference: lipgloss detects no
// TTY under `go test` and strips every escape, so a rendered segment carries no
// colour to compare. What this cannot catch is gitSegment routing a state to
// the wrong style — that routing is covered state-by-state by the text
// assertions in TestGitSegmentRendersEveryState, and by the visual step of the
// PR's verification handoff.
func TestGitStylesAreThreeDifferentColours(t *testing.T) {
	colours := map[string]string{
		"unknown": fmt.Sprint(styles.GitUnknown.GetForeground()),
		"absent":  fmt.Sprint(styles.GitPending.GetForeground()),
		"clean":   fmt.Sprint(styles.GitClean.GetForeground()),
		"dirty":   fmt.Sprint(styles.GitAttention.GetForeground()),
	}

	for aName, a := range colours {
		for bName, b := range colours {
			if aName < bName && a == b {
				t.Errorf("%s and %s share the foreground colour %s", aName, bName, a)
			}
		}
	}

	// The git segment must not reuse the header title's colour: they sit
	// adjacent in the top bar and would visually merge.
	title := fmt.Sprint(styles.Header.GetForeground())
	for name, c := range colours {
		if c == title {
			t.Errorf("the %s segment reuses the header title's colour %s", name, c)
		}
	}
}

// TestProjectListShowsNoGitState pins the lazy-read decision at the surface
// where it is visible. The project list issues no reads, so it must show no
// git state — including for a project whose state happens to be cached from an
// earlier drill-in.
func TestProjectListShowsNoGitState(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main", Dirty: true}))
	m.view = viewProjects

	header := ansi.Strip(strings.SplitN(m.renderProjectList(), "\n", 2)[0])

	for _, leaked := range []string{"main", "dirty", "clean", "no repo", "git unknown", "…"} {
		if strings.Contains(header, leaked) {
			t.Errorf("the project header leaked git state %q: %q", leaked, header)
		}
	}
	if !strings.Contains(header, "mkx › Projects") {
		t.Errorf("the project header lost its title: %q", header)
	}
}

// TestTargetHeaderShowsGitState is the positive half: the same state that must
// stay out of the project list must appear in the target view.
func TestTargetHeaderShowsGitState(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main", Dirty: true}))

	header := ansi.Strip(strings.SplitN(m.renderTargetList(), "\n", 2)[0])

	if !strings.Contains(header, "main ● dirty") {
		t.Errorf("the target header is missing the git segment: %q", header)
	}
	if !strings.Contains(header, "mkx › alpha") {
		t.Errorf("the target header lost its title: %q", header)
	}
	if !strings.Contains(header, "2/2") {
		t.Errorf("the git segment displaced the position counter: %q", header)
	}
}

// TestDrillInSeedsLoadingAndIssuesTheRead covers the read timing: Enter is the
// only thing that starts a read, the entry is seeded synchronously so the
// first render carries a marker, and the read itself is a command rather than
// inline work.
func TestDrillInSeedsLoadingAndIssuesTheRead(t *testing.T) {
	m := testModel()
	m.view = viewProjects
	m.projectCursor = 0

	after, cmd := projectKeymap().dispatch("enter")(m, "enter")

	if after.view != viewTargets {
		t.Fatal("Enter did not open the target view")
	}
	entry, ok := after.gitCache["alpha"]
	if !ok {
		t.Fatal("Enter seeded no cache entry; the first render would have nothing to show")
	}
	if entry.status != gitLoading {
		t.Errorf("seeded entry status = %v, want gitLoading", entry.status)
	}
	if cmd == nil {
		t.Fatal("Enter issued no read command")
	}

	// The synchronous seed is what makes the very first render non-blank.
	if seg := strings.TrimSpace(ansi.Strip(after.gitSegment("alpha"))); seg != "…" {
		t.Errorf("first render's segment = %q, want the in-flight marker", seg)
	}
}

// TestDrillInIsFreeOnACacheHit checks re-entering a project issues no second
// read while the cache is still valid.
func TestDrillInIsFreeOnACacheHit(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}))
	m.view = viewProjects

	_, cmd := projectKeymap().dispatch("enter")(m, "enter")
	if cmd != nil {
		t.Error("re-entering a cached project issued a read")
	}
}

// TestProjectListIssuesNoReads is the negative half of the read-timing
// decision, asserted against the reader itself rather than against the render:
// navigating the project list must spawn nothing.
func TestProjectListIssuesNoReads(t *testing.T) {
	calls := 0
	m := testModel()
	m.app = app.New(fakeScanner{}, fakeRunner{}, fakeGitReader{calls: &calls})
	m.view = viewProjects

	for _, key := range []string{"down", "up", "j", "k"} {
		if h := projectKeymap().dispatch(key); h != nil {
			m, _ = h(m, key)
		}
	}

	if calls != 0 {
		t.Errorf("navigating the project list issued %d git reads, want 0", calls)
	}
}

// TestReadRepoStateClassifiesOutcomes checks the port's three answers map onto
// the three cache statuses, and that neither failure mode carries a
// half-filled RepoState into the cache.
func TestReadRepoStateClassifiesOutcomes(t *testing.T) {
	tests := []struct {
		name       string
		state      domain.RepoState
		err        error
		wantStatus gitStatus
	}{
		{
			name:       "success",
			state:      domain.RepoState{Head: domain.HeadOnBranch, Branch: "main", Dirty: true},
			wantStatus: gitOK,
		},
		{
			name:       "not a repository is an absence, not a failure",
			err:        app.ErrNotARepository,
			wantStatus: gitAbsent,
		},
		{
			name:       "a wrapped sentinel is still an absence",
			err:        errWrapping(app.ErrNotARepository),
			wantStatus: gitAbsent,
		},
		{
			name:       "timeout is unknown",
			err:        context.DeadlineExceeded,
			wantStatus: gitUnknown,
		},
		{
			// The specific hazard: a failed read that still handed back a
			// zero RepoState must not be cached as one, or Dirty:false would
			// be indistinguishable from a clean tree.
			name:       "a failed read carrying a zero state is unknown, not clean",
			err:        errors.New("git status: exit 128"),
			wantStatus: gitUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := app.New(fakeScanner{}, fakeRunner{}, fakeGitReader{state: tt.state, err: tt.err})
			proj := domain.Project{Name: "alpha", Path: "/w/alpha"}

			msg, ok := readRepoState(context.Background(), a, proj, 7)().(gitStateMsg)
			if !ok {
				t.Fatal("readRepoState did not produce a gitStateMsg")
			}

			if msg.status != tt.wantStatus {
				t.Errorf("status = %v, want %v", msg.status, tt.wantStatus)
			}
			if msg.project != "alpha" {
				t.Errorf("project = %q, want alpha", msg.project)
			}
			if msg.gen != 7 {
				t.Errorf("gen = %d, want the generation the read was issued in", msg.gen)
			}
			zero := msg.state.Head == domain.HeadUnknown &&
				msg.state.Branch == "" && !msg.state.Dirty && len(msg.state.Branches) == 0
			if tt.wantStatus != gitOK && !zero {
				t.Errorf("a %v result carried a RepoState: %+v", tt.wantStatus, msg.state)
			}
		})
	}
}

func errWrapping(err error) error { return &wrapped{err} }

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "reading git state: " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }

// TestStaleReadIsDiscarded covers the race the generation counter exists for: a
// read issued before a handover landing after it would otherwise repopulate the
// cache with pre-handover data — ADR-M003's exact failure mode, reintroduced
// inside the invalidation machinery.
func TestStaleReadIsDiscarded(t *testing.T) {
	m := testModel()
	m.gitGen = 3

	stale := gitStateMsg{
		project: "alpha",
		gen:     2, // issued before the last invalidation
		status:  gitOK,
		state:   domain.RepoState{Head: domain.HeadOnBranch, Branch: "stale-branch"},
	}

	if _, ok := m.applyGitState(stale).gitCache["alpha"]; ok {
		t.Error("a superseded read was written to the cache")
	}

	// The current generation still lands, so the guard is discarding by age
	// rather than discarding everything.
	fresh := stale
	fresh.gen = 3
	entry, ok := m.applyGitState(fresh).gitCache["alpha"]
	if !ok {
		t.Fatal("a current-generation read was discarded")
	}
	if entry.state.Branch != "stale-branch" {
		t.Errorf("cached branch = %q, want the read's answer", entry.state.Branch)
	}
}

// TestHandoverInvalidatesAndReReads proves the effect rather than the
// structure: the wrapper clears the cache, bumps the generation, re-issues a
// read for the open view, and still dispatches its inner message.
func TestHandoverInvalidatesAndReReads(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}))
	genBefore := m.gitGen

	updated, cmd := m.Update(handoverDone{inner: execFinishedMsg{exitCode: 0}})
	after := updated.(Model)

	if after.gitGen == genBefore {
		t.Error("the handover did not bump the generation; a read in flight could repopulate stale state")
	}
	// The pre-handover entry is gone. It is replaced by a freshly seeded
	// loading entry for the open view, never by the old one.
	if entry, ok := after.gitCache["alpha"]; ok && entry.status != gitLoading {
		t.Errorf("the pre-handover entry survived: %+v", entry)
	}
	if cmd == nil {
		t.Error("the handover issued no re-read for the open target view")
	}
	// The inner message still reached its own case.
	if after.lastRun == nil {
		t.Error("the inner execFinishedMsg was swallowed by the unwrap")
	}
}

// TestHandoverWithNoInnerMessageStillInvalidates covers the README viewer,
// which has nothing to report but has still given up the terminal.
func TestHandoverWithNoInnerMessageStillInvalidates(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}))
	genBefore := m.gitGen

	updated, cmd := m.Update(handoverDone{})
	after := updated.(Model)

	if after.gitGen == genBefore {
		t.Error("a handover with no inner message did not invalidate")
	}
	if cmd == nil {
		t.Error("no re-read was issued after the README handover")
	}
}

// TestAnUnwrappedMessageDoesNotInvalidate is the direct demonstration of what
// a bypass would cost, and the runtime counterpart to the AST guard: the same
// message unwrapped leaves the stale state on screen.
func TestAnUnwrappedMessageDoesNotInvalidate(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}))
	genBefore := m.gitGen

	updated, _ := m.Update(execFinishedMsg{exitCode: 0})
	after := updated.(Model)

	if after.gitGen != genBefore {
		t.Error("an unwrapped message invalidated; the guard's premise no longer holds")
	}
	if entry := after.gitCache["alpha"]; entry.state.Branch != "main" {
		t.Error("an unwrapped message cleared the cache")
	}
	if seg := ansi.Strip(after.gitSegment("alpha")); !strings.Contains(seg, "main") {
		t.Errorf("segment after an unwrapped message = %q; this is the stale display "+
			"handoverDone exists to prevent", seg)
	}
}

// TestHandoverOnTheProjectListIssuesNoRead checks the re-read follows the same
// rule as the initial one: nothing is read while the project list is up, even
// though the invalidation still happens.
func TestHandoverOnTheProjectListIssuesNoRead(t *testing.T) {
	m := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}))
	m.view = viewProjects
	genBefore := m.gitGen

	updated, cmd := m.Update(handoverDone{inner: gitPullFinishedMsg{projectIndex: 0}})
	after := updated.(Model)

	if after.gitGen == genBefore {
		t.Error("a handover from the project list did not invalidate")
	}
	if len(after.gitCache) != 0 {
		t.Errorf("the cache was not cleared: %+v", after.gitCache)
	}
	// gitPullFinishedMsg returns tea.EnterAltScreen, so cmd is non-nil; what
	// matters is that no read was seeded.
	_ = cmd
}

// TestSetGitEntryDoesNotMutateTheReceiver guards the one field that would
// quietly break Model's value semantics. A map mutated in place writes through
// every copy that ever shared it, including the pre-handover model a test is
// holding to compare against.
func TestSetGitEntryDoesNotMutateTheReceiver(t *testing.T) {
	before := gitModel(okEntry(domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}))

	after := before.setGitEntry("alpha", gitEntry{status: gitUnknown})

	if before.gitCache["alpha"].status != gitOK {
		t.Error("setGitEntry mutated the receiver's cache")
	}
	if after.gitCache["alpha"].status != gitUnknown {
		t.Error("setGitEntry did not apply to the returned Model")
	}
}

// TestGitReadIsACommandNotInlineWork checks the read never runs on the update
// path. A read done inline would block the event loop for its whole timeout on
// a hung repository, which is exactly what the AC forbids.
func TestGitReadIsACommandNotInlineWork(t *testing.T) {
	calls := 0
	m := testModel()
	m.app = app.New(fakeScanner{}, fakeRunner{}, fakeGitReader{calls: &calls})
	m.view = viewProjects

	_, cmd := projectKeymap().dispatch("enter")(m, "enter")

	if calls != 0 {
		t.Errorf("the read ran during Update (%d calls); it must run in the tea.Cmd", calls)
	}
	if cmd == nil {
		t.Fatal("no command was returned")
	}

	// It does run when the runtime executes the command.
	var msg tea.Msg = cmd()
	if _, ok := msg.(gitStateMsg); !ok {
		t.Fatalf("the command produced %T, want gitStateMsg", msg)
	}
	if calls != 1 {
		t.Errorf("the command issued %d reads, want 1", calls)
	}
}
