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

// PhonyStatus is whether a Target is a phony target — declared as a
// prerequisite of `.PHONY:` rather than naming a file it builds.
//
// Per ADR-M002 the zero value is PhonyUnknown, not PhonyNo: a discovery path
// that cannot determine phony status reports the honest answer by construction
// rather than by remembering to set the field.
type PhonyStatus int

const (
	// PhonyUnknown means the discovery path could not determine phony status.
	PhonyUnknown PhonyStatus = iota
	// PhonyNo means the target is not declared phony.
	PhonyNo
	// PhonyYes means the target is declared phony.
	PhonyYes
)

// String renders the status for display and for the characterization golden.
func (p PhonyStatus) String() string {
	switch p {
	case PhonyNo:
		return "no"
	case PhonyYes:
		return "yes"
	default:
		return "unknown"
	}
}

// Target is one Make target in a Project's Makefile. Description comes from the
// `## comment` convention, and is empty when the target carries none.
type Target struct {
	Name        string
	Description string

	// Phony is PhonyUnknown on any discovery path that cannot determine it.
	// Per ADR-M002, consumers must not treat PhonyUnknown and PhonyNo as
	// equivalent.
	Phony PhonyStatus

	// Prerequisites is make's resolved prerequisite list, and is meaningful
	// only when Phony != PhonyUnknown.
	//
	// nil carries two distinct meanings that share one representation: on the
	// database path it means the target has no prerequisites; on the regex
	// fallback path, which does not populate this field at all, it means
	// unknown. Phony is what distinguishes them — PhonyUnknown implies the
	// fallback path and therefore that Prerequisites is unknown too.
	Prerequisites []string
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
