package app

import (
	"context"
	"errors"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// WorkspaceScanner reads the filesystem: it finds the projects under a
// workspace root and locates their README files.
type WorkspaceScanner interface {
	// Scan walks root and returns every directory that holds a Makefile.
	// Targets are not populated — DiscoverProjects composes them in.
	Scan(ctx context.Context, root string) ([]domain.Project, error)

	// ReadmePath returns the README path for dir, or "" when there is none.
	ReadmePath(ctx context.Context, dir string) string
}

// MakeRunner reads Makefiles: it discovers a project's targets and builds the
// command that runs one.
type MakeRunner interface {
	// Discover returns the targets declared in dir's Makefile.
	Discover(ctx context.Context, dir string) ([]domain.Target, error)

	// TargetCommand returns the command that runs the named target in dir.
	TargetCommand(ctx context.Context, dir, name string) domain.Command
}

// ErrNotARepository is returned by GitReader.State when dir is not inside a
// git repository at all.
//
// Per the MkX Domain Model this is an absence, not a failure: a Project
// outside a repository simply has no RepoState. It is declared here rather
// than in the adapter because ui/tui needs errors.Is on it, and ui/tui must
// not import an adapter (ADR-M001).
var ErrNotARepository = errors.New("not a git repository")

// GitReader reads git state. It never mutates — per ADR-M003 pull and
// checkout are handovers, not reads.
type GitReader interface {
	// State resolves dir to the repository containing it — dir need not be a
	// repository root — and reports that repository's state.
	//
	// It returns ErrNotARepository when dir is inside no repository. Any
	// other error means the read failed and the caller treats the state as
	// unknown; it never means the tree is clean.
	State(ctx context.Context, dir string) (domain.RepoState, error)
}
