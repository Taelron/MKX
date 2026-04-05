package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("mkx %s (%s)\n", version, commit)
		os.Exit(0)
	}

	workspace, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	projects, err := scanWorkspace(workspace, defaultExcludes, 4)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning workspace: %v\n", err)
		os.Exit(1)
	}

	m := newModel(workspace, projects)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
