//go:build unix

package gitx

// The one test in this package that spawns processes.
//
// TAE-62: the read deadline kills git, but Wait then blocks on the captured
// stdout pipe for as long as a descendant holds it open. The fix is
// cmd.WaitDelay; this is the proof that it is doing something, and the shape
// below is the one the M2 exit verification isolated the defect down to.
//
// The whole reproduction is one word. `exec sleep` replaces the shell, so the
// process the deadline kills is the only one holding the pipe and the read was
// already bounded without the fix. A bare `sleep` leaves a child behind: the
// shell is killed, the sleep is not, and it holds the write end. Both shapes
// run here, because a test that reddens for both is failing for some other
// reason and proves nothing about the mechanism — scripts/verify-waitdelay-guard.sh
// asserts exactly that asymmetry.
//
// unix-only: the reproduction needs /bin/sh and orphan semantics. The fix
// itself is platform-independent — WaitDelay is not a unix concept — so
// nothing about the behaviour is untested on Windows except this proof of it.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
)

const (
	// A short stand-in for app's two seconds. This test is about the shape of
	// the bound, not about its production value, and a process-spawning test
	// should not cost two seconds per case to make its point.
	testDeadline = 300 * time.Millisecond

	// Room for a loaded CI box on top of deadline + WaitDelay. It can be
	// generous: what it has to distinguish itself from is the shim's
	// 60-second sleep, so the margin is a factor of twelve rather than a few
	// milliseconds. A false failure needs a machine six times slower than the
	// budget already allows; a false pass would need a 60-second sleep to
	// finish early, which is not a thing that happens.
	slack = 4 * time.Second
)

func TestRunBoundsAKilledProcessWhoseDescendantHoldsStdout(t *testing.T) {
	cases := []struct {
		name string
		shim string
	}{
		{
			// Nothing survives the kill: the deadline alone bounds this, with
			// or without the fix. Here to prove the other case is not simply
			// red for an unrelated reason.
			name: "exec-sleep",
			shim: "#!/bin/sh\nexec sleep 60\n",
		},
		{
			// The defect. sh is killed at the deadline; the backgrounded sleep
			// is not, and it still holds the stdout pipe MkX is capturing
			// into. The pid is recorded so the test can clean up after itself
			// instead of leaving a minute-long orphan behind.
			name: "bare-sleep",
			shim: "#!/bin/sh\nsleep 60 &\necho $! > \"$MKX_TEST_PIDFILE\"\nwait\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			pidfile := filepath.Join(t.TempDir(), "descendant.pid")

			if err := os.WriteFile(filepath.Join(bin, "git"), []byte(tc.shim), 0o755); err != nil {
				t.Fatalf("write shim: %v", err)
			}

			t.Setenv("MKX_TEST_PIDFILE", pidfile)
			// Prepended, never replaced: LookPath takes the first match so the
			// shim still wins, and the shim's own `sleep` has to resolve.
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Cleanup(func() { killRecordedDescendant(t, pidfile) })

			ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
			defer cancel()

			budget := testDeadline + app.GitWaitDelay + slack

			// run goes in a goroutine so an unbounded read fails the test in
			// `budget` rather than hanging until go test's own timeout. That
			// matters for the guard script, which runs this sabotaged and
			// needs the red verdict in seconds.
			done := make(chan result, 1)
			start := time.Now()
			go func() { done <- run(ctx, t.TempDir(), "rev-parse", "--abbrev-ref", "HEAD") }()

			select {
			case res := <-done:
				elapsed := time.Since(start)

				// The two anti-vacuity assertions. A test asserting that
				// something does NOT hang passes perfectly when the
				// reproduction silently fails to reproduce — a shim that is
				// not found, or a `sleep` that does not resolve, returns in a
				// millisecond and looks exactly like a fix working. Both
				// checks below fail in that case: an unstarted command leaves
				// ctxErr nil, and it cannot have taken the deadline to do it.
				if !errors.Is(res.ctxErr, context.DeadlineExceeded) {
					t.Fatalf("ctxErr = %v (exit %d, stderr %q), want context.DeadlineExceeded — "+
						"the shim did not hang, so this run proves nothing about the bound",
						res.ctxErr, res.exitCode, strings.TrimSpace(res.stderr))
				}
				if elapsed < testDeadline {
					t.Fatalf("run returned after %v, before its own %v deadline — "+
						"the read never reached the deadline it is supposed to be bounded past",
						elapsed, testDeadline)
				}

				t.Logf("bounded: run returned after %v (%v deadline + %v WaitDelay)",
					elapsed.Round(time.Millisecond), testDeadline, app.GitWaitDelay)

			case <-time.After(budget):
				t.Fatalf("run did not return within %v (%v deadline + %v WaitDelay + %v slack) — "+
					"the read is unbounded: a descendant is holding the captured stdout pipe "+
					"open past the deadline",
					budget, testDeadline, app.GitWaitDelay, slack)
			}
		})
	}
}

// killRecordedDescendant kills the orphan the bare-sleep shim left behind, if
// there is one. Absence is not a failure: the exec-sleep case records no pid
// because it deliberately leaves nothing running.
//
// It also unblocks a run() that is still hanging — in the sabotaged build the
// goroutine above is stuck on the pipe this process is holding.
func killRecordedDescendant(t *testing.T, pidfile string) {
	t.Helper()

	raw, err := os.ReadFile(pidfile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Logf("descendant pid %q is not a number; leaving it to exit on its own", raw)
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Kill()
}
