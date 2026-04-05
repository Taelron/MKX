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

func (m model) renderProjectList() string {
	s := headerStyle.Render("mkx") + "\n\n"

	if len(m.projects) == 0 {
		s += "  No projects with Makefiles found.\n"
	} else {
		// dynamic column width from longest project name
		colWidth := len("PROJECT")
		for _, p := range m.projects {
			if len(p.Name) > colWidth {
				colWidth = len(p.Name)
			}
		}
		colWidth += 4

		s += fmt.Sprintf("  %-*s %s\n", colWidth, "PROJECT", "TARGETS")
		s += "  " + strings.Repeat("─", colWidth+10) + "\n"

		// scrollable viewport: 5 lines reserved for header + hint bar
		maxVisible := m.height - 5
		if maxVisible < 1 {
			maxVisible = 1
		}

		offset := 0
		if m.projectCursor >= maxVisible {
			offset = m.projectCursor - maxVisible + 1
		}
		end := offset + maxVisible
		if end > len(m.projects) {
			end = len(m.projects)
		}

		for i := offset; i < end; i++ {
			p := m.projects[i]
			cursor := "  "
			if i == m.projectCursor {
				cursor = "> "
			}
			line := fmt.Sprintf("%s%-*s %d", cursor, colWidth, p.Name, len(p.Targets))
			if i == m.projectCursor {
				s += selectedStyle.Render(line) + "\n"
			} else {
				s += line + "\n"
			}
		}
	}

	// pad to fill screen
	lines := strings.Count(s, "\n")
	for i := lines; i < m.height-2; i++ {
		s += "\n"
	}

	hint := "  Enter drill in   ? readme   q quit"
	if m.flash != "" {
		hint += "   " + m.flash
	}
	s += hintBarStyle.Render(hint)
	return s
}

func (m model) renderTargetList() string {
	proj := m.projects[m.selectedProject]

	s := headerStyle.Render("mkx › "+proj.Name) + "\n\n"

	if len(proj.Targets) == 0 {
		s += "  No targets found.\n"
	} else {
		colWidth := len("TARGET")
		for _, t := range proj.Targets {
			if len(t.Name) > colWidth {
				colWidth = len(t.Name)
			}
		}
		colWidth += 4

		s += fmt.Sprintf("  %-*s %s\n", colWidth, "TARGET", "DESCRIPTION")
		s += "  " + strings.Repeat("─", colWidth+30) + "\n"

		maxVisible := m.height - 5
		if maxVisible < 1 {
			maxVisible = 1
		}

		offset := 0
		if m.targetCursor >= maxVisible {
			offset = m.targetCursor - maxVisible + 1
		}
		end := offset + maxVisible
		if end > len(proj.Targets) {
			end = len(proj.Targets)
		}

		for i := offset; i < end; i++ {
			t := proj.Targets[i]
			cursor := "  "
			if i == m.targetCursor {
				cursor = "> "
			}
			desc := t.Description
			if desc == "" {
				desc = "-"
			}
			line := fmt.Sprintf("%s%-*s %s", cursor, colWidth, t.Name, desc)
			if i == m.targetCursor {
				s += selectedStyle.Render(line) + "\n"
			} else {
				s += line + "\n"
			}
		}
	}

	// pad to fill screen
	lines := strings.Count(s, "\n")
	for i := lines; i < m.height-2; i++ {
		s += "\n"
	}

	hint := "  / filter   r run   ? readme   Esc back"
	if m.lastRun != nil {
		status := "✓"
		if m.lastRun.ExitCode != 0 {
			status = fmt.Sprintf("✗ exit %d", m.lastRun.ExitCode)
		}
		hint += fmt.Sprintf("   %s %s", status, m.lastRun.Duration.Round(time.Second))
	}
	s += hintBarStyle.Render(hint)
	return s
}
