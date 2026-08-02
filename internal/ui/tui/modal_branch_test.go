package tui

// The branch picker's tests. Nothing here spawns git or checks anything out:
// the App is wired with fakeGitReader, the picker is driven through its real
// keymap, and the return from the handover is simulated by feeding Update the
// same handoverDone wrapper tea.Exec would have produced.

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// pickerModel is a Model on alpha's target list with the given git state
// cached, ready for `b`.
func pickerModel(state domain.RepoState) Model {
	return testModel().setGitEntry("alpha", gitEntry{status: gitOK, state: state})
}

// openPicker presses `b` and asserts the picker — not a notice — opened.
func openPicker(t *testing.T, m Model) Model {
	t.Helper()

	opened, cmd := targetKeymap().dispatch("b")(m, "b")
	if cmd != nil {
		t.Error("`b` issued a command; opening the picker reads nothing")
	}
	if !opened.modal.active {
		t.Fatal("`b` opened no modal")
	}
	if opened.modal.content != modalBranchPicker {
		t.Fatalf("`b` opened content %v, want the branch picker (body: %q)",
			opened.modal.content, opened.renderModalBody())
	}
	return opened
}

// press sends a key to the open modal's own keymap.
func press(t *testing.T, m Model, key string) (Model, tea.Cmd) {
	t.Helper()

	h := m.modal.keys.dispatch(key)
	if h == nil {
		t.Fatalf("the modal does not bind %q", key)
	}
	return h(m, key)
}

// runCmd executes cmd and everything a tea.Batch fans it out into, returning
// the messages produced. A tea.Exec command yields its internal message
// without running any subprocess, so this is safe on the checkout path.
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// returnFromHandover drives the return path a real tea.Exec would take: Update
// unwraps the handover, invalidates, re-issues the git read, and this folds
// that read's answer back in. What the model shows afterwards therefore comes
// from the fakeGitReader — that is, from a read — and from nothing else.
func returnFromHandover(t *testing.T, m Model, inner tea.Msg) Model {
	t.Helper()

	updated, cmd := m.Update(handoverDone{inner: inner})
	after := updated.(Model)
	if cmd == nil {
		t.Fatal("the handover return issued no command; the re-read is missing")
	}

	reads := 0
	for _, msg := range runCmd(cmd) {
		if state, ok := msg.(gitStateMsg); ok {
			after = after.applyGitState(state)
			reads++
		}
	}
	if reads != 1 {
		t.Fatalf("the handover return produced %d git reads, want exactly 1", reads)
	}
	return after
}

func header(m Model) string {
	return ansi.Strip(strings.SplitN(m.renderTargetList(), "\n", 2)[0])
}

// ------------------------------------------------------- the option list

// TestBranchPickerMarksTheCurrentBranch covers the marker rule and the two
// ways it can go wrong: marking more than one row, and marking a row whose
// name merely resembles the current branch.
func TestBranchPickerMarksTheCurrentBranch(t *testing.T) {
	tests := []struct {
		name    string
		options []string
		current string
		want    string // the row that must carry the marker, "" for none
	}{
		{
			name:    "on a branch",
			options: []string{"feat/api", "main", "release/2.1"},
			current: "main",
			want:    "main",
		},
		{
			// A detached HEAD has an empty Branch, so nothing matches and
			// nothing is marked. No special case needed for it anywhere.
			name:    "detached",
			options: []string{"feat/api", "main"},
			current: "",
		},
		{
			// String equality, not prefix: main, main-2 and mainline are three
			// different branches and only one of them is checked out. The
			// current branch is deliberately the *shortest* here — a prefix
			// test marks all three, an equality test marks one.
			name:    "the current branch is a prefix of two others",
			options: []string{"main", "main-2", "mainline"},
			current: "main",
			want:    "main",
		},
		{
			// And not containment either, in the other direction.
			name:    "the current branch is contained in another",
			options: []string{"feature/main", "main"},
			current: "main",
			want:    "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := renderBranchPickerBody(tt.options, tt.current, 0, branchPickerRows)

			marked := []string{}
			for _, row := range strings.Split(body, "\n") {
				if strings.Contains(row, "(current)") {
					marked = append(marked, strings.TrimSpace(row))
				}
			}

			if tt.want == "" {
				if len(marked) != 0 {
					t.Errorf("nothing is checked out, but %v is marked current", marked)
				}
				return
			}
			if len(marked) != 1 {
				t.Fatalf("%d rows marked current, want exactly 1: %v\n%s", len(marked), marked, body)
			}
			if !strings.HasPrefix(marked[0], tt.want+" ") && !strings.HasPrefix(marked[0], "▸ "+tt.want+" ") {
				t.Errorf("the marker is on the wrong row: %q, want %q", marked[0], tt.want)
			}
		})
	}
}

// TestBranchPickerWindowsALongList is the @UI Patterns select-picker
// exception, tested: a fixed number of rows whatever the repository holds, the
// cursor always inside the window, and the hidden count stated rather than
// implied.
func TestBranchPickerWindowsALongList(t *testing.T) {
	options := make([]string, 40)
	for i := range options {
		options[i] = fmt.Sprintf("branch-%02d", i)
	}

	for _, cursor := range []int{0, 1, 6, 7, 20, 38, 39} {
		t.Run(fmt.Sprintf("cursor %d", cursor), func(t *testing.T) {
			rows := strings.Split(
				renderBranchPickerBody(options, "branch-00", cursor, branchPickerRows), "\n")

			// The box does not grow: branchPickerRows options plus the one
			// "… N more" line, at every cursor position.
			if len(rows) != branchPickerRows+1 {
				t.Fatalf("%d rows at cursor %d, want %d:\n%s",
					len(rows), cursor, branchPickerRows+1, strings.Join(rows, "\n"))
			}

			cursorRows := 0
			for _, row := range rows {
				if strings.Contains(row, "▸") {
					cursorRows++
					if !strings.Contains(row, options[cursor]) {
						t.Errorf("the cursor is on %q, want %q", row, options[cursor])
					}
				}
			}
			if cursorRows != 1 {
				t.Errorf("%d rows carry the cursor, want 1:\n%s", cursorRows, strings.Join(rows, "\n"))
			}

			// The hidden count is explicit, and it is the real number.
			want := fmt.Sprintf("… %d more", len(options)-branchPickerRows)
			if last := rows[len(rows)-1]; !strings.Contains(last, want) {
				t.Errorf("last row = %q, want it to state %q", last, want)
			}
		})
	}
}

// A list that fits shows every option and claims nothing is hidden.
func TestBranchPickerShortListHidesNothing(t *testing.T) {
	body := renderBranchPickerBody([]string{"main", "wip"}, "main", 0, branchPickerRows)

	if n := len(strings.Split(body, "\n")); n != 2 {
		t.Errorf("%d rows for a 2-branch repository, want 2:\n%s", n, body)
	}
	if strings.Contains(body, "more") {
		t.Errorf("a list that fits claims rows are hidden:\n%s", body)
	}
}

// The body is never blank. openBranchModal routes an empty list to a notice,
// so this is the guard behind that rather than a reachable state — a blank
// content area is the silent empty state @UI Patterns forbids.
func TestBranchPickerBodyIsNeverBlank(t *testing.T) {
	if body := renderBranchPickerBody(nil, "", 0, branchPickerRows); strings.TrimSpace(body) == "" {
		t.Error("an empty option list rendered a blank body")
	}
}

// TestBranchPickerFitsAtEightyByTwentyFour is the sizing claim that can fail a
// build: forty branches, the minimum terminal, and the whole box still on
// screen with its hint bar and bottom border.
func TestBranchPickerFitsAtEightyByTwentyFour(t *testing.T) {
	branches := make([]string, 40)
	for i := range branches {
		branches[i] = fmt.Sprintf("feature/long-branch-name-%02d", i)
	}

	m := openPicker(t, pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: branches[0], Branches: branches,
	}))
	m.width, m.height = 80, 24

	out := ansi.Strip(m.View())
	lines := strings.Split(out, "\n")

	if len(lines) > 24 {
		t.Errorf("the picker rendered %d lines at height 24", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line %d is %d columns wide at width 80: %q", i, w, line)
		}
	}
	// The two things truncation would take first, per the reason the picker
	// windows rather than overflowing.
	if !strings.Contains(out, "Esc/Cancel") {
		t.Error("the hint bar was pushed off the screen")
	}
	if !strings.Contains(out, "╰") {
		t.Error("the bottom border was pushed off the screen")
	}
}

// ------------------------------------------------------------- the notices

// noticeStates is every state that has no branch list. Each must open a modal
// with a legible reason: @UI Patterns forbids a silent empty state, and a key
// that does nothing at all is exactly that.
func noticeStates() []struct {
	name  string
	entry *gitEntry
	want  string
} {
	return []struct {
		name  string
		entry *gitEntry
		want  string
	}{
		{name: "never asked", entry: nil, want: "git state"},
		{name: "still loading", entry: &gitEntry{status: gitLoading}, want: "git state"},
		{name: "not a repository", entry: &gitEntry{status: gitAbsent}, want: "not in a git repository"},
		{name: "unreadable", entry: &gitEntry{status: gitUnknown}, want: "could not be read"},
		{
			name:  "no commits yet",
			entry: &gitEntry{status: gitOK, state: domain.RepoState{Head: domain.HeadUnborn}},
			want:  "no commits yet",
		},
	}
}

func TestEveryUnavailableStateOpensALegibleNotice(t *testing.T) {
	seen := map[string]string{}

	for _, tt := range noticeStates() {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel()
			if tt.entry != nil {
				m = m.setGitEntry("alpha", *tt.entry)
			}

			opened, cmd := targetKeymap().dispatch("b")(m, "b")
			if cmd != nil {
				t.Error("`b` issued a command on a state with no branches")
			}
			if !opened.modal.active {
				t.Fatal("`b` was a dead key; every unavailable state gets a reason")
			}
			if opened.modal.content != modalNotice {
				t.Fatalf("content = %v, want a notice", opened.modal.content)
			}

			body := opened.renderModalBody()
			if strings.TrimSpace(body) == "" {
				t.Fatal("the notice body is blank")
			}
			if !strings.Contains(body, tt.want) {
				t.Errorf("the notice does not explain itself: %q, want it to mention %q", body, tt.want)
			}

			// Info content: nothing to confirm, so the bar closes.
			if got := ansi.Strip(opened.modal.keys.hintBar()); got != "Esc/Close" {
				t.Errorf("notice hint bar = %q, want %q", got, "Esc/Close")
			}

			// It fits, untruncated, at the floor.
			out := ansi.Strip(opened.View())
			if n := len(strings.Split(out, "\n")); n > 24 {
				t.Errorf("the notice rendered %d lines at height 24", n)
			}
			for i, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > 80 {
					t.Errorf("line %d is %d columns wide at width 80: %q", i, w, line)
				}
			}
			if strings.Contains(body, "…") {
				t.Errorf("the notice was truncated rather than wrapped: %q", body)
			}

			closed, cmd := press(t, opened, "esc")
			if closed.modal.active {
				t.Error("Esc left the notice open")
			}
			if cmd != nil {
				t.Error("Esc issued a command while closing")
			}

			seen[tt.name] = strings.TrimSpace(body)
		})
	}

	// Three different sentences, not one sentence three times: the user must
	// be able to tell "no repository" from "unreadable" from "no commits".
	for aName, a := range seen {
		for bName, b := range seen {
			// The two in-flight states are deliberately the same sentence —
			// "no entry yet" and "read in flight" are the same fact.
			if aName < bName && a == b && !(strings.Contains(a, "Still reading")) {
				t.Errorf("%s and %s show the same sentence %q", aName, bName, a)
			}
		}
	}
}

// ------------------------------------------------------------ opening it

func TestPickerOpensOnTheCurrentBranch(t *testing.T) {
	m := openPicker(t, pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Branches: []string{"feat/api", "main", "wip"},
	}))

	if m.modal.input.cursor != 1 {
		t.Errorf("cursor opened at %d, want 1 — the row for the current branch", m.modal.input.cursor)
	}
	if m.modal.title != "Switch branch" {
		t.Errorf("modal title = %q", m.modal.title)
	}
	if m.modal.context != "alpha" {
		t.Errorf("modal context = %q, want the project name", m.modal.context)
	}
}

// A detached HEAD opens the picker normally — switching to a branch is a
// legitimate escape from it, so nothing is disabled.
func TestPickerOpensWhenDetached(t *testing.T) {
	m := openPicker(t, pickerModel(domain.RepoState{
		Head: domain.HeadDetached, Branches: []string{"main", "wip"},
	}))

	if m.modal.input.cursor != 0 {
		t.Errorf("cursor opened at %d, want the top", m.modal.input.cursor)
	}
	if strings.Contains(m.renderModalBody(), "(current)") {
		t.Error("a detached HEAD marked a row as current")
	}
}

func TestBranchPickerHintBar(t *testing.T) {
	// The AC's prescribed Enter/Confirm  Esc/Cancel, plus the navigation key
	// @UI Patterns permits a picker to add.
	want := "↑↓/Navigate  Enter/Confirm  Esc/Cancel"
	if got := ansi.Strip(branchPickerKeymap().hintBar()); got != want {
		t.Errorf("picker hint bar = %q, want %q", got, want)
	}
}

func TestPickerNavigationStaysInBounds(t *testing.T) {
	m := openPicker(t, pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Branches: []string{"main", "wip"},
	}))

	for i := 0; i < 4; i++ {
		m, _ = press(t, m, "up")
	}
	if m.modal.input.cursor != 0 {
		t.Errorf("cursor walked above the first row: %d", m.modal.input.cursor)
	}
	for i := 0; i < 4; i++ {
		m, _ = press(t, m, "down")
	}
	if m.modal.input.cursor != 1 {
		t.Errorf("cursor walked past the last row: %d", m.modal.input.cursor)
	}
}

func TestEscCancelsWithoutCheckingOut(t *testing.T) {
	before := pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Branches: []string{"main", "wip"},
	})

	m, cmd := press(t, openPicker(t, before), "esc")
	if m.modal.active {
		t.Error("Esc left the picker open")
	}
	if cmd != nil {
		t.Error("Esc issued a command; cancelling runs nothing")
	}
	if m.targetCursor != before.targetCursor || m.view != before.view {
		t.Error("Esc disturbed the underlying view")
	}
}

// `b` inside an open modal is a no-op, like every other key the modal does not
// bind. Without this the picker could open over the help overlay.
func TestBInsideAnOpenModalIsANoOp(t *testing.T) {
	opened, _ := targetKeymap().dispatch("?")(testModel(), "?")

	updated, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	after := updated.(Model)

	if cmd != nil {
		t.Error("`b` issued a command through an open modal")
	}
	if after.modal.content != modalHelp {
		t.Errorf("`b` changed the open modal's content to %v", after.modal.content)
	}
}

// ------------------------------------------------- one read, one snapshot

func TestOpeningThePickerSnapshotsTheRepoState(t *testing.T) {
	state := domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Dirty: true,
		Branches: []string{"feature", "main"},
	}

	m := openPicker(t, pickerModel(state))

	if m.modal.repo.Branch != state.Branch || !slices.Equal(m.modal.repo.Branches, state.Branches) {
		t.Errorf("the modal snapshotted %+v, want %+v", m.modal.repo, state)
	}
	if !m.modal.repo.Dirty {
		t.Error("the snapshot dropped the dirty flag; CheckoutBranch is validated against the whole state")
	}
}

// TestAReadLandingMidModalDoesNotInvalidateAVisibleRow is why the snapshot is
// the whole RepoState rather than the option list alone.
//
// A background read landing while the picker is up replaces the cache entry.
// If the Enter handler validated against the cache, the user could select a
// row that is on screen in front of them and be refused for a branch list they
// cannot see — a narrow window, and unreproducible once it happens.
func TestAReadLandingMidModalDoesNotInvalidateAVisibleRow(t *testing.T) {
	m := openPicker(t, pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Branches: []string{"feature", "main"},
	}))

	// A read lands: feature is gone from the repository's branch list.
	m = m.applyGitState(gitStateMsg{
		project: "alpha", gen: m.gitGen, status: gitOK,
		state: domain.RepoState{Head: domain.HeadOnBranch, Branch: "main", Branches: []string{"main"}},
	})
	if slices.Contains(m.gitCache["alpha"].state.Branches, "feature") {
		t.Fatal("the mid-modal read did not land; this test is not exercising the hazard")
	}

	// The user selects the row they can still see.
	m, _ = press(t, m, "up")
	if got := m.modal.repo.Branches[m.modal.input.cursor]; got != "feature" {
		t.Fatalf("the cursor is on %q, want feature", got)
	}

	_, cmd := press(t, m, "enter")
	for _, msg := range runCmd(cmd) {
		if failed, ok := msg.(runFailedMsg); ok {
			t.Fatalf("selecting a row that is on screen was refused: %v — "+
				"the Enter handler read the cache instead of the snapshot", failed.err)
		}
	}

	// Non-vacuity: the refusal path is live, so the assertion above is not
	// passing because nothing can ever be refused.
	proj, _ := m.currentProject()
	msgs := runCmd(m.checkout(proj, "never-existed"))
	if len(msgs) != 1 {
		t.Fatalf("checkout of an unknown branch produced %d messages, want 1", len(msgs))
	}
	if _, ok := msgs[0].(runFailedMsg); !ok {
		t.Errorf("a branch absent from the snapshot produced %T, want a refusal", msgs[0])
	}
}

// A branch name too wide for the modal is ellipsised in the display, but the
// checkout gets the real string: the snapshot holds it, and the rendered row is
// never what gets passed on.
func TestALongBranchNameIsEllipsisedButCheckedOutInFull(t *testing.T) {
	long := "feature/" + strings.Repeat("very-long-", 8) + "name"

	m := openPicker(t, pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Branches: []string{long, "main"},
	}))
	m.width, m.height = 80, 24

	if !strings.Contains(ansi.Strip(m.View()), "…") {
		t.Error("an over-wide branch name was not ellipsised")
	}

	m, _ = press(t, m, "up")
	if got := m.modal.repo.Branches[m.modal.input.cursor]; got != long {
		t.Fatalf("the snapshot holds %q, want the full name", got)
	}

	// A refusal here would mean the truncated display string reached
	// CheckoutBranch, which validates against the snapshot's real names.
	_, cmd := press(t, m, "enter")
	for _, msg := range runCmd(cmd) {
		if failed, ok := msg.(runFailedMsg); ok {
			t.Fatalf("the checkout was refused: %v — the display string reached the command", failed.err)
		}
	}
}

// ------------------------------------------------------- the return path

// TestARefusedCheckoutDoesNotClaimTheNewBranch is the load-bearing test of
// this feature.
//
// git refuses the checkout — a dirty tree it will not overwrite — so HEAD does
// not move. The reader still answers main, because that is what the repository
// still says. Nothing on screen may claim otherwise: not the header, not a
// flash, not anything derived from the branch the user picked.
func TestARefusedCheckoutDoesNotClaimTheNewBranch(t *testing.T) {
	dirtyOnMain := domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Dirty: true,
		Branches: []string{"feature", "main"},
	}

	m := pickerModel(dirtyOnMain)
	// The repository after the refused checkout: unchanged.
	m.app = app.New(fakeScanner{}, fakeRunner{}, fakeGitReader{state: dirtyOnMain})

	m = openPicker(t, m)
	m, _ = press(t, m, "up")
	if got := m.modal.repo.Branches[m.modal.input.cursor]; got != "feature" {
		t.Fatalf("the cursor is on %q, want feature", got)
	}

	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter ran nothing")
	}
	if m.modal.active {
		t.Error("Enter left the picker open behind the handover")
	}

	after := returnFromHandover(t, m, checkoutFinishedMsg{})

	if got := header(after); !strings.Contains(got, "main ● dirty") {
		t.Errorf("the header does not show the state that was read: %q", got)
	}
	// The whole screen, not just the header: a flash naming the branch would
	// be the same lie one line lower.
	if screen := ansi.Strip(after.View()); strings.Contains(screen, "feature") {
		t.Errorf("the display names the branch the user selected after git refused to switch to it:\n%s", screen)
	}
	if after.flash != "" {
		t.Errorf("a flash was set on return from a checkout: %q", after.flash)
	}
}

// TestASuccessfulCheckoutShowsTheNewBranch is the twin the test above needs to
// mean anything. Without it, a header that never updated at all would pass.
func TestASuccessfulCheckoutShowsTheNewBranch(t *testing.T) {
	before := domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main",
		Branches: []string{"feature", "main"},
	}

	m := pickerModel(before)
	// The repository after the checkout succeeded: HEAD moved.
	m.app = app.New(fakeScanner{}, fakeRunner{}, fakeGitReader{state: domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "feature",
		Branches: []string{"feature", "main"},
	}})

	m = openPicker(t, m)
	m, _ = press(t, m, "up")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter ran nothing")
	}

	after := returnFromHandover(t, m, checkoutFinishedMsg{})

	if got := header(after); !strings.Contains(got, "feature ✓ clean") {
		t.Errorf("the header does not show the branch that was read: %q", got)
	}
}

// TestCheckoutFinishedMsgCarriesNothing makes the load-bearing property
// structural rather than sampled.
//
// TestARefusedCheckoutDoesNotClaimTheNewBranch drives the return path with a
// message it constructs itself, so it can only observe what the *model* does
// with that message — a field added here and rendered from would be invisible
// to it if the test happened to leave the field zero. A message type with no
// fields cannot carry the branch, the exit code, or anything else derived from
// the checkout, whatever a later case does with it.
//
// If a future feature genuinely needs something back from a checkout, this is
// the test to argue with first: the reason it is empty is that everything the
// display says about the branch must come from a read.
func TestCheckoutFinishedMsgCarriesNothing(t *testing.T) {
	if n := reflect.TypeOf(checkoutFinishedMsg{}).NumField(); n != 0 {
		t.Errorf("checkoutFinishedMsg has %d fields; it must carry nothing back from the "+
			"handover, so that no code path can write the selected branch into the display", n)
	}
}

// The return carries nothing into the model: no flash, no lastRun, no record
// of what was selected.
func TestCheckoutFinishedSaysNothing(t *testing.T) {
	m := pickerModel(domain.RepoState{
		Head: domain.HeadOnBranch, Branch: "main", Branches: []string{"main"},
	})
	m.flash = "an earlier message"

	after, cmd := m.dispatch(checkoutFinishedMsg{})

	if after.flash != "an earlier message" {
		t.Errorf("checkoutFinishedMsg touched the flash: %q", after.flash)
	}
	if after.lastRun != nil {
		t.Errorf("checkoutFinishedMsg recorded a run result: %+v", after.lastRun)
	}
	// Compared against the message tea.EnterAltScreen itself produces: the
	// type is unexported, and a bare cmd != nil would pass for any other
	// command the case might return instead.
	if got := runCmd(cmd); len(got) != 1 || got[0] != tea.EnterAltScreen() {
		t.Errorf("checkoutFinishedMsg produced %v, want tea.EnterAltScreen — the handover left "+
			"the alt screen and something has to re-enter it", got)
	}
}

// checkoutStatus quotes git rather than narrating over it, and names no
// branch on either outcome.
func TestCheckoutStatusNamesNoBranch(t *testing.T) {
	ok := checkoutStatus(0, nil, 0)
	failed := checkoutStatus(1, fmt.Errorf("exit status 1"), 0)

	if strings.Contains(ok, "branch '") || strings.Contains(failed, "branch '") {
		t.Errorf("checkoutStatus named a branch: %q / %q", ok, failed)
	}
	if ok == failed {
		t.Error("checkoutStatus reports success and failure identically")
	}
	if !strings.Contains(failed, "exit status 1") {
		t.Errorf("the failure line drops git's own error: %q", failed)
	}
}
