package gitx

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// This file holds the parsing half of the git adapter. Every function here is
// pure: it takes one captured `result` and returns values, running no
// subprocess and reading no file. The impure edge — spawning git — lives in
// gitx.go, so the classifiers are table-testable without git on the box and
// without a repository to point them at (ADR-M003, and the same split ADR-M002
// established for makex).

// headKind is what a `git rev-parse --abbrev-ref HEAD` read established about
// the directory it was run in.
type headKind int

const (
	// headFailed means the read gave no usable answer: it timed out, was
	// cancelled, or git failed for a reason that is not "no repository here".
	// The state is unknown, which is emphatically not "clean".
	headFailed headKind = iota
	// headAbsent means the directory is inside no git repository. Per the MkX
	// Domain Model this is an absence, not a failure.
	headAbsent
	// headPresent means a repository was found and its HEAD resolved.
	headPresent
)

// headOutcome is classifyHead's result: which of the three kinds above, plus
// the detail that kind carries.
type headOutcome struct {
	kind headKind

	// head is meaningful only when kind is headPresent. It is never
	// HeadUnknown there.
	head domain.Head

	// branch is set only when head is domain.HeadOnBranch.
	branch string

	// err is set only when kind is headFailed, and describes why.
	err error
}

// classifyHead turns one captured `git rev-parse --abbrev-ref HEAD` into a
// head outcome.
//
// The order of the rules is load-bearing twice over.
//
// ctxErr is checked first and wins unconditionally: a process the context
// killed may still have written an exit code and a stderr line, and neither is
// evidence about the repository.
//
// The exit code is checked before stdout because git prints the literal
// "HEAD" on stdout in BOTH the detached case (exit 0) and the unborn case
// (exit 128, with the ambiguous-argument fatal on stderr). A classifier that
// reads stdout first reports an empty repository as detached. Verified against
// git 2.47.3.
func classifyHead(r result) headOutcome {
	if r.ctxErr != nil {
		return headOutcome{kind: headFailed, err: fmt.Errorf("git rev-parse: %w", r.ctxErr)}
	}

	if r.exitCode == 0 {
		name := strings.TrimSpace(r.stdout)
		if name == "HEAD" {
			return headOutcome{kind: headPresent, head: domain.HeadDetached}
		}
		if name == "" {
			// Exit 0 with nothing to name the head is not a state git
			// produces; treating it as a branch called "" would render as a
			// clean repository on a nameless branch.
			return headOutcome{kind: headFailed, err: fmt.Errorf("git rev-parse: empty output")}
		}
		return headOutcome{kind: headPresent, head: domain.HeadOnBranch, branch: name}
	}

	// Below here git failed. Only two failures are known states rather than
	// broken reads, and both are matched on git's English text — which is why
	// gitx.go pins LC_ALL=C. Without the pin a non-English git degrades both
	// of these to unknown rather than mis-reporting them.
	stderr := r.stderr
	switch {
	case strings.Contains(stderr, "fatal: not a git repository"):
		return headOutcome{kind: headAbsent}
	case strings.Contains(stderr, "ambiguous argument 'HEAD'"):
		// A repository with no commits yet. It is a real repository: status
		// and branch both work there and both are meaningful.
		return headOutcome{kind: headPresent, head: domain.HeadUnborn}
	}

	// Everything else — a missing directory, an invalid gitfile, a permission
	// error, a git that is not on PATH. Exit 128 is necessary but never
	// sufficient: git uses it for every fatal, so an unmatched 128 lands here
	// rather than being guessed at.
	return headOutcome{kind: headFailed, err: fmt.Errorf("git rev-parse: exit %d: %s", r.exitCode, firstLine(stderr))}
}

// parseDirty reports whether `git status --porcelain` output shows any
// uncommitted change — modified, staged, or untracked alike.
//
// Any non-blank line is a change. The porcelain v1 format gives one line per
// path with a two-character status prefix, and prints nothing at all for a
// clean tree.
func parseDirty(porcelain string) bool {
	scanner := bufio.NewScanner(strings.NewReader(porcelain))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			return true
		}
	}
	return false
}

// parseBranches extracts the local branch names from
// `git branch --format=%(refname:short)` output.
//
// The format is the one ADR-M003 names, and it gives one bare branch name per
// line: no two-character marker column, so no column to strip and no marker
// character that could silently become part of a name.
//
// What --format does NOT remove is the pseudo-entry git synthesises when HEAD
// is not on a branch. Verified against git 2.47.3, both of these appear in
// --format output alongside the real branches:
//
//	(HEAD detached at c5020c5)
//	(no branch, bisect started on main)
//
// They are not branches and must never reach a list offered for selection, so
// they are dropped by shape rather than by matching either phrasing: a
// parenthesised entry containing a space. git's own refname rules forbid a
// space in a real branch name, so the rule cannot discard one.
//
// A repository with no commits yet produces no output at all, and therefore
// no branches. That is a correct empty list, not a failed read; the caller
// distinguishes the two by the head outcome, never by this being empty.
func parseBranches(out string) []string {
	var branches []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") && strings.Contains(name, " ") {
			continue
		}
		branches = append(branches, name)
	}
	return branches
}

// firstLine returns the first non-blank line of s, trimmed — enough to name
// what went wrong without pasting git's multi-line advice into an error.
func firstLine(s string) string {
	scanner := bufio.NewScanner(strings.NewReader(s))
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			return line
		}
	}
	return ""
}
