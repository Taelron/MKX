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
// carrying the description from its `## comment`.
func TestDiscover(t *testing.T) {
	want := []domain.Target{
		{Name: "confirm", Description: "Ask for confirmation before proceeding"},
		{Name: "fail", Description: "Always exits with error"},
		{Name: "greet", Description: "Greet someone (usage: make greet NAME=Alice)"},
		{Name: "hello", Description: "Print a greeting"},
		{Name: "long-running", Description: "Simulate a long task (5s)"},
		{Name: "multiline", Description: "Print lots of output"},
		{Name: "no-desc", Description: ""},
		{Name: "prompt", Description: "Ask for user input"},
	}

	got, err := makex.NewRunner().Discover(context.Background(), testdataDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("got %d targets, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("target %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
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
