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
func viewReadme(path string) tea.Cmd {
	if path == "" {
		return func() tea.Msg {
			return readmeNotFoundMsg{}
		}
	}
	return tea.Exec(&readmeExec{path: path}, func(error) tea.Msg {
		return nil
	})
}
