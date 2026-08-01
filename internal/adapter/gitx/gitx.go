// Package gitx is the git adapter: it reads a repository's state through
// short, captured, read-only commands.
//
// Per ADR-M003 the distinction MkX draws is mutating vs reading, not git vs
// make. Mutating commands — `git pull`, `git checkout` — own the terminal and
// run through the handover path in ui/tui. Reading commands return a value and
// never touch the terminal; those are what this package runs.
//
// This file holds the impure edges: spawning git and composing the three reads.
// The classification and parsing are in parse.go and are pure.
package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// Reader reads git state through the git binary. Construct it with NewReader.
//
// Per ADR-M003 MkX deliberately does not use go-git: the git binary is the
// same source of truth the user's other tooling reads, and a second one can
// disagree with it.
type Reader struct{}

var _ app.GitReader = (*Reader)(nil)

// NewReader returns a Reader.
func NewReader() *Reader {
	return &Reader{}
}

// State reports the state of the repository containing dir.
//
// dir need not be a repository root: git's own upward .git discovery resolves
// every command below to the containing repository, which is what satisfies
// the Domain Model's containing-repository rule. There is no root-walking code
// here and no `rev-parse --show-toplevel` call — it would be a fourth
// subprocess computing a value nothing consumes.
//
// It returns app.ErrNotARepository when dir is inside no repository. Any other
// error means the read failed; the caller renders that as unknown, never as
// clean.
//
// The caller owns the deadline. State spawns nothing once ctx is done.
func (r *Reader) State(ctx context.Context, dir string) (domain.RepoState, error) {
	head := classifyHead(run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD"))

	// rev-parse gates the other two. If there is no repository, or the read
	// failed, running status and branch would spawn two more processes to
	// produce output that is already known to be unusable.
	switch head.kind {
	case headAbsent:
		return domain.RepoState{}, app.ErrNotARepository
	case headFailed:
		return domain.RepoState{}, head.err
	}

	// Unborn deliberately does not gate: status and branch both work on a
	// repository with no commits, and both are meaningful there.

	status := run(ctx, dir, "status", "--porcelain")
	if err := status.failure(); err != nil {
		// Checked rather than ignored. An unchecked failure here returns empty
		// output, and parseDirty("") would fabricate Dirty:false — the
		// "absence reads as clean" bug relocated one call deeper.
		return domain.RepoState{}, fmt.Errorf("git status: %w", err)
	}

	branch := run(ctx, dir, "branch")
	if err := branch.failure(); err != nil {
		return domain.RepoState{}, fmt.Errorf("git branch: %w", err)
	}

	return domain.RepoState{
		Head:     head.head,
		Branch:   head.branch,
		Dirty:    parseDirty(status.stdout),
		Branches: parseBranches(branch.stdout),
	}, nil
}

// result is one captured git subprocess. It is a plain struct with no
// behaviour tied to exec, so parse.go's classifiers can be driven from a table
// of literals rather than from a real git run.
type result struct {
	stdout   string
	stderr   string
	exitCode int
	ctxErr   error
}

// failure returns a non-nil error when the command did not complete
// successfully. It is the check for the two reads whose output is only
// meaningful on success; rev-parse's richer outcomes go through classifyHead.
func (r result) failure() error {
	if r.ctxErr != nil {
		return r.ctxErr
	}
	if r.exitCode != 0 {
		return fmt.Errorf("exit %d: %s", r.exitCode, firstLine(r.stderr))
	}
	return nil
}

// run captures one git command against dir.
//
// Everything about the invocation is deliberate:
//
//   - --no-optional-locks and -C are top-level git options and must precede the
//     subcommand; placed after it, git rejects them outright.
//   - --no-optional-locks stops `git status` opportunistically refreshing and
//     locking the index. That converts "may block on another process's
//     index.lock" into "never attempts the lock" — a stronger guarantee than
//     the timeout, which would only bound the wait.
//   - LC_ALL/LANG pin git's fatals to English, which classifyHead matches on.
//     This matters more here than in makex: without it, a non-English git
//     silently degrades both absent and unborn to unknown.
//   - GIT_TERMINAL_PROMPT=0 enforces ADR-M003's non-interactive constraint.
//     None of the three reads touch a remote, so it costs nothing today; it is
//     there so a future edit that does cannot hang on a credential prompt.
//
// stdout and stderr go to buffers, never to the process's own — these are
// captured reads, never handover.
func run(ctx context.Context, dir string, args ...string) result {
	argv := append([]string{"--no-optional-locks", "-C", dir}, args...)

	cmd := exec.CommandContext(ctx, "git", argv...)
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"GIT_TERMINAL_PROMPT=0",
	)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	res := result{stdout: stdout.String(), stderr: stderr.String()}

	// The context is consulted directly rather than inferred from err. A
	// killed process surfaces as a plain *ExitError, so exit status alone
	// cannot tell a timeout apart from a repository that is genuinely broken.
	if ctxErr := ctx.Err(); ctxErr != nil {
		res.ctxErr = ctxErr
		return res
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.exitCode = exitErr.ExitCode()
		} else {
			// git could not be started at all — not on PATH, or not
			// executable. Not a repository state; a failed read.
			res.exitCode = -1
			if res.stderr == "" {
				res.stderr = err.Error()
			}
		}
	}

	return res
}
