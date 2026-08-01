// Package makex is the Make adapter: it discovers a project's targets and
// builds the command that runs one.
//
// Discovery is the two-strategy model ratified in ADR-M002 — `make -pRrq`
// first, a regex scan of the Makefile as the fallback when make errors or
// yields nothing.
package makex

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	targets, err := parseWithMake(ctx, projectPath)
	if err != nil || len(targets) == 0 {
		targets, err = parseWithRegex(projectPath)
	}
	sortTargets(targets)
	return targets, err
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

func parseWithMake(ctx context.Context, projectPath string) ([]domain.Target, error) {
	cmd := exec.CommandContext(ctx, "make", "-pRrq", "-C", projectPath)
	cmd.Stdin = nil
	out, _ := cmd.Output() // make -q exits non-zero, that's OK

	var targets []domain.Target
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	inFiles := false

	for scanner.Scan() {
		line := scanner.Text()

		// skip the "# Files" section marker
		if line == "# Files" {
			inFiles = true
			continue
		}
		if strings.HasPrefix(line, "# Not a target:") {
			// next line is not a real target, skip it
			if scanner.Scan() {
				// consumed the fake target line
			}
			continue
		}

		if !inFiles {
			continue
		}

		// target lines look like "name: deps"
		if len(line) == 0 || line[0] == '#' || line[0] == '\t' || line[0] == '.' {
			continue
		}

		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}

		name := strings.TrimSpace(line[:idx])

		// filter out pattern rules, variable refs, internal targets
		if strings.ContainsAny(name, "%$") {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		targets = append(targets, domain.Target{Name: name})
	}

	// enrich with descriptions from the Makefile comments
	enrichDescriptions(projectPath, targets)

	return targets, nil
}

var makeTargetRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*):\s*(?:.*?##\s*(.*))?$`)

func parseWithRegex(projectPath string) ([]domain.Target, error) {
	for _, name := range []string{"Makefile", "makefile"} {
		f, err := os.Open(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		defer f.Close()

		var targets []domain.Target
		seen := make(map[string]bool)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			matches := makeTargetRe.FindStringSubmatch(scanner.Text())
			if matches == nil {
				continue
			}
			tName := matches[1]
			if seen[tName] {
				continue
			}
			seen[tName] = true
			targets = append(targets, domain.Target{
				Name:        tName,
				Description: strings.TrimSpace(matches[2]),
			})
		}
		return targets, nil
	}
	return nil, nil
}

var descRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*):\s*.*?##\s*(.*)$`)

func enrichDescriptions(projectPath string, targets []domain.Target) {
	for _, name := range []string{"Makefile", "makefile"} {
		f, err := os.Open(filepath.Join(projectPath, name))
		if err != nil {
			continue
		}
		defer f.Close()

		descs := make(map[string]string)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			matches := descRe.FindStringSubmatch(scanner.Text())
			if matches != nil {
				descs[matches[1]] = strings.TrimSpace(matches[2])
			}
		}

		for i := range targets {
			if d, ok := descs[targets[i].Name]; ok {
				targets[i].Description = d
			}
		}
		return
	}
}
