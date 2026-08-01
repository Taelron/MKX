package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// statusFunc renders the line printed under the separator once a handover
// finishes. Each caller supplies its own so the terminal output is worded for
// the action the user asked for.
type statusFunc func(exitCode int, err error, d time.Duration) string

// commandExec hands the terminal to a domain.Command and waits for Enter
// before returning to the TUI. It is the single handover path for everything
// MkX runs — target runs and git pull alike — per ADR-M003.
type commandExec struct {
	command domain.Command
	label   string
	status  statusFunc

	start    time.Time
	exitCode int
	duration time.Duration
	err      error
}

func (c *commandExec) Run() error {
	sep := strings.Repeat("━", 50)
	fmt.Printf("\n%s\n▶ %s  (%s)\n%s\n\n", sep, strings.Join(c.command.Argv, " "), c.label, sep)

	cmd := exec.Command(c.command.Argv[0], c.command.Argv[1:]...)
	cmd.Dir = c.command.WorkDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	c.err = cmd.Run()
	c.duration = time.Since(c.start)

	if c.err != nil {
		if exitErr, ok := c.err.(*exec.ExitError); ok {
			c.exitCode = exitErr.ExitCode()
		} else {
			c.exitCode = 1
		}
	}

	fmt.Printf("\n%s\n%s\nPress Enter to return to mkx...", sep, c.status(c.exitCode, c.err, c.duration))
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	return c.err
}

func (c *commandExec) SetStdin(io.Reader)  { /* handled in Run */ }
func (c *commandExec) SetStdout(io.Writer) { /* handled in Run */ }
func (c *commandExec) SetStderr(io.Writer) { /* handled in Run */ }

// handoverDone wraps the message every tea.Exec handover produces.
//
// Update keys RepoState invalidation on this one type rather than on each
// handover's own message, so a handover added later invalidates automatically
// with nothing to remember at the call site. ADR-M003 says RepoState is
// invalidated by *every* handover, and a rule phrased as "remember to
// invalidate in each case" was already broken by the code that existed before
// this type did.
//
// inner is nil for a handover that has no result to report; Update unwraps,
// invalidates, and only then dispatches inner if there is one.
//
// handover_guard_test.go enforces that every tea.Exec callback in this package
// constructs one of these.
type handoverDone struct{ inner tea.Msg }

// handover runs cmd with full terminal handover, then turns the finished
// commandExec into the message the model expects back — wrapped, so the
// invalidation in Update fires before that message is dispatched.
func handover(cmd domain.Command, label string, status statusFunc, done func(*commandExec) tea.Msg) tea.Cmd {
	ce := &commandExec{
		command: cmd,
		label:   label,
		status:  status,
		start:   time.Now(),
	}
	return tea.Exec(ce, func(error) tea.Msg { return handoverDone{inner: done(ce)} })
}

// targetStatus reports a Make target's exit code and how long it took.
func targetStatus(exitCode int, _ error, d time.Duration) string {
	status := "✓ success"
	if exitCode != 0 {
		status = fmt.Sprintf("✗ exit %d", exitCode)
	}
	return fmt.Sprintf("%s (%s)", status, d.Round(time.Second))
}

// pullStatus reports whether git pull succeeded, quoting git's own error.
func pullStatus(_ int, err error, _ time.Duration) string {
	if err != nil {
		return fmt.Sprintf("✗ %v", err)
	}
	return "✓ pull complete"
}
