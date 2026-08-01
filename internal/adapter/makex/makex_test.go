package makex_test

import (
	"context"
	"testing"

	"github.com/Gaetan-Jaminon/mkx/internal/adapter/makex"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

const testdataDir = "../../../testdata"

// TestDiscover checks the make -pRrq parse over the repository's own sample
// Makefile: every target is found, in case-insensitive name order, each
// carrying the description from its `## comment` and the phony status from the
// Makefile's `.PHONY:` line — which names all eight.
func TestDiscover(t *testing.T) {
	want := []domain.Target{
		{Name: "confirm", Description: "Ask for confirmation before proceeding", Phony: domain.PhonyYes},
		{Name: "fail", Description: "Always exits with error", Phony: domain.PhonyYes},
		{Name: "greet", Description: "Greet someone (usage: make greet NAME=Alice)", Phony: domain.PhonyYes},
		{Name: "hello", Description: "Print a greeting", Phony: domain.PhonyYes},
		{Name: "long-running", Description: "Simulate a long task (5s)", Phony: domain.PhonyYes},
		{Name: "multiline", Description: "Print lots of output", Phony: domain.PhonyYes},
		{Name: "no-desc", Description: "", Phony: domain.PhonyYes},
		{Name: "prompt", Description: "Ask for user input", Phony: domain.PhonyYes},
	}

	got, err := makex.NewRunner().Discover(context.Background(), testdataDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !equalTarget(got[i], want[i]) {
			t.Errorf("target %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// equalTarget compares two targets field by field. domain.Target holds a slice
// and so is no longer comparable with ==; a hand-rolled helper keeps the test
// honest without adding a module dependency for it.
func equalTarget(a, b domain.Target) bool {
	if a.Name != b.Name || a.Description != b.Description || a.Phony != b.Phony {
		return false
	}
	if len(a.Prerequisites) != len(b.Prerequisites) {
		return false
	}
	for i := range a.Prerequisites {
		if a.Prerequisites[i] != b.Prerequisites[i] {
			return false
		}
	}
	return true
}

// TestDiscoverNoMakefile checks that a directory without a Makefile yields no
// targets rather than an error — the walk only ever calls Discover on
// directories that have one, but the fallback path must not panic.
func TestDiscoverNoMakefile(t *testing.T) {
	got, err := makex.NewRunner().Discover(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d targets, want 0: %+v", len(got), got)
	}
}

// TestTargetCommand checks the descriptor handed to the terminal.
func TestTargetCommand(t *testing.T) {
	got := makex.NewRunner().TargetCommand(context.Background(), testdataDir, "hello")

	want := domain.Command{Argv: []string{"make", "hello"}, WorkDir: testdataDir}
	if got.WorkDir != want.WorkDir {
		t.Errorf("WorkDir: got %q, want %q", got.WorkDir, want.WorkDir)
	}
	if len(got.Argv) != len(want.Argv) {
		t.Fatalf("Argv: got %q, want %q", got.Argv, want.Argv)
	}
	for i := range want.Argv {
		if got.Argv[i] != want.Argv[i] {
			t.Errorf("Argv: got %q, want %q", got.Argv, want.Argv)
			break
		}
	}
}
