// Command mkx is a terminal UI for discovering and running Make targets
// across a workspace of projects.
//
// This file is the sole wiring point: it constructs the adapters, injects them
// into the app layer, and hands the result to the TUI. See ADR-M001.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Gaetan-Jaminon/mkx/internal/adapter/gitx"
	"github.com/Gaetan-Jaminon/mkx/internal/adapter/makex"
	"github.com/Gaetan-Jaminon/mkx/internal/adapter/workspace"
	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui"
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

	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	application := app.New(
		workspace.NewScanner(workspace.DefaultExcludes, workspace.DefaultMaxDepth),
		makex.NewRunner(),
		gitx.NewReader(),
	)

	projects, err := application.DiscoverProjects(rootCtx, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning workspace: %v\n", err)
		os.Exit(1)
	}

	p := tui.NewProgram(rootCtx, application, root, projects)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
