package gitx

// Table tests for the pure classification core. Per TAE-58's acceptance
// criteria the adapter logic is tested against captured command output as a
// pure function, not by invoking git: nothing in this file spawns a process,
// touches the filesystem, or needs a repository to point at. Every input is a
// literal, visible in the test.
//
// The captured strings below were taken from git 2.47.3 under LC_ALL=C, which
// is the locale gitx.go pins. Two of them are the reason this file exists:
// detached and unborn both print "HEAD" on stdout and differ only in the exit
// code.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// Real stderr text from git 2.47.3, kept verbatim — including the trailing
// advice lines — because classifyHead matches on substrings of it and a
// hand-tidied sample would not prove the match survives the real output.
const (
	stderrNoRepo = `fatal: not a git repository (or any parent up to mount point /)
Stopping at filesystem boundary (GIT_DISCOVERY_ACROSS_FILESYSTEM not set).
`
	// The other phrasing, emitted when discovery stops for a reason other than
	// a mount boundary. Both must classify as absent.
	stderrNoRepoShort = "fatal: not a git repository: /w/broken/garbage\n"

	stderrUnborn = `fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.
Use '--' to separate paths from revisions, like this:
'git <command> [<revision>...] -- [<file>...]'
`
	stderrMissingDir  = "fatal: cannot change to '/w/gone': No such file or directory\n"
	stderrInvalidFile = "fatal: invalid gitfile format: /w/broken/.git\n"
)

func TestClassifyHead(t *testing.T) {
	tests := []struct {
		name string
		in   result

		wantKind   headKind
		wantHead   domain.Head
		wantBranch string
	}{
		{
			name:       "on a branch",
			in:         result{stdout: "main\n"},
			wantKind:   headPresent,
			wantHead:   domain.HeadOnBranch,
			wantBranch: "main",
		},
		{
			name:       "branch name with slashes is not mangled",
			in:         result{stdout: "gaetanjaminon/tae-58-git-state\n"},
			wantKind:   headPresent,
			wantHead:   domain.HeadOnBranch,
			wantBranch: "gaetanjaminon/tae-58-git-state",
		},
		{
			// git 2.47.3: exit 0, stdout "HEAD". The exit code is the ONLY
			// thing separating this from the unborn case below.
			name:     "detached HEAD is detached, not a branch named HEAD",
			in:       result{stdout: "HEAD\n"},
			wantKind: headPresent,
			wantHead: domain.HeadDetached,
		},
		{
			// git 2.47.3: exit 128, stdout "HEAD", ambiguous-argument stderr.
			// The regression test for the single most load-bearing finding in
			// the design: a classifier reading stdout before the exit code
			// reports an empty repository as detached.
			name: "unborn HEAD is unborn, not detached, despite the same stdout",
			in: result{
				stdout:   "HEAD\n",
				stderr:   stderrUnborn,
				exitCode: 128,
			},
			wantKind: headPresent,
			wantHead: domain.HeadUnborn,
		},
		{
			name:     "not a repository, mount-boundary phrasing",
			in:       result{stderr: stderrNoRepo, exitCode: 128},
			wantKind: headAbsent,
		},
		{
			name:     "not a repository, path phrasing",
			in:       result{stderr: stderrNoRepoShort, exitCode: 128},
			wantKind: headAbsent,
		},
		{
			// Exit 128 is necessary but never sufficient — git uses it for
			// every fatal. An unmatched 128 must land in failed rather than
			// being guessed at as absent.
			name:     "missing directory is a failed read, not an absent repo",
			in:       result{stderr: stderrMissingDir, exitCode: 128},
			wantKind: headFailed,
		},
		{
			name:     "invalid gitfile is a failed read, not an absent repo",
			in:       result{stderr: stderrInvalidFile, exitCode: 128},
			wantKind: headFailed,
		},
		{
			name:     "git could not be started",
			in:       result{stderr: `exec: "git": executable file not found in $PATH`, exitCode: -1},
			wantKind: headFailed,
		},
		{
			name:     "timeout",
			in:       result{ctxErr: context.DeadlineExceeded},
			wantKind: headFailed,
		},
		{
			// The trap case. A context-killed process may still have written
			// an exit code and a stderr line, and neither is evidence about
			// the repository. If the stderr rules ran first this would report
			// a perfectly fine repository as absent and stop MkX ever
			// retrying it. Proves the ctx-first rule rather than assuming it.
			name: "context error wins over a not-a-repository stderr",
			in: result{
				stderr:   stderrNoRepo,
				exitCode: 128,
				ctxErr:   context.DeadlineExceeded,
			},
			wantKind: headFailed,
		},
		{
			name: "context error wins over a successful exit",
			in: result{
				stdout: "main\n",
				ctxErr: context.Canceled,
			},
			wantKind: headFailed,
		},
		{
			// Not a state git produces. Taking it as a branch named "" would
			// render as a clean repository on a nameless branch — exactly the
			// "absence reads as clean" failure the AC forbids.
			name:     "exit 0 with empty output is a failed read",
			in:       result{stdout: "\n"},
			wantKind: headFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyHead(tt.in)

			if got.kind != tt.wantKind {
				t.Fatalf("kind = %v, want %v (outcome %+v)", got.kind, tt.wantKind, got)
			}
			if got.head != tt.wantHead {
				t.Errorf("head = %v, want %v", got.head, tt.wantHead)
			}
			if got.branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", got.branch, tt.wantBranch)
			}

			// A failed outcome must carry a reason: it is what the caller
			// wraps and what a future diagnostic surface would show.
			if tt.wantKind == headFailed && got.err == nil {
				t.Errorf("kind is headFailed but err is nil")
			}
			if tt.wantKind != headFailed && got.err != nil {
				t.Errorf("err = %v on a %v outcome, want nil", got.err, tt.wantKind)
			}
		})
	}
}

// TestClassifyHeadPreservesContextError checks the timeout is not merely
// reported as some error, but as one the caller can still test with
// errors.Is — the TUI discards a cancellation silently and shows a timeout,
// and it cannot tell them apart from an error string.
func TestClassifyHeadPreservesContextError(t *testing.T) {
	for _, want := range []error{context.DeadlineExceeded, context.Canceled} {
		got := classifyHead(result{ctxErr: want})
		if !errors.Is(got.err, want) {
			t.Errorf("classifyHead(ctxErr=%v).err = %v, want it to wrap %v", want, got.err, want)
		}
	}
}

// TestClassifyHeadNeverReportsUnknownHeadAsPresent pins the invariant the
// display layer relies on: a present outcome always names a real head state,
// so the renderer never has to treat domain.HeadUnknown as "we did read this".
func TestClassifyHeadNeverReportsUnknownHeadAsPresent(t *testing.T) {
	inputs := []result{
		{stdout: "main\n"},
		{stdout: "HEAD\n"},
		{stdout: "HEAD\n", stderr: stderrUnborn, exitCode: 128},
	}
	for _, in := range inputs {
		got := classifyHead(in)
		if got.kind == headPresent && got.head == domain.HeadUnknown {
			t.Errorf("classifyHead(%+v) reported present with HeadUnknown", in)
		}
	}
}

func TestParseDirty(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "clean tree produces no output", in: "", want: false},
		{name: "whitespace only", in: "\n  \n\t\n", want: false},
		{name: "modified", in: " M internal/app/ports.go\n", want: true},
		{name: "staged", in: "M  internal/domain/domain.go\n", want: true},
		{name: "untracked only", in: "?? internal/adapter/gitx/\n", want: true},
		{name: "deleted", in: " D README.md\n", want: true},
		{name: "renamed", in: "R  old.go -> new.go\n", want: true},
		{
			name: "several lines",
			in: ` M internal/app/ports.go
M  internal/domain/domain.go
?? internal/adapter/gitx/
`,
			want: true,
		},
		{
			name: "no trailing newline",
			in:   " M internal/app/ports.go",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseDirty(tt.in); got != tt.want {
				t.Errorf("parseDirty(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseBranches(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "plain list with the current-branch marker stripped",
			in: `  develop
* main
  gaetanjaminon/tae-58
`,
			want: []string{"develop", "main", "gaetanjaminon/tae-58"},
		},
		{
			// git 2.47.3 emits this pseudo-entry alongside the real branches
			// when HEAD is detached. It is not a branch and must not appear in
			// a list offered for selection.
			name: "detached pseudo-entry dropped, real branches kept",
			in: `* (HEAD detached at 9d77cf6)
  main
  develop
`,
			want: []string{"main", "develop"},
		},
		{
			name: "older no-branch phrasing dropped",
			in: `* (no branch)
  main
`,
			want: []string{"main"},
		},
		{
			name: "worktree marker stripped like any other",
			in: `+ feature/wt
* main
`,
			want: []string{"feature/wt", "main"},
		},
		{
			// A repository with no commits yet. Zero branches is the correct
			// answer here, not a failed read — the caller distinguishes the
			// two by the head outcome, never by this being empty.
			name: "empty output yields no branches",
			in:   "",
			want: nil,
		},
		{
			name: "blank lines ignored",
			in:   "  main\n\n  develop\n",
			want: []string{"main", "develop"},
		},
		{
			name: "no trailing newline",
			in:   "* main",
			want: []string{"main"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBranches(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseBranches(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseBranches(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestParseBranchesKeepsNamesUntrimmedOfContent guards the two-character strip
// against eating a branch name. The marker column is exactly two characters;
// a name is never shortened by it.
func TestParseBranchesKeepsNamesUntrimmedOfContent(t *testing.T) {
	const name = "ab"
	got := parseBranches("  " + name + "\n")
	if len(got) != 1 || got[0] != name {
		t.Errorf("parseBranches lost content: got %v, want [%q]", got, name)
	}
}

// TestResultFailure covers the check that stops a timed-out `git status`
// returning empty output and being parsed as a clean tree.
func TestResultFailure(t *testing.T) {
	tests := []struct {
		name    string
		in      result
		wantErr bool
	}{
		{name: "success", in: result{stdout: " M x.go\n"}, wantErr: false},
		{name: "clean success", in: result{}, wantErr: false},
		{name: "non-zero exit", in: result{exitCode: 128, stderr: stderrMissingDir}, wantErr: true},
		{name: "timeout with empty output", in: result{ctxErr: context.DeadlineExceeded}, wantErr: true},
		{
			// The specific bug this method exists to prevent: a killed read
			// looks exactly like a clean tree if only stdout is consulted.
			name:    "timeout that also exited zero",
			in:      result{stdout: "", ctxErr: context.DeadlineExceeded},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.failure()
			if (err != nil) != tt.wantErr {
				t.Errorf("failure() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestFirstLine checks the error-string helper drops git's multi-line advice.
func TestFirstLine(t *testing.T) {
	if got := firstLine(stderrNoRepo); got != "fatal: not a git repository (or any parent up to mount point /)" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine("\n\n  fatal: x\n"); got != "fatal: x" {
		t.Errorf("firstLine skipping blanks = %q", got)
	}
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine(\"\") = %q, want empty", got)
	}
}

// TestNoGitInvokedByParsers is a non-vacuity check on the claim this file
// makes in its header. It does not attempt to intercept exec — it asserts the
// weaker, verifiable thing: that the pure file declares no dependency capable
// of spawning a process.
func TestNoGitInvokedByParsers(t *testing.T) {
	src, err := os.ReadFile("parse.go")
	if err != nil {
		t.Fatalf("reading parse.go: %v", err)
	}
	for _, forbidden := range []string{`"os/exec"`, `"os"`} {
		if strings.Contains(string(src), forbidden) {
			t.Errorf("parse.go imports %s; the parsing half must stay pure", forbidden)
		}
	}
}
