// Package app holds MkX's use cases and the port interfaces they call through.
// It imports domain only — never an adapter, never the UI.
package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// gitReadTimeout bounds one whole RepoState read — all three git commands,
// not one deadline each. ADR-M003 requires captured reads to be bounded; the
// AC asks for "a context with a timeout", singular. When the budget runs out
// mid-sequence the next command fails immediately rather than spawning.
//
// A warm local read of these commands is single-digit milliseconds. A cold or
// very large working tree can push `status --porcelain` into the hundreds,
// occasionally past a second on networked storage. The index-lock hazard is
// eliminated by --no-optional-locks in the adapter rather than budgeted for
// here. Two seconds is roughly two orders of magnitude above a warm read and
// still short enough that a genuinely stuck read resolves while the user is
// looking at the view.
//
// Unexported and unconfigurable, per ADR-M004.
const gitReadTimeout = 2 * time.Second

// App composes the ports into MkX's use cases. Constructed in cmd/mkx/main.go.
type App struct {
	scanner WorkspaceScanner
	runner  MakeRunner
	git     GitReader
}

// New wires the adapters behind their ports.
func New(scanner WorkspaceScanner, runner MakeRunner, git GitReader) *App {
	return &App{scanner: scanner, runner: runner, git: git}
}

// DiscoverProjects walks the workspace, discovers each project's targets, and
// returns the projects ordered by name, case-insensitively.
//
// A project whose target discovery fails still appears, with no targets — a
// broken Makefile hides that project's targets, it does not fail the scan.
func (a *App) DiscoverProjects(ctx context.Context, root string) ([]domain.Project, error) {
	projects, err := a.scanner.Scan(ctx, root)

	for i := range projects {
		targets, _ := a.runner.Discover(ctx, projects[i].Path)
		projects[i].Targets = targets
	}

	// order projects alphabetically by name, case-insensitive
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	return projects, err
}

// RunTarget returns the command that runs the named target in the given
// project. It decides and validates; the UI hands the result to the terminal.
func (a *App) RunTarget(ctx context.Context, project domain.Project, targetName string) (domain.Command, error) {
	for _, t := range project.Targets {
		if t.Name == targetName {
			return a.runner.TargetCommand(ctx, project.Path, targetName), nil
		}
	}
	return domain.Command{}, fmt.Errorf("project %q has no target %q", project.Name, targetName)
}

// PullAndRefresh returns the command that pulls the project's git repository.
//
// It is the first half of the pull-and-refresh action: the command runs with
// terminal handover, and the UI calls RefreshTargets when control returns.
// Per ADR-M003 a handover may have changed the Makefile, so what was
// discovered before it is stale.
func (a *App) PullAndRefresh(ctx context.Context, project domain.Project) (domain.Command, error) {
	return domain.Command{Argv: []string{"git", "pull"}, WorkDir: project.Path}, nil
}

// RefreshTargets re-discovers a project's targets after a handover.
func (a *App) RefreshTargets(ctx context.Context, project domain.Project) ([]domain.Target, error) {
	return a.runner.Discover(ctx, project.Path)
}

// RepoState reads the git state of the repository containing the project.
//
// It returns ErrNotARepository when the project is in no repository — an
// absence, not a failure. Any other error means the read did not produce a
// usable answer and the caller must render unknown, never clean.
//
// The deadline is applied here rather than in the adapter for two reasons.
// Bounding a read is a policy, and app is the port's only caller; and it is
// the only placement a test can assert, since a fake GitReader can check
// ctx.Deadline() where an adapter-side timeout would need a deliberately slow
// subprocess to exercise.
func (a *App) RepoState(ctx context.Context, project domain.Project) (domain.RepoState, error) {
	ctx, cancel := context.WithTimeout(ctx, gitReadTimeout)
	defer cancel()
	return a.git.State(ctx, project.Path)
}

// ReadmePath returns the project's README path, or "" when it has none.
func (a *App) ReadmePath(ctx context.Context, project domain.Project) string {
	return a.scanner.ReadmePath(ctx, project.Path)
}
