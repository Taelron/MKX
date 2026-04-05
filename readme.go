package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	tea "github.com/charmbracelet/bubbletea"
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

func (r *readmeExec) SetStdin(rd io.Reader)  {}
func (r *readmeExec) SetStdout(w io.Writer) {}
func (r *readmeExec) SetStderr(w io.Writer) {}

// findReadme looks for a README file in the given directory.
func findReadme(dir string) string {
	candidates := []string{"README.md", "readme.md", "Readme.md", "README.MD"}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// viewReadme returns a command to display the README for a project.
func viewReadme(proj project) tea.Cmd {
	path := findReadme(proj.Path)
	if path == "" {
		return func() tea.Msg {
			return readmeNotFoundMsg{}
		}
	}
	return tea.Exec(&readmeExec{path: path}, func(err error) tea.Msg {
		return nil
	})
}
