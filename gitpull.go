package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type gitPullFinishedMsg struct {
	projectIndex int
	err          error
}

type gitPullExec struct {
	project      string
	dir          string
	projectIndex int
	err          error
}

func (g *gitPullExec) Run() error {
	sep := strings.Repeat("━", 50)
	fmt.Printf("\n%s\n▶ git pull  (%s)\n%s\n\n", sep, g.project, sep)

	c := exec.Command("git", "pull")
	c.Dir = g.dir
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	g.err = c.Run()

	status := "✓ pull complete"
	if g.err != nil {
		status = fmt.Sprintf("✗ %v", g.err)
	}
	fmt.Printf("\n%s\n%s\nPress Enter to return to mkx...", sep, status)
	bufio.NewReader(os.Stdin).ReadBytes('\n')

	return g.err
}

func (g *gitPullExec) SetStdin(r io.Reader)  {}
func (g *gitPullExec) SetStdout(w io.Writer) {}
func (g *gitPullExec) SetStderr(w io.Writer) {}

func gitPull(projectIndex int, proj project) tea.Cmd {
	gp := &gitPullExec{
		project:      proj.Name,
		dir:          proj.Path,
		projectIndex: projectIndex,
	}

	return tea.Exec(gp, func(err error) tea.Msg {
		return gitPullFinishedMsg{
			projectIndex: gp.projectIndex,
			err:          gp.err,
		}
	})
}
