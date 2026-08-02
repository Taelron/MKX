package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// The branch picker: the `b` key's whole surface.
//
// The load-bearing property of this file is that nothing in it ever names a
// branch it did not just read. The picker's rows come from a RepoState
// snapshot, the checkout carries that same snapshot into app.CheckoutBranch,
// and the header segment afterwards comes from a fresh read issued by the
// handover invalidation — never from the checkout's exit code, and never from
// the branch the user selected. A refused checkout therefore cannot produce a
// display claiming the new branch, because no code path exists that could
// write one.

// branchPickerRows is how many branch rows the picker shows at once.
//
// The box is fixed at this height whatever the repository holds: seven rows,
// the "… N more" line, header, hint bar, two rules and two borders is 13 lines,
// which leaves room at the 80×24 floor @UI Patterns requires modals to fit.
const branchPickerRows = 7

// renderBranchPickerBody renders the option list: one row per branch, the
// cursor on one of them, "(current)" against the branch the snapshot says is
// checked out, and a count of what the window hides.
//
// It windows rather than growing, which @UI Patterns permits for exactly this
// content: "a select picker over an unbounded option list may window its
// options within a fixed-height box. The box does not grow, the header and hint
// bar are never pushed out, and the number of hidden options is stated
// explicitly." The alternative is not a taller box — buildModalBox has no
// height clamp and splice silently drops overflowing lines, so a thirty-branch
// repository would lose its hint bar and bottom border with no signal at all.
//
// It is a pure function over display data — two strings and two ints, no Model,
// no RepoState — so every windowing and marking rule below is table-testable.
//
// current is matched by string equality, not by prefix or containment: a
// repository holding both `main` and `main-2` must mark exactly one of them,
// and a detached HEAD (whose Branch is empty) must mark neither.
func renderBranchPickerBody(options []string, current string, cursor, maxRows int) string {
	if len(options) == 0 {
		// Not reachable through openBranchModal, which routes an empty list
		// to a notice. Kept because renderModalBody is reachable from any
		// modalBranchPicker state, and a blank body is the silent empty state
		// @UI Patterns forbids.
		return "No local branches to switch to."
	}

	// Column width from the data, per @UI Patterns — from every option rather
	// than the visible window, so the marker does not shift as the cursor
	// pages.
	nameCol := 0
	for _, name := range options {
		if w := ansi.StringWidth(name); w > nameCol {
			nameCol = w
		}
	}

	// The same offset arithmetic the two list views use: the window starts at
	// the top and follows the cursor down, so it always contains the cursor.
	offset := 0
	if cursor >= maxRows {
		offset = cursor - maxRows + 1
	}
	end := min(offset+maxRows, len(options))

	rows := make([]string, 0, maxRows+1)
	for i := offset; i < end; i++ {
		row := "   "
		if i == cursor {
			row = " ▸ "
		}
		row += fmt.Sprintf("%-*s", nameCol, options[i])
		if options[i] == current {
			row += "  (current)"
		}
		// The name column's padding is trailing whitespace on every row but
		// the current one, and buildModalBox counts it: an unTrimmed row of
		// spaces past the inner width comes back ellipsised for no visible
		// reason.
		rows = append(rows, strings.TrimRight(row, " "))
	}

	if hidden := len(options) - (end - offset); hidden > 0 {
		rows = append(rows, fmt.Sprintf("   … %d more", hidden))
	}

	return strings.Join(rows, "\n")
}

// renderNoticeBody renders a notice's sentence, wrapped to the modal's inner
// width.
//
// Wrapped rather than clamped: buildModalBox truncates an over-wide line with
// an ellipsis, which for a sentence explaining why the picker is unavailable
// would hide the explanation. Notices are authored and short, so wrapping them
// keeps the whole reason on screen and the modal still fits — the @UI Patterns
// exception this file uses for the picker's option list explicitly does not
// extend to info content.
func renderNoticeBody(text string, inner int) string {
	return ansi.WrapWc(text, inner, "-")
}

// branchPickerKeymap is the picker's own bindings, and its hint bar: the
// baseline's prescribed Enter/Confirm  Esc/Cancel plus the navigation key it
// permits a picker to add.
func branchPickerKeymap() keymap {
	return keymap{
		{keys: []string{"up", "k"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor up", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if m.modal.input.cursor > 0 {
					m.modal.input.cursor--
				}
				return m, nil
			}},
		{keys: []string{"down", "j"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor down", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if m.modal.input.cursor < len(m.modal.repo.Branches)-1 {
					m.modal.input.cursor++
				}
				return m, nil
			}},
		{keys: []string{"enter"}, display: "Enter", label: "Confirm",
			help: "Check out the selected branch", inBar: true,
			handler: confirmBranch},
		{keys: []string{"esc"}, display: "Esc", label: "Cancel",
			help: "Close without switching", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				return m.closeModal(), nil
			}},
	}
}

// noticeKeymap is a notice's bindings. Esc/Close rather than Enter/Confirm,
// following the help overlay: there is nothing to confirm, and a bar
// advertising a no-op Enter is worse than one that omits it.
func noticeKeymap() keymap {
	return keymap{
		{keys: []string{"esc"}, display: "Esc", label: "Close",
			help: "Close this notice", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				return m.closeModal(), nil
			}},
	}
}

// openBranchModal is the `b` handler. The modal always opens; only its body
// varies. Every state that has no branch list gets a sentence saying so —
// @UI Patterns forbids a silent empty state, and a key that does nothing is
// exactly that.
//
// The git cache is read here, once. Everything downstream — the rows, the
// marker, the validation — reads the snapshot this puts on modalState.
func openBranchModal(m Model, _ string) (Model, tea.Cmd) {
	proj, ok := m.currentProject()
	if !ok {
		return m, nil
	}

	entry, cached := m.gitCache[proj.Name]

	switch {
	case !cached || entry.status == gitLoading:
		return m.showNotice(proj, "Still reading this project's git state. Try again in a moment."), nil
	case entry.status == gitAbsent:
		return m.showNotice(proj, "This project is not in a git repository, so it has no branches to switch between."), nil
	case entry.status == gitUnknown:
		return m.showNotice(proj, "This project's git state could not be read, so its branches are unknown."), nil
	case entry.state.Head == domain.HeadUnborn:
		return m.showNotice(proj, "This repository has no commits yet, so there are no branches to switch to."), nil
	case len(entry.state.Branches) == 0:
		return m.showNotice(proj, "This repository reports no local branches to switch to."), nil
	}

	// Detached HEAD needs no case of its own: state.Branch is empty, so no row
	// matches, nothing is marked current, and the cursor opens at the top.
	// Checking out a branch is a legitimate escape from a detached head, so
	// nothing is disabled.
	cursor := 0
	for i, name := range entry.state.Branches {
		if name == entry.state.Branch {
			cursor = i
			break
		}
	}

	m = m.showModal(modalBranchPicker, "Switch branch", proj.Name,
		modalInput{cursor: cursor}, branchPickerKeymap())
	m.modal.repo = entry.state
	return m, nil
}

// showNotice opens the modal on a one-sentence explanation.
func (m Model) showNotice(proj domain.Project, text string) Model {
	m = m.showModal(modalNotice, "Switch branch", proj.Name, modalInput{}, noticeKeymap())
	m.modal.notice = text
	return m
}

// confirmBranch is Enter inside the picker: close the modal and hand the
// terminal to git.
//
// The branch and the snapshot are both taken before closeModal, which clears
// modalState — and the snapshot is taken from the modal rather than re-read
// from the cache, which is the whole point of holding it.
func confirmBranch(m Model, _ string) (Model, tea.Cmd) {
	proj, ok := m.currentProject()
	if !ok {
		return m.closeModal(), nil
	}
	branches := m.modal.repo.Branches
	if m.modal.input.cursor < 0 || m.modal.input.cursor >= len(branches) {
		return m.closeModal(), nil
	}

	cmd := m.checkout(proj, branches[m.modal.input.cursor])
	return m.closeModal(), cmd
}

// checkout asks the app for the checkout command and hands the terminal to it,
// through the same handover() every other MkX subprocess goes through. That is
// what keeps this package at two tea.Exec sites and makes ADR-M003's "RepoState
// is invalidated by every handover" cover the checkout with nothing written for
// it.
//
// The RepoState it validates against is m.modal.repo — the snapshot the rows
// were drawn from. m.gitCache is not consulted here; see modalState.repo.
//
// Selecting the branch already checked out is allowed and hands over normally:
// git says "Already on 'main'". Special-casing it would be pre-validation in
// miniature, for no gain.
func (m Model) checkout(proj domain.Project, branch string) tea.Cmd {
	cmd, err := m.app.CheckoutBranch(m.rootCtx, proj, m.modal.repo, branch)
	if err != nil {
		return func() tea.Msg { return runFailedMsg{err: err} }
	}

	return handover(cmd, proj.Name, checkoutStatus, func(*commandExec) tea.Msg {
		// Deliberately empty. Nothing about the outcome travels back into the
		// TUI: what the header shows next comes from the re-read the wrapper
		// triggers, and an exit code is not a reading of where HEAD points.
		return checkoutFinishedMsg{}
	})
}
