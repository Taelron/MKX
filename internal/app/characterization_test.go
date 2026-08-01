package app_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Gaetan-Jaminon/mkx/internal/adapter/makex"
	"github.com/Gaetan-Jaminon/mkx/internal/adapter/workspace"
	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

const (
	characterizationRoot   = "../../testdata/fixtures"
	characterizationGolden = "../../testdata/fixtures.golden"
	characterizationDepth  = 2
)

// rebaseline re-anchors the golden to current behaviour. It is deliberately a
// flag rather than an automatic fallback: nothing in a plain `go test ./...`
// or `make test` can set it, so the suite can never quietly rewrite the oracle
// it is supposed to be checked against. Reach it through `make
// rebaseline-golden`, and only for a reviewed behaviour change.
var rebaseline = flag.Bool("rebaseline", false,
	"rewrite the characterization golden from current behaviour (deliberate, reviewed changes only)")

// TestCharacterization locks MkX's observable discovery behaviour — which
// projects are found, and which targets and descriptions each one carries —
// against a golden file recorded before the ADR-M001 layer extraction.
//
// The golden holds parsed results only: no raw make output, no absolute paths.
// A project's identity is its workspace-relative name, so the file is machine-
// and make-version-independent (the fixtures use plain explicit targets with
// `## ` descriptions, nothing that varies across GNU make releases).
//
// A diff here means behaviour changed. This test never writes the golden: a
// missing, unreadable, or malformed golden fails outright rather than being
// regenerated, because a test that rewrites its own oracle on failure checks
// nothing. Re-anchoring is `make rebaseline-golden`, and only for a reviewed
// behaviour change.
func TestCharacterization(t *testing.T) {
	root, err := filepath.Abs(characterizationRoot)
	if err != nil {
		t.Fatalf("resolving %s: %v", characterizationRoot, err)
	}

	application := app.New(
		workspace.NewScanner(workspace.DefaultExcludes, characterizationDepth),
		makex.NewRunner(),
	)

	projects, err := application.DiscoverProjects(context.Background(), root)
	if err != nil {
		t.Fatalf("DiscoverProjects(%s): %v", root, err)
	}

	// Guard the vacuous pass. Serialization of zero projects is the empty
	// string, which would compare equal to an empty golden — a green run that
	// proves nothing. Discovery finding nothing is itself the failure.
	if len(projects) == 0 {
		t.Fatalf("DiscoverProjects(%s) found no projects; the fixtures are missing or discovery is broken", root)
	}

	got := serializeProjects(projects)

	if *rebaseline {
		rewriteGolden(t, got)
		return
	}

	want, err := os.ReadFile(characterizationGolden)
	if os.IsNotExist(err) {
		t.Fatalf("golden %s does not exist. It is not written automatically. "+
			"If this is a deliberate, reviewed behaviour change, run: make rebaseline-golden",
			characterizationGolden)
	}
	if err != nil {
		t.Fatalf("reading golden %s: %v", characterizationGolden, err)
	}

	assertGoldenWellFormed(t, string(want))

	if got != string(want) {
		t.Errorf("discovery over %s differs from %s.\n\n--- want (golden) ---\n%s\n--- got ---\n%s",
			characterizationRoot, characterizationGolden, want, got)
	}
}

// assertGoldenWellFormed rejects a golden that cannot be a real recording, so
// an empty or truncated file fails as malformed rather than quietly comparing
// equal to a degenerate result.
func assertGoldenWellFormed(t *testing.T, golden string) {
	t.Helper()

	if strings.TrimSpace(golden) == "" {
		t.Fatalf("golden %s is empty; it cannot be a recording of any behaviour", characterizationGolden)
	}
	if !strings.HasPrefix(golden, "project ") {
		first, _, _ := strings.Cut(golden, "\n")
		t.Fatalf("golden %s is malformed: first line is %q, want a line beginning with %q",
			characterizationGolden, first, "project ")
	}
}

// rewriteGolden re-anchors the golden and reports what moved, so an unintended
// rebaseline is visible in the terminal at the moment it happens rather than
// only later in a diff.
func rewriteGolden(t *testing.T, got string) {
	t.Helper()

	display := strings.TrimPrefix(characterizationGolden, "../../")

	old, err := os.ReadFile(characterizationGolden)
	switch {
	case os.IsNotExist(err):
		fmt.Fprintf(os.Stderr, "\nREBASELINE %s (creating)\n", display)
		fmt.Fprintf(os.Stderr, "  %d projects, %d targets recorded\n", countPrefix(got, "project "), countPrefix(got, "  target "))
	case err != nil:
		t.Fatalf("reading golden %s before rebaselining: %v", characterizationGolden, err)
	default:
		added, removed := lineDelta(string(old), got)
		fmt.Fprintf(os.Stderr, "\nREBASELINE %s\n", display)
		for _, l := range removed {
			fmt.Fprintf(os.Stderr, "  - %s\n", l)
		}
		for _, l := range added {
			fmt.Fprintf(os.Stderr, "  + %s\n", l)
		}
		if len(added) == 0 && len(removed) == 0 {
			fmt.Fprintf(os.Stderr, "  (no change — behaviour matches the existing golden)\n")
		} else {
			fmt.Fprintf(os.Stderr, "%d added, %d removed\n", len(added), len(removed))
		}
	}

	if err := os.WriteFile(characterizationGolden, []byte(got), 0o644); err != nil {
		t.Fatalf("writing golden %s: %v", characterizationGolden, err)
	}
	fmt.Fprintf(os.Stderr, "This re-anchors the behaviour oracle. Commit only if the change is intended and reviewed.\n\n")
}

// lineDelta reports which lines the new recording gained and lost. The golden
// is line-oriented and deterministically ordered, so a multiset difference is
// enough and needs no real diff algorithm.
func lineDelta(old, current string) (added, removed []string) {
	count := func(s string) map[string]int {
		m := make(map[string]int)
		for _, line := range strings.Split(s, "\n") {
			if line != "" {
				m[line]++
			}
		}
		return m
	}
	oldLines, currentLines := count(old), count(current)

	for line, n := range currentLines {
		for i := 0; i < n-oldLines[line]; i++ {
			added = append(added, line)
		}
	}
	for line, n := range oldLines {
		for i := 0; i < n-currentLines[line]; i++ {
			removed = append(removed, line)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func countPrefix(s, prefix string) int {
	var n int
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, prefix) {
			n++
		}
	}
	return n
}

// serializeProjects renders discovery results as deterministic, path-free text.
func serializeProjects(projects []domain.Project) string {
	var b strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&b, "project %s\n", filepath.ToSlash(p.Name))
		fmt.Fprintf(&b, "  description: %s\n", p.Description)
		for _, t := range p.Targets {
			fmt.Fprintf(&b, "  target %s: %s\n", t.Name, t.Description)
		}
	}
	return b.String()
}
