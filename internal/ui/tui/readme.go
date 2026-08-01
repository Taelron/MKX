package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

type readmeNotFoundMsg struct{}

type readmeExec struct {
	path string
}

func (r *readmeExec) Run() error {
	content, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		// fallback: print raw markdown
		fmt.Println(string(content))
		fmt.Print("\nPress Enter to return to mkx...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return nil
	}

	rendered, err := renderer.Render(string(content))
	if err != nil {
		fmt.Println(string(content))
		fmt.Print("\nPress Enter to return to mkx...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return nil
	}

	sep := strings.Repeat("━", 50)
	fmt.Printf("\n%s\n", sep)
	fmt.Print(rendered)
	fmt.Printf("%s\nPress Enter to return to mkx...", sep)
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	return nil
}

func (r *readmeExec) SetStdin(io.Reader)  {}
func (r *readmeExec) SetStdout(io.Writer) {}
func (r *readmeExec) SetStderr(io.Writer) {}

// viewReadme returns a command to display the README at path, or a
// not-found message when the project has none.
//
// It returns handoverDone rather than nil. Viewing a README cannot change a
// branch or dirty a tree, so an exemption here is arguable — but an exemption
// has to be defended and enforced forever, and enforcing it is the expensive
// part. Routing this through costs one redundant sub-second re-read and buys
// ADR-M003's rule with zero exceptions, which is what it literally says.
//
// The previous nil return is also what made the old "invalidate in each
// handover's case" phrasing unenforceable: Update never saw this handover at
// all.
func viewReadme(path string) tea.Cmd {
	if path == "" {
		// Not a handover — no tea.Exec, no terminal given up, nothing that
		// could have changed the repository.
		return func() tea.Msg {
			return readmeNotFoundMsg{}
		}
	}
	return tea.Exec(&readmeExec{path: path}, func(error) tea.Msg {
		return handoverDone{}
	})
}
