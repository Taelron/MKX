package app_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// TestCharacterization locks MkX's observable discovery behaviour — which
// projects are found, and which targets and descriptions each one carries —
// against a golden file recorded before the ADR-M001 layer extraction.
//
// The golden holds parsed results only: no raw make output, no absolute paths.
// A project's identity is its workspace-relative name, so the file is machine-
// and make-version-independent (the fixtures use plain explicit targets with
// `## ` descriptions, nothing that varies across GNU make releases).
//
// A diff here means behaviour changed. The golden is not to be regenerated to
// make this test pass.
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

	got := serializeProjects(projects)

	want, err := os.ReadFile(characterizationGolden)
	if os.IsNotExist(err) {
		if writeErr := os.WriteFile(characterizationGolden, []byte(got), 0o644); writeErr != nil {
			t.Fatalf("writing initial golden %s: %v", characterizationGolden, writeErr)
		}
		t.Fatalf("golden %s did not exist; wrote it from this run. Inspect it, commit it, and re-run.", characterizationGolden)
	}
	if err != nil {
		t.Fatalf("reading golden %s: %v", characterizationGolden, err)
	}

	if got != string(want) {
		t.Errorf("discovery over %s differs from %s.\n\n--- want (golden) ---\n%s\n--- got ---\n%s",
			characterizationRoot, characterizationGolden, want, got)
	}
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
