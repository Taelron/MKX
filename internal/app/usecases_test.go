package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// fakeScanner and fakeRunner stand in for the filesystem and Make, so the use
// cases can be exercised without either.
type fakeScanner struct {
	projects []domain.Project
	err      error
	readme   string
}

func (f *fakeScanner) Scan(context.Context, string) ([]domain.Project, error) {
	return f.projects, f.err
}

func (f *fakeScanner) ReadmePath(context.Context, string) string { return f.readme }

type fakeRunner struct {
	targets map[string][]domain.Target
	err     error
}

func (f *fakeRunner) Discover(_ context.Context, dir string) ([]domain.Target, error) {
	return f.targets[dir], f.err
}

func (f *fakeRunner) TargetCommand(_ context.Context, dir, name string) domain.Command {
	return domain.Command{Argv: []string{"make", name}, WorkDir: dir}
}

func newApp(s *fakeScanner, r *fakeRunner) *app.App { return app.New(s, r) }

// TestDiscoverProjectsComposesAndSorts checks that DiscoverProjects fills each
// project's targets from the runner and returns the projects in
// case-insensitive name order.
func TestDiscoverProjectsComposesAndSorts(t *testing.T) {
	scanner := &fakeScanner{projects: []domain.Project{
		{Name: "zulu", Path: "/w/zulu"},
		{Name: "Alpha", Path: "/w/Alpha"},
		{Name: "mike", Path: "/w/mike"},
	}}
	runner := &fakeRunner{targets: map[string][]domain.Target{
		"/w/Alpha": {{Name: "build"}},
		"/w/mike":  {{Name: "test"}, {Name: "lint"}},
	}}

	got, err := newApp(scanner, runner).DiscoverProjects(context.Background(), "/w")
	if err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}

	wantOrder := []string{"Alpha", "mike", "zulu"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d projects, want %d", len(got), len(wantOrder))
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Errorf("project %d: got %q, want %q", i, got[i].Name, name)
		}
	}

	if len(got[0].Targets) != 1 || got[0].Targets[0].Name != "build" {
		t.Errorf("Alpha targets: got %+v, want [build]", got[0].Targets)
	}
	if len(got[2].Targets) != 0 {
		t.Errorf("zulu targets: got %+v, want none", got[2].Targets)
	}
}

// TestDiscoverProjectsKeepsProjectsWhoseTargetsFail checks the as-built
// behaviour that a Makefile which will not parse hides that project's targets
// but does not fail the scan.
func TestDiscoverProjectsKeepsProjectsWhoseTargetsFail(t *testing.T) {
	scanner := &fakeScanner{projects: []domain.Project{{Name: "alpha", Path: "/w/alpha"}}}
	runner := &fakeRunner{err: errors.New("make exploded")}

	got, err := newApp(scanner, runner).DiscoverProjects(context.Background(), "/w")
	if err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d projects, want 1", len(got))
	}
	if len(got[0].Targets) != 0 {
		t.Errorf("targets: got %+v, want none", got[0].Targets)
	}
}

// TestRunTarget checks that a known target produces the make command, and an
// unknown one is refused rather than handed to the terminal.
func TestRunTarget(t *testing.T) {
	project := domain.Project{
		Name:    "alpha",
		Path:    "/w/alpha",
		Targets: []domain.Target{{Name: "hello"}},
	}
	application := newApp(&fakeScanner{}, &fakeRunner{})

	got, err := application.RunTarget(context.Background(), project, "hello")
	if err != nil {
		t.Fatalf("RunTarget: %v", err)
	}
	assertCommand(t, got, []string{"make", "hello"}, "/w/alpha")

	if _, err := application.RunTarget(context.Background(), project, "nope"); err == nil {
		t.Error("RunTarget with an unknown target: got nil error, want a refusal")
	}
}

// TestPullAndRefresh checks the git pull descriptor, and that RefreshTargets
// re-reads the project afterwards.
func TestPullAndRefresh(t *testing.T) {
	project := domain.Project{Name: "alpha", Path: "/w/alpha"}
	runner := &fakeRunner{targets: map[string][]domain.Target{
		"/w/alpha": {{Name: "build"}},
	}}
	application := newApp(&fakeScanner{}, runner)

	got, err := application.PullAndRefresh(context.Background(), project)
	if err != nil {
		t.Fatalf("PullAndRefresh: %v", err)
	}
	assertCommand(t, got, []string{"git", "pull"}, "/w/alpha")

	targets, err := application.RefreshTargets(context.Background(), project)
	if err != nil {
		t.Fatalf("RefreshTargets: %v", err)
	}
	if len(targets) != 1 || targets[0].Name != "build" {
		t.Errorf("RefreshTargets: got %+v, want [build]", targets)
	}
}

// TestReadmePath checks the use case passes the project directory through to
// the scanner.
func TestReadmePath(t *testing.T) {
	application := newApp(&fakeScanner{readme: "/w/alpha/README.md"}, &fakeRunner{})

	got := application.ReadmePath(context.Background(), domain.Project{Path: "/w/alpha"})
	if got != "/w/alpha/README.md" {
		t.Errorf("ReadmePath: got %q, want %q", got, "/w/alpha/README.md")
	}
}

func assertCommand(t *testing.T, got domain.Command, wantArgv []string, wantDir string) {
	t.Helper()
	if got.WorkDir != wantDir {
		t.Errorf("WorkDir: got %q, want %q", got.WorkDir, wantDir)
	}
	if len(got.Argv) != len(wantArgv) {
		t.Fatalf("Argv: got %q, want %q", got.Argv, wantArgv)
	}
	for i := range wantArgv {
		if got.Argv[i] != wantArgv[i] {
			t.Errorf("Argv: got %q, want %q", got.Argv, wantArgv)
			return
		}
	}
}
