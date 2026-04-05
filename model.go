package main

import (
	"fmt"
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

	case execFinishedMsg:
		m.lastRun = &runResult{
			ExitCode: msg.exitCode,
			Duration: msg.duration,
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	}
	return m, nil
}

func (m model) runTarget(proj project, t target, args string) tea.Cmd {
	makeArgs := []string{"-C", proj.Path, t.Name}
	if args != "" {
		makeArgs = append(makeArgs, args)
	}
	c := exec.Command("make", makeArgs...)

	start := time.Now()

	return tea.ExecProcess(c, func(err error) tea.Msg {
		duration := time.Since(start)
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		return execFinishedMsg{
			exitCode: exitCode,
			duration: duration,
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
		s += fmt.Sprintf("  %-40s %s\n", "PROJECT", "TARGETS")
		s += "  " + strings.Repeat("─", 50) + "\n"

		for i, p := range m.projects {
			cursor := "  "
			if i == m.projectCursor {
				cursor = "> "
			}
			line := fmt.Sprintf("%s%-40s %d", cursor, p.Name, len(p.Targets))
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

	s += hintBarStyle.Render("  / filter   Enter drill in   ? readme   q quit")
	return s
}

func (m model) renderTargetList() string {
	proj := m.projects[m.selectedProject]

	s := headerStyle.Render("mkx › "+proj.Name) + "\n\n"

	if len(proj.Targets) == 0 {
		s += "  No targets found.\n"
	} else {
		s += fmt.Sprintf("  %-25s %s\n", "TARGET", "DESCRIPTION")
		s += "  " + strings.Repeat("─", 50) + "\n"

		for i, t := range proj.Targets {
			cursor := "  "
			if i == m.targetCursor {
				cursor = "> "
			}
			desc := t.Description
			if desc == "" {
				desc = "-"
			}
			line := fmt.Sprintf("%s%-25s %s", cursor, t.Name, desc)
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
