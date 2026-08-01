// Package domain holds MkX's entities and value types. It imports nothing
// project-specific — no Bubble Tea, no filesystem, no exec.
//
// See ADR-M001 for the layer contract.
package domain

// Project is a directory in the workspace that has a Makefile.
//
// Name is the workspace-relative path and is the project's identity; Path is
// the absolute location on disk.
type Project struct {
	Name        string
	Path        string
	Description string
	Targets     []Target
}

// Target is one Make target in a Project's Makefile. Description comes from the
// `## comment` convention, and is empty when the target carries none.
type Target struct {
	Name        string
	Description string
}

// Command describes a subprocess to run with full terminal handover.
//
// Per ADR-M001 and ADR-M003, app use cases decide what to run and return a
// Command; ui/tui executes it via tea.Exec. The decision is testable without a
// terminal, and the handover stays where the framework is.
type Command struct {
	Argv    []string
	WorkDir string
}
