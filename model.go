package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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

type model struct {
	workspace string
	projects  []project
	view      view

	// project list
	projectCursor int

	// target list
	targetCursor   int
	selectedProject int

	// last run
	lastRun *runResult

	// flash message
	flash string

	// terminal size
	width  int
	height int
}

func newModel(workspace string, projects []project) model {
	return model{
		workspace: workspace,
		projects:  projects,
		view:      viewProjects,
	}
}

type execFinishedMsg struct {
	exitCode int
	duration time.Duration
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case readmeNotFoundMsg:
		m.flash = "No README.md found"
		return m, nil

	case gitPullFinishedMsg:
		targets, _ := parseTargets(m.projects[msg.projectIndex].Path)
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

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch m.view {
	case viewProjects:
		return m.handleProjectKeys(msg)
	case viewTargets:
		return m.handleTargetKeys(msg)
	}
	return m, nil
}

func (m model) handleProjectKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, viewReadme(m.projects[m.projectCursor])
		}
	case "g":
		if len(m.projects) > 0 {
			return m, gitPull(m.projectCursor, m.projects[m.projectCursor])
		}
	}
	return m, nil
}

func (m model) handleTargetKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, m.runTarget(proj, proj.Targets[m.targetCursor], "")
		}
	case "?":
		m.flash = ""
		return m, viewReadme(proj)
	case "g":
		return m, gitPull(m.selectedProject, proj)
	}
	return m, nil
}

// makeExec wraps a make command + "press Enter" wait into a single tea.ExecCommand.
type makeExec struct {
	project  string
	dir      string
	makeArgs []string
	start    time.Time
	exitCode int
	duration time.Duration
}

func (m *makeExec) Run() error {
	sep := strings.Repeat("━", 50)
	fmt.Printf("\n%s\n▶ make %s  (%s)\n%s\n\n", sep, strings.Join(m.makeArgs, " "), m.project, sep)

	c := exec.Command("make", m.makeArgs...)
	c.Dir = m.dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	err := c.Run()
	m.duration = time.Since(m.start)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			m.exitCode = exitErr.ExitCode()
		} else {
			m.exitCode = 1
		}
	}

	status := "✓ success"
	if m.exitCode != 0 {
		status = fmt.Sprintf("✗ exit %d", m.exitCode)
	}
	fmt.Printf("\n%s\n%s (%s)\nPress Enter to return to mkx...", sep, status, m.duration.Round(time.Second))
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	return err
}

func (m *makeExec) SetStdin(r io.Reader)  { /* handled in Run */ }
func (m *makeExec) SetStdout(w io.Writer) { /* handled in Run */ }
func (m *makeExec) SetStderr(w io.Writer) { /* handled in Run */ }

func (m model) runTarget(proj project, t target, args string) tea.Cmd {
	makeArgs := []string{t.Name}
	if args != "" {
		makeArgs = append(makeArgs, args)
	}

	me := &makeExec{
		project:  proj.Name,
		dir:      proj.Path,
		makeArgs: makeArgs,
		start:    time.Now(),
	}

	return tea.Exec(me, func(err error) tea.Msg {
		return execFinishedMsg{
			exitCode: me.exitCode,
			duration: me.duration,
		}
	})
}

func (m model) View() string {
	switch m.view {
	case viewProjects:
		return m.renderProjectList()
	case viewTargets:
		return m.renderTargetList()
	}
	return ""
}

func (m model) renderHintBar(hints [][]string) string {
	var parts []string
	for _, h := range hints {
		parts = append(parts, hintKeyStyle.Render("<"+h[0]+">")+" "+hintActionStyle.Render(h[1]))
	}
	bar := strings.Join(parts, "  ")
	if m.flash != "" {
		bar += "  " + flashStyle.Render(m.flash)
	}
	return hintBarStyle.Width(m.width).Render(bar)
}

func (m model) renderHeader(section string, current, total int) string {
	title := "mkx › " + section
	left := headerStyle.Render(title)
	count := headerCountStyle.Render(fmt.Sprintf("%d/%d", current, total))
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
	b := borderStyle.Render("│")
	return b + style.Render(content) + b
}

func (m model) renderProjectList() string {
	w := m.width
	if w < 20 {
		w = 80
	}

	iw := w - 2 // inner width (excluding │ borders)

	// header
	s := m.renderHeader("Projects", m.projectCursor+1, len(m.projects)) + "\n"
	s += borderStyle.Render("┌" + strings.Repeat("─", iw) + "┐") + "\n"

	if len(m.projects) == 0 {
		s += borderedRow("  No projects with Makefiles found.", iw, normalRowStyle) + "\n"
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
		s += borderedRow(hdr, iw, colHeaderStyle) + "\n"
		s += borderStyle.Render("├" + strings.Repeat("─", iw) + "┤") + "\n"

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
				style = selectedRowStyle
			} else if i%2 == 0 {
				style = altRowStyle
			} else {
				style = normalRowStyle
			}
			s += borderedRow(line, iw, style) + "\n"
		}
	}

	// pad to fill screen with bordered empty rows
	lines := strings.Count(s, "\n")
	for i := lines; i < m.height-3; i++ {
		s += borderedRow("", iw, normalRowStyle) + "\n"
	}

	s += borderStyle.Render("└" + strings.Repeat("─", iw) + "┘") + "\n"
	s += m.renderHintBar([][]string{
		{"Enter", "Drill In"},
		{"g", "Pull"},
		{"?", "Readme"},
		{"q", "Quit"},
	})
	return s
}

func (m model) renderTargetList() string {
	proj := m.projects[m.selectedProject]
	w := m.width
	if w < 20 {
		w = 80
	}

	iw := w - 2

	// header
	s := m.renderHeader(proj.Name, m.targetCursor+1, len(proj.Targets)) + "\n"
	s += borderStyle.Render("┌" + strings.Repeat("─", iw) + "┐") + "\n"

	if len(proj.Targets) == 0 {
		s += borderedRow("  No targets found.", iw, normalRowStyle) + "\n"
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
		s += borderedRow(hdr, iw, colHeaderStyle) + "\n"
		s += borderStyle.Render("├" + strings.Repeat("─", iw) + "┤") + "\n"

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
				style = selectedRowStyle
			} else if i%2 == 0 {
				style = altRowStyle
			} else {
				style = normalRowStyle
			}
			s += borderedRow(line, iw, style) + "\n"
		}
	}

	// pad to fill screen with bordered empty rows
	lines := strings.Count(s, "\n")
	for i := lines; i < m.height-3; i++ {
		s += borderedRow("", iw, normalRowStyle) + "\n"
	}

	s += borderStyle.Render("└" + strings.Repeat("─", iw) + "┘") + "\n"

	hints := [][]string{
		{"r", "Run"},
		{"g", "Pull"},
		{"?", "Readme"},
		{"Esc", "Back"},
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
