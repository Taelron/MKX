package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// The view keymaps. Each is the single declaration of what its view does:
// what the hint bar advertises, what the help overlay documents, and what a
// key press actually runs.
//
// `r` appears in neither. @UI Patterns reserves it for refresh across every
// Taelron TUI, and a list backed by stable local data — MkX parses Make
// targets once at startup — omits it rather than binding it to a no-op.

func projectKeymap() keymap {
	return keymap{
		{keys: []string{"up", "k"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor up", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if m.projectCursor > 0 {
					m.projectCursor--
				}
				return m, nil
			}},
		{keys: []string{"down", "j"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor down", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if m.projectCursor < len(m.projects)-1 {
					m.projectCursor++
				}
				return m, nil
			}},
		{keys: []string{"enter"}, display: "Enter", label: "Targets",
			help: "Open the project's target list", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if len(m.projects) > 0 {
					m.selectedProject = m.projectCursor
					m.targetCursor = 0
					m.view = viewTargets
				}
				return m, nil
			}},
		{keys: []string{"g"}, display: "g", label: "Pull",
			help: "Git pull and re-read targets", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if len(m.projects) == 0 {
					return m, nil
				}
				return m, m.pull(m.projectCursor)
			}},
		{keys: []string{"R"}, display: "R", label: "Readme",
			help: "Show the project's README", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if len(m.projects) == 0 {
					return m, nil
				}
				return m, viewReadme(m.app.ReadmePath(m.rootCtx, m.projects[m.projectCursor]))
			}},
		{keys: []string{"q", "ctrl+c"}, display: "q", label: "Quit",
			help: "Quit mkx", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				return m, tea.Quit
			}},
		{keys: []string{"?"}, display: "?", label: "Help",
			help: "Keys available in this view", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				return m.showModal(modalHelp, "Help", "Projects", modalInput{}, helpKeymap()), nil
			}},
	}
}

func targetKeymap() keymap {
	return keymap{
		{keys: []string{"up", "k"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor up", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if m.targetCursor > 0 {
					m.targetCursor--
				}
				return m, nil
			}},
		{keys: []string{"down", "j"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor down", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				proj, ok := m.currentProject()
				if ok && m.targetCursor < len(proj.Targets)-1 {
					m.targetCursor++
				}
				return m, nil
			}},
		{keys: []string{"enter"}, display: "Enter", label: "Run",
			help: "Run the selected target", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				proj, ok := m.currentProject()
				if !ok || len(proj.Targets) == 0 {
					return m, nil
				}
				return m, m.runTarget(proj, proj.Targets[m.targetCursor])
			}},
		{keys: []string{"g"}, display: "g", label: "Pull",
			help: "Git pull and re-read targets", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				if _, ok := m.currentProject(); !ok {
					return m, nil
				}
				return m, m.pull(m.selectedProject)
			}},
		{keys: []string{"R"}, display: "R", label: "Readme",
			help: "Show the project's README", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				proj, ok := m.currentProject()
				if !ok {
					return m, nil
				}
				return m, viewReadme(m.app.ReadmePath(m.rootCtx, proj))
			}},
		{keys: []string{"esc"}, display: "Esc", label: "Back",
			help: "Back to the project list", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				m.view = viewProjects
				m.lastRun = nil
				return m, nil
			}},
		// Help-only: the hint bar's last two slots are the prescribed
		// Esc/Back  ?/Help, so q does not claim one.
		{keys: []string{"q", "ctrl+c"}, display: "q", label: "Quit",
			help: "Quit mkx",
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				return m, tea.Quit
			}},
		{keys: []string{"?"}, display: "?", label: "Help",
			help: "Keys available in this view", inBar: true,
			handler: func(m Model, _ string) (Model, tea.Cmd) {
				proj, _ := m.currentProject()
				return m.showModal(modalHelp, "Help", proj.Name, modalInput{}, helpKeymap()), nil
			}},
	}
}

// helpKeymap is the help overlay's own bindings — the keys that reach the
// modal while it is up, and the modal's hint bar.
func helpKeymap() keymap {
	closeHelp := func(m Model, _ string) (Model, tea.Cmd) {
		return m.closeModal(), nil
	}
	return keymap{
		{keys: []string{"esc"}, display: "Esc", label: "Close",
			help: "Close this overlay", inBar: true, handler: closeHelp},
		{keys: []string{"?"}, display: "?", label: "Close",
			help: "Close this overlay", inBar: true, handler: closeHelp},
	}
}
