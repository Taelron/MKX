// Package tui is MkX's Bubble Tea front-end. It calls app use cases and owns
// all rendering, keymap and terminal handover; it never touches the
// filesystem or Make itself.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

type view int

const (
	viewProjects view = iota
	viewTargets
)

type runResult struct {
	ExitCode int
	Duration time.Duration
}

// Model holds every piece of MkX's UI state. It is a value type: Bubble Tea's
// update loop takes a Model and returns a possibly-modified Model.
type Model struct {
	app     *app.App
	rootCtx context.Context

	workspace string
	projects  []domain.Project
	view      view

	// project list
	projectCursor int

	// target list
	targetCursor    int
	selectedProject int

	// filter mode over the target list; the zero value is no filter
	filter filterState

	// last run
	lastRun *runResult

	// flash message
	flash string

	// modal overlay; the zero value is inactive
	modal modalState

	// terminal size
	width  int
	height int
}

type execFinishedMsg struct {
	exitCode int
	duration time.Duration
}

type runFailedMsg struct {
	err error
}

type gitPullFinishedMsg struct {
	projectIndex int
	err          error
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case readmeNotFoundMsg:
		m.flash = "No README.md found"
		return m, nil

	case runFailedMsg:
		m.flash = msg.err.Error()
		return m, nil

	case gitPullFinishedMsg:
		targets, _ := m.app.RefreshTargets(m.rootCtx, m.projects[msg.projectIndex])
		m.projects[msg.projectIndex].Targets = targets
		if msg.err == nil {
			m.flash = "Pulled & refreshed"
		} else {
			m.flash = fmt.Sprintf("Pull failed: %v", msg.err)
		}
		if m.view == viewTargets {
			m.targetCursor = 0
		}
		return m, tea.EnterAltScreen

	case execFinishedMsg:
		m.lastRun = &runResult{
			ExitCode: msg.exitCode,
			Duration: msg.duration,
		}
		return m, tea.EnterAltScreen

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey is the only path from a tea.KeyMsg into the model, which is what
// makes the modal interception below total: no key can reach a view's bindings
// while a modal is up, because the view keymap is only consulted past the early
// return.
//
// Dispatch is three tiers, strictly ordered:
//
//  1. the modal's keymap — absolute, an unbound key is a no-op
//  2. filter mode's rune capture, when filter mode is active
//  3. the view's keymap — the registry path
//
// Tier 2 sits after tier 1's unconditional return, so with a modal up the
// filter never sees a key. When filter mode is inactive it is a no-op and the
// path is what it was before filtering existed.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.modal.active {
		if h := m.modal.keys.dispatch(key); h != nil {
			return h(m, key)
		}
		// An unbound key inside a modal is a no-op, never a fall-through.
		return m, nil
	}

	// Below the interception, so keys pressed inside a modal do not clear a
	// flash that is hidden behind it anyway.
	m.flash = ""

	// Rune capture is not expressible as a binding: a binding.keys is a finite
	// list, and capture claims the whole rune space — including keys this same
	// keymap binds (g, R, q, ?, /). So it is its own tier, and it enumerates
	// nothing rather than being a second key list.
	//
	// The consequence is deliberate: while filtering, printable keys type
	// instead of firing. A filter for `generate` is untypeable otherwise.
	// Arrows, Enter, Esc and Ctrl+C fall through to tier 3 and still dispatch.
	//
	// Switching on msg.Type rather than msg.String(): Key.String() prefixes
	// "alt+" for alt-modified runes, and a space arrives as KeySpace, not
	// KeyRunes. A string test would take alt+g as text; a bare KeyRunes test
	// would silently drop spaces, which target descriptions are full of.
	if m.filter.active {
		switch {
		case msg.Type == tea.KeyRunes && !msg.Alt:
			return m.appendFilterRunes(msg.Runes), nil
		case msg.Type == tea.KeySpace && !msg.Alt:
			return m.appendFilterRunes([]rune{' '}), nil
		case msg.Type == tea.KeyBackspace:
			return m.backspaceFilter(), nil
		}
	}

	if h := m.viewKeymap().dispatch(key); h != nil {
		return h(m, key)
	}
	return m, nil
}

// viewKeymap returns the current view's bindings — the single source for its
// key dispatch, its hint bar and its help overlay.
func (m Model) viewKeymap() keymap {
	switch m.view {
	case viewProjects:
		return projectKeymap()
	case viewTargets:
		return targetKeymap()
	}
	return nil
}

// currentProject returns the project the target view is showing. The false
// return covers a Model whose selectedProject is out of range, which the
// bindings guard rather than panicking on.
func (m Model) currentProject() (domain.Project, bool) {
	if m.selectedProject < 0 || m.selectedProject >= len(m.projects) {
		return domain.Project{}, false
	}
	return m.projects[m.selectedProject], true
}

// runTarget asks the app for the command that runs t, then hands the terminal
// to it.
func (m Model) runTarget(proj domain.Project, t domain.Target) tea.Cmd {
	cmd, err := m.app.RunTarget(m.rootCtx, proj, t.Name)
	if err != nil {
		return func() tea.Msg { return runFailedMsg{err: err} }
	}

	return handover(cmd, proj.Name, targetStatus, func(ce *commandExec) tea.Msg {
		return execFinishedMsg{
			exitCode: ce.exitCode,
			duration: ce.duration,
		}
	})
}

// pull asks the app for the pull command, hands the terminal to it, and
// reports back so Update can re-discover the project's targets.
func (m Model) pull(projectIndex int) tea.Cmd {
	proj := m.projects[projectIndex]

	cmd, err := m.app.PullAndRefresh(m.rootCtx, proj)
	if err != nil {
		return func() tea.Msg {
			return gitPullFinishedMsg{projectIndex: projectIndex, err: err}
		}
	}

	return handover(cmd, proj.Name, pullStatus, func(ce *commandExec) tea.Msg {
		return gitPullFinishedMsg{
			projectIndex: projectIndex,
			err:          ce.err,
		}
	})
}

func (m Model) View() string {
	var base string
	switch m.view {
	case viewProjects:
		base = m.renderProjectList()
	case viewTargets:
		base = m.renderTargetList()
	}

	if m.modal.active {
		return m.renderModal(base)
	}
	return base
}

// renderHintBar renders a view's keymap as the bottom bar, with any flash
// message appended. The key/Action formatting lives on keymap.hintBar; this
// function only owns the flash and the bar's width.
func (m Model) renderHintBar(k keymap) string {
	bar := k.hintBar()
	if m.flash != "" {
		bar += "  " + styles.Flash.Render(m.flash)
	}
	return styles.HintBar.Width(m.width).Render(bar)
}

func (m Model) renderHeader(section string, current, total int) string {
	title := "mkx › " + section
	left := styles.Header.Render(title)
	count := styles.HeaderCount.Render(fmt.Sprintf("%d/%d", current, total))
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(count)
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + count
}

// borderedRow wraps content with │ on each side, clamped to exactly w display columns.
func borderedRow(content string, w int, style lipgloss.Style) string {
	dw := runewidth.StringWidth(content)
	if dw > w {
		// truncate to fit
		truncated := ""
		col := 0
		for _, r := range content {
			rw := runewidth.RuneWidth(r)
			if col+rw >= w {
				break
			}
			truncated += string(r)
			col += rw
		}
		content = truncated + "…"
		dw = runewidth.StringWidth(content)
	}
	if dw < w {
		content += strings.Repeat(" ", w-dw)
	}
	b := styles.Border.Render("│")
	return b + style.Render(content) + b
}

func (m Model) renderProjectList() string {
	w := m.width
	if w < 20 {
		w = 80
	}

	iw := w - 2 // inner width (excluding │ borders)

	// header
	s := m.renderHeader("Projects", m.projectCursor+1, len(m.projects)) + "\n"
	s += styles.Border.Render("┌"+strings.Repeat("─", iw)+"┐") + "\n"

	if len(m.projects) == 0 {
		s += borderedRow("  No projects with Makefiles found.", iw, styles.NormalRow) + "\n"
	} else {
		// column widths
		nameCol := len("PROJECT")
		for _, p := range m.projects {
			if len(p.Name) > nameCol {
				nameCol = len(p.Name)
			}
		}
		nameCol += 2
		targetsCol := 8

		// column headers
		tHdr := "TARGETS"
		tPad := targetsCol - len(tHdr)
		tLpad := tPad / 2
		tRpad := tPad - tLpad
		centeredTargets := strings.Repeat(" ", tLpad) + tHdr + strings.Repeat(" ", tRpad)
		hdr := fmt.Sprintf("      %-*s  %s  %s", nameCol, "PROJECT", centeredTargets, "DESCRIPTION")
		s += borderedRow(hdr, iw, styles.ColumnHeader) + "\n"
		s += styles.Border.Render("├"+strings.Repeat("─", iw)+"┤") + "\n"

		// scrollable viewport: 8 lines reserved for header + col header + borders + hint bar
		maxVisible := m.height - 8
		if maxVisible < 1 {
			maxVisible = 1
		}

		offset := 0
		if m.projectCursor >= offset+maxVisible {
			offset = m.projectCursor - maxVisible + 1
		}
		end := offset + maxVisible
		if end > len(m.projects) {
			end = len(m.projects)
		}

		for i := offset; i < end; i++ {
			p := m.projects[i]
			cur := "   "
			if i == m.projectCursor {
				cur = " ▸ "
			}

			name := fmt.Sprintf("%-*s", nameCol, p.Name)
			countStr := fmt.Sprintf("%d", len(p.Targets))
			pad := targetsCol - len(countStr)
			lpad := pad / 2
			rpad := pad - lpad
			count := strings.Repeat(" ", lpad) + countStr + strings.Repeat(" ", rpad)
			desc := p.Description
			if desc == "" {
				desc = "—"
			}
			line := fmt.Sprintf("%s  %s  %s  %s", cur, name, count, desc)

			var style lipgloss.Style
			if i == m.projectCursor {
				style = styles.SelectedRow
			} else if i%2 == 0 {
				style = styles.AltRow
			} else {
				style = styles.NormalRow
			}
			s += borderedRow(line, iw, style) + "\n"
		}
	}

	// pad to fill screen with bordered empty rows
	lines := strings.Count(s, "\n")
	for i := lines; i < m.height-3; i++ {
		s += borderedRow("", iw, styles.NormalRow) + "\n"
	}

	s += styles.Border.Render("└"+strings.Repeat("─", iw)+"┘") + "\n"
	s += m.renderHintBar(projectKeymap())
	return s
}

func (m Model) renderTargetList() string {
	proj := m.projects[m.selectedProject]
	// Computed once: the header count, the column widths, the viewport window
	// and the row loop all read this, so nothing below branches on whether a
	// filter is in effect.
	targets := m.filteredTargets(proj)
	w := m.width
	if w < 20 {
		w = 80
	}

	iw := w - 2

	// header
	s := m.renderHeader(proj.Name, m.targetCursor+1, len(targets)) + "\n"
	s += styles.Border.Render("┌"+strings.Repeat("─", iw)+"┐") + "\n"

	// The filter bar renders on the text, not on the mode: after Enter closes
	// filter mode the list stays filtered, and this bar is what explains why.
	// It is part of the view, below the top border, never an overlay.
	filterBar := m.filter.text != ""
	if filterBar {
		s += borderedRow("  Filter: "+m.filter.text, iw, styles.FilterBar) + "\n"
		s += styles.Border.Render("├"+strings.Repeat("─", iw)+"┤") + "\n"
	}

	switch {
	case len(proj.Targets) == 0:
		s += borderedRow("  No targets found.", iw, styles.NormalRow) + "\n"
	case len(targets) == 0:
		// A filter matching nothing gets an empty state naming a real key in
		// this view, not a blank content area.
		s += borderedRow("  No targets match. Press Esc to clear the filter.", iw, styles.NormalRow) + "\n"
	default:
		nameCol := len("TARGET")
		for _, t := range targets {
			if len(t.Name) > nameCol {
				nameCol = len(t.Name)
			}
		}
		nameCol += 2

		// column headers
		hdr := fmt.Sprintf("      %-*s  %s", nameCol, "TARGET", "DESCRIPTION")
		s += borderedRow(hdr, iw, styles.ColumnHeader) + "\n"
		s += styles.Border.Render("├"+strings.Repeat("─", iw)+"┤") + "\n"

		maxVisible := m.height - 8
		if filterBar {
			// The bar and its separator are two more lines; without this the
			// content area overflows its frame by two rows.
			maxVisible -= 2
		}
		if maxVisible < 1 {
			maxVisible = 1
		}

		offset := 0
		if m.targetCursor >= offset+maxVisible {
			offset = m.targetCursor - maxVisible + 1
		}
		end := offset + maxVisible
		if end > len(targets) {
			end = len(targets)
		}

		for i := offset; i < end; i++ {
			t := targets[i]
			cur := "   "
			if i == m.targetCursor {
				cur = " ▸ "
			}

			desc := t.Description
			if desc == "" {
				desc = "—"
			}
			name := fmt.Sprintf("%-*s", nameCol, t.Name)
			line := fmt.Sprintf("%s  %s  %s", cur, name, desc)

			var style lipgloss.Style
			if i == m.targetCursor {
				style = styles.SelectedRow
			} else if i%2 == 0 {
				style = styles.AltRow
			} else {
				style = styles.NormalRow
			}
			s += borderedRow(line, iw, style) + "\n"
		}
	}

	// pad to fill screen with bordered empty rows
	lines := strings.Count(s, "\n")
	for i := lines; i < m.height-3; i++ {
		s += borderedRow("", iw, styles.NormalRow) + "\n"
	}

	s += styles.Border.Render("└"+strings.Repeat("─", iw)+"┘") + "\n"

	if m.lastRun != nil {
		status := "✓"
		if m.lastRun.ExitCode != 0 {
			status = fmt.Sprintf("✗ exit %d", m.lastRun.ExitCode)
		}
		m.flash = fmt.Sprintf("%s %s", status, m.lastRun.Duration.Round(time.Second))
	}
	s += m.renderHintBar(targetKeymap())
	return s
}
