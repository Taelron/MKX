package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

// Git state in the TUI: when it is read, how it is cached, and how it renders.
//
// Reads are lazy and happen on drill-in only — Enter opening a project's
// target view, and nowhere else. The project list issues none and shows none.
// That is what keeps discovery free: a workspace of thirty projects costs zero
// git subprocesses and no added startup latency, where reading eagerly would
// cost ninety before the TUI opened, with one hung repository delaying startup
// by the whole timeout.
//
// The cost is that the project list cannot be scanned for branch or dirty
// state; the user drills in to see it. TAE-58 asks for the state of the
// *selected* Project, and in MkX selection is exactly what Enter sets.

// gitStatus is what is known about a project's git state right now. It is
// deliberately separate from domain.RepoState: "we have not finished asking"
// and "there is nothing to ask about" are UI states, not repository states.
type gitStatus int

const (
	// gitLoading is the zero value, so a freshly seeded entry is in flight by
	// construction rather than by remembering to set the field.
	gitLoading gitStatus = iota
	// gitOK means the read succeeded and state is filled in.
	gitOK
	// gitAbsent means the project is in no git repository. Not an error.
	gitAbsent
	// gitUnknown means the read failed or timed out. Emphatically not clean.
	gitUnknown
)

// gitEntry is one project's cache slot.
type gitEntry struct {
	// gen is the invalidation generation this entry belongs to. An entry from
	// a superseded generation is discarded rather than displayed.
	gen    int
	status gitStatus
	state  domain.RepoState
}

// gitStateMsg carries a finished read back into Update.
type gitStateMsg struct {
	project string
	gen     int
	status  gitStatus
	state   domain.RepoState
}

// readRepoState issues one git read as a tea.Cmd.
//
// It is a plain tea.Cmd per the TUI Go Conventions baseline — no raw
// goroutine, no channel — and the deadline is applied inside app.RepoState, so
// nothing here can hang the event loop. The view is open and fully interactive
// while it runs.
//
// Staleness is guarded by gen rather than by cancelling a view-scoped context.
// That is the generation-counter deviation the baseline permits: MkX has no
// view-scoped contexts today, and the guard closes the race that actually
// matters here — see Model.invalidateGitState.
func readRepoState(ctx context.Context, a *app.App, project domain.Project, gen int) tea.Cmd {
	return func() tea.Msg {
		state, err := a.RepoState(ctx, project)

		msg := gitStateMsg{project: project.Name, gen: gen, state: state}
		switch {
		case err == nil:
			msg.status = gitOK
		case errors.Is(err, app.ErrNotARepository):
			// An absence, per the Domain Model — never an error condition,
			// and never surfaced as a flash.
			msg.status = gitAbsent
			msg.state = domain.RepoState{}
		default:
			// Everything else: a timeout, a cancellation, a broken .git, git
			// missing from PATH. The state is unknown and the RepoState is
			// dropped so no half-filled struct can reach the renderer.
			msg.status = gitUnknown
			msg.state = domain.RepoState{}
		}
		return msg
	}
}

// ensureGitState seeds a loading entry for the project and returns the command
// that reads it, or nil when the project's state is already cached.
//
// The entry is seeded synchronously so the target view's *first* render
// already shows a marker rather than a blank that fills in a frame later.
func (m Model) ensureGitState(project domain.Project) (Model, tea.Cmd) {
	if entry, ok := m.gitCache[project.Name]; ok && entry.gen == m.gitGen {
		// Free until invalidated.
		return m, nil
	}

	m = m.setGitEntry(project.Name, gitEntry{gen: m.gitGen, status: gitLoading})
	return m, readRepoState(m.rootCtx, m.app, project, m.gitGen)
}

// setGitEntry returns a Model whose cache has the entry set, without mutating
// the receiver's map.
//
// Model is a value type per the TUI Go Conventions baseline, and a map field
// is the one thing that quietly breaks that: mutating it in place writes
// through every copy of the Model that ever shared it. Copying keeps the value
// semantics real, and at workspace scale the copy is free.
func (m Model) setGitEntry(name string, entry gitEntry) Model {
	next := make(map[string]gitEntry, len(m.gitCache)+1)
	for k, v := range m.gitCache {
		next[k] = v
	}
	next[name] = entry
	m.gitCache = next
	return m
}

// invalidateGitState drops every cached RepoState and moves to a new
// generation. ADR-M003: RepoState is invalidated by every handover.
//
// The whole cache is cleared, not the handed-over project's entry. A Make
// recipe can cd anywhere and touch any repository, so a per-project clear is
// false precision. Over-invalidating costs one sub-second read on the next
// drill-in; under-invalidating is the silently-wrong-status bug this issue
// exists to prevent.
//
// Bumping the generation is what closes the race the clear alone leaves open:
// a read already in flight for project A when the handover starts can land
// afterwards and repopulate the cache with pre-handover data — ADR-M003's
// exact failure mode, reintroduced inside the machinery meant to prevent it.
// A gitStateMsg whose gen no longer matches is discarded.
func (m Model) invalidateGitState() Model {
	m.gitCache = nil
	m.gitGen++
	return m
}

// applyGitState folds a finished read into the cache, discarding one that a
// handover has already superseded.
func (m Model) applyGitState(msg gitStateMsg) Model {
	if msg.gen != m.gitGen {
		// Silently discarded, per @UI Patterns: a stale result produces no
		// flash, no warning, and no late-arriving render.
		return m
	}
	return m.setGitEntry(msg.project, gitEntry{
		gen:    msg.gen,
		status: msg.status,
		state:  msg.state,
	})
}

// gitSegment renders the git state of the project whose target view is open.
//
// Every branch returns non-empty text. There is deliberately no path that
// returns "" — including the no-entry case, which renders the same in-flight
// marker as a seeded one. That is the first of three independent guards
// against the AC's central hazard, absence reading as clean:
//
//  1. no branch here renders nothing,
//  2. "clean" is spelled out rather than being the absence of a dirty marker,
//  3. domain.RepoState's zero value is HeadUnknown, so even a state that
//     reached this function unfilled reads as unknown rather than as a clean
//     repository on a nameless branch.
//
// Precedence when several facts overlap: absent, then no-commits, then
// unknown, then detached, then dirty, then clean. Detached outranks dirty
// because dirty is meaningless context before knowing you are not on a branch.
func (m Model) gitSegment(projectName string) string {
	entry, ok := m.gitCache[projectName]
	if !ok || entry.status == gitLoading {
		return styles.GitPending.Render("…")
	}

	switch entry.status {
	case gitAbsent:
		return styles.GitPending.Render("no repo")
	case gitUnknown:
		// Red: a read that failed is a failure, and must not be mistakable
		// for the dimmed "there is simply no repository here".
		return styles.GitUnknown.Render("git unknown")
	}

	switch entry.state.Head {
	case domain.HeadUnborn:
		// A repository with no commits yet is a normal state per the Domain
		// Model, so it is dimmed like absence rather than red like failure.
		return styles.GitPending.Render("no commits")
	case domain.HeadDetached:
		return styles.GitAttention.Render("detached " + dirtyMarker(entry.state.Dirty))
	case domain.HeadOnBranch:
		if entry.state.Dirty {
			return styles.GitAttention.Render(entry.state.Branch + " ● dirty")
		}
		return styles.GitClean.Render(entry.state.Branch + " ✓ clean")
	}

	// domain.HeadUnknown with a gitOK status is not a state the adapter
	// produces — classifyHead never reports present with an unknown head. If
	// it ever did, this must read as unknown, never as clean.
	return styles.GitUnknown.Render("git unknown")
}

// dirtyMarker renders the dirty flag on its own, for the detached case where
// the branch name is replaced rather than prefixed.
func dirtyMarker(dirty bool) string {
	if dirty {
		return "● dirty"
	}
	return "✓ clean"
}
