// Package makex is the Make adapter: it discovers a project's targets and
// builds the command that runs one.
//
// Discovery is the two-strategy model ratified in ADR-M002 — `make -pRrq`
// first, a regex scan of the Makefile as the fallback when make errors or
// yields nothing.
//
// This file holds the impure edges: running make, reading Makefiles, and the
// composition of the two. The parsing itself is in parse.go and is pure.
package makex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// Runner discovers and runs Make targets. Construct it with NewRunner.
type Runner struct{}

var _ app.MakeRunner = (*Runner)(nil)

// NewRunner returns a Runner.
func NewRunner() *Runner {
	return &Runner{}
}

// Discover extracts Make targets from a project directory.
// Tries `make -pRrq` first, falls back to regex parsing.
func (r *Runner) Discover(ctx context.Context, projectPath string) ([]domain.Target, error) {
	targets := parseDatabase(runDatabase(ctx, projectPath))
	src := readMakefile(projectPath)

	if len(targets) == 0 {
		// The fallback carries its own descriptions, read from the same source.
		targets = parseMakefile(src)
	} else {
		applyDescriptions(targets, descriptions(src))
	}

	sortTargets(targets)
	return targets, nil
}

// TargetCommand returns the command that runs the named target in dir.
func (r *Runner) TargetCommand(_ context.Context, dir, name string) domain.Command {
	return domain.Command{Argv: []string{"make", name}, WorkDir: dir}
}

// sortTargets orders targets alphabetically by name, case-insensitive.
func sortTargets(targets []domain.Target) {
	sort.Slice(targets, func(i, j int) bool {
		return strings.ToLower(targets[i].Name) < strings.ToLower(targets[j].Name)
	})
}

// runDatabase captures `make -pRrq` output for a project directory, or "" when
// make cannot be run at all.
func runDatabase(ctx context.Context, projectPath string) string {
	cmd := exec.CommandContext(ctx, "make", "-pRrq", "-C", projectPath)
	cmd.Stdin = nil
	// The C locale is pinned per ADR-M002; the parser keys on the database's
	// English section markers.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")

	out, _ := cmd.Output() // make -q exits non-zero, that's OK
	return string(out)
}

// readMakefile returns the project's Makefile source, or "" when it has none.
// `Makefile` wins over `makefile` when both exist.
func readMakefile(projectPath string) string {
	for _, name := range []string{"Makefile", "makefile"} {
		src, err := os.ReadFile(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		return string(src)
	}
	return ""
}
