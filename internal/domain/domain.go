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

// Head is how a RepoState's HEAD resolved.
//
// It exists because `git rev-parse --abbrev-ref HEAD` prints the literal
// string "HEAD" for both a detached head and a repository with no commits
// yet — only the exit code separates them. Encoding either as a branch named
// "HEAD" would be indistinguishable from a real branch, so the state is
// carried as a discriminator rather than inferred from the branch name.
//
// The zero value is HeadUnknown, so a RepoState no read has filled in reads
// as unknown rather than as a clean repository on an empty-named branch. That
// is the "absence of a marker must not read as clean" rule enforced by the
// zero value rather than by every caller remembering it — the same discipline
// PhonyStatus applies.
type Head int

const (
	// HeadUnknown means no read has resolved this repository's HEAD.
	HeadUnknown Head = iota
	// HeadOnBranch means HEAD is on a branch, whose name is in Branch.
	HeadOnBranch
	// HeadDetached means HEAD points at a commit, not a branch.
	HeadDetached
	// HeadUnborn means the repository has no commits yet. Per the MkX Domain
	// Model this is a normal state, not an error: a freshly `git init`-ed
	// project must render without a marker that reads as failure.
	HeadUnborn
)

// String renders the head state for display and diagnostics.
func (h Head) String() string {
	switch h {
	case HeadOnBranch:
		return "on branch"
	case HeadDetached:
		return "detached"
	case HeadUnborn:
		return "unborn"
	default:
		return "unknown"
	}
}

// RepoState is a Project's git state, resolved against the repository
// containing the Project — which may sit above the Project's directory —
// never against the Project directory in isolation.
//
// It is optional per Project: a Project outside any git repository has none,
// and that is not an error condition. Per ADR-M003 it is read state, never
// assumed current across a terminal handover.
type RepoState struct {
	// Head is how HEAD resolved. Consumers branch on this, never on Branch
	// being empty.
	Head Head

	// Branch is the branch name, and is set only when Head is HeadOnBranch.
	// It never holds the literal "HEAD" and never holds a display marker —
	// markers are the UI's business.
	Branch string

	// Dirty is whether the working tree has any uncommitted change:
	// modified, staged, or untracked.
	Dirty bool

	// Branches is the repository's local branches, for selection. It is empty
	// for a repository with no commits yet.
	Branches []string
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
