package app

import (
	"context"

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
