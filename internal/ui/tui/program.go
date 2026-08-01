package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// NewProgram builds the Bubble Tea program around an already-discovered set of
// projects.
//
// Discovery runs in main, before the program starts, because a failed scan
// must exit rather than open an empty TUI. Reconciling that with the
// Init()-fetch shape in the TUI Go Conventions baseline is deferred — it would
// be a behaviour change, and TAE-46 is behaviour-preserving.
func NewProgram(rootCtx context.Context, application *app.App, workspace string, projects []domain.Project) *tea.Program {
	m := Model{
		app:       application,
		rootCtx:   rootCtx,
		workspace: workspace,
		projects:  projects,
		view:      viewProjects,
	}
	return tea.NewProgram(m, tea.WithAltScreen())
}
