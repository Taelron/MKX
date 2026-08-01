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

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch m.view {
	case viewProjects:
		return m.handleProjectKeys(msg)
	case viewTargets:
		return m.handleTargetKeys(msg)
	}
	return m, nil
}

func (m Model) handleProjectKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
	case "down", "j":
		if m.projectCursor < len(m.projects)-1 {
			m.projectCursor++
		}
	case "enter":
		if len(m.projects) > 0 {
			m.selectedProject = m.projectCursor
			m.targetCursor = 0
			m.view = viewTargets
		}
	case "?":
		if len(m.projects) > 0 {
			m.flash = ""
			return m, viewReadme(m.app.ReadmePath(m.rootCtx, m.projects[m.projectCursor]))
		}
	case "g":
		if len(m.projects) > 0 {
			return m, m.pull(m.projectCursor)
		}
	}
	return m, nil
}

func (m Model) handleTargetKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	proj := m.projects[m.selectedProject]

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.view = viewProjects
		m.lastRun = nil
	case "up", "k":
		if m.targetCursor > 0 {
			m.targetCursor--
		}
	case "down", "j":
		if m.targetCursor < len(proj.Targets)-1 {
			m.targetCursor++
		}
	case "r", "enter":
		if len(proj.Targets) > 0 {
			return m, m.runTarget(proj, proj.Targets[m.targetCursor])
		}
	case "?":
		m.flash = ""
		return m, viewReadme(m.app.ReadmePath(m.rootCtx, proj))
	case "g":
		return m, m.pull(m.selectedProject)
	}
	return m, nil
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
	s += m.renderHintBar(keymap{
		{display: "Enter", label: "Drill In", inBar: true},
		{display: "g", label: "Pull", inBar: true},
		{display: "?", label: "Readme", inBar: true},
		{display: "q", label: "Quit", inBar: true},
	})
	return s
}

func (m Model) renderTargetList() string {
	proj := m.projects[m.selectedProject]
	w := m.width
	if w < 20 {
		w = 80
	}

	iw := w - 2

	// header
	s := m.renderHeader(proj.Name, m.targetCursor+1, len(proj.Targets)) + "\n"
	s += styles.Border.Render("┌"+strings.Repeat("─", iw)+"┐") + "\n"

	if len(proj.Targets) == 0 {
		s += borderedRow("  No targets found.", iw, styles.NormalRow) + "\n"
	} else {
		nameCol := len("TARGET")
		for _, t := range proj.Targets {
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
		if maxVisible < 1 {
			maxVisible = 1
		}

		offset := 0
		if m.targetCursor >= offset+maxVisible {
			offset = m.targetCursor - maxVisible + 1
		}
		end := offset + maxVisible
		if end > len(proj.Targets) {
			end = len(proj.Targets)
		}

		for i := offset; i < end; i++ {
			t := proj.Targets[i]
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

	hints := keymap{
		{display: "r", label: "Run", inBar: true},
		{display: "g", label: "Pull", inBar: true},
		{display: "?", label: "Readme", inBar: true},
		{display: "Esc", label: "Back", inBar: true},
	}
	if m.lastRun != nil {
		status := "✓"
		if m.lastRun.ExitCode != 0 {
			status = fmt.Sprintf("✗ exit %d", m.lastRun.ExitCode)
		}
		m.flash = fmt.Sprintf("%s %s", status, m.lastRun.Duration.Round(time.Second))
	}
	s += m.renderHintBar(hints)
	return s
}
