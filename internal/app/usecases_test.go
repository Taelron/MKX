package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// fakeScanner and fakeRunner stand in for the filesystem and Make, so the use
// cases can be exercised without either.
type fakeScanner struct {
	projects []domain.Project
	err      error
	readmes  map[string]string
}

func (f *fakeScanner) Scan(context.Context, string) ([]domain.Project, error) {
	return f.projects, f.err
}

func (f *fakeScanner) ReadmePath(_ context.Context, dir string) string { return f.readmes[dir] }

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

// fakeGitReader stands in for git. It records the context it was called with,
// which is how the deadline App.RepoState applies is asserted without a
// deliberately slow subprocess.
type fakeGitReader struct {
	state domain.RepoState
	err   error

	gotDir      string
	gotDeadline time.Time
	gotHasDL    bool
	calls       int
}

func (f *fakeGitReader) State(ctx context.Context, dir string) (domain.RepoState, error) {
	f.calls++
	f.gotDir = dir
	f.gotDeadline, f.gotHasDL = ctx.Deadline()
	return f.state, f.err
}

func newApp(s *fakeScanner, r *fakeRunner) *app.App { return app.New(s, r, &fakeGitReader{}) }

func newAppWithGit(s *fakeScanner, r *fakeRunner, g *fakeGitReader) *app.App {
	return app.New(s, r, g)
}

// TestRepoStateBoundsTheRead checks the read runs under a deadline, and one
// that is neither absent nor unreasonably far out. ADR-M003 requires captured
// reads to be bounded; an unbounded read is a hung TUI.
func TestRepoStateBoundsTheRead(t *testing.T) {
	git := &fakeGitReader{state: domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"}}
	proj := domain.Project{Name: "alpha", Path: "/w/alpha"}

	before := time.Now()
	got, err := newAppWithGit(&fakeScanner{}, &fakeRunner{}, git).RepoState(context.Background(), proj)
	if err != nil {
		t.Fatalf("RepoState: %v", err)
	}

	if !git.gotHasDL {
		t.Fatal("RepoState called the port with no deadline; a captured read must be bounded")
	}
	if budget := git.gotDeadline.Sub(before); budget <= 0 || budget > 10*time.Second {
		t.Errorf("deadline budget = %v, want a positive budget no larger than a few seconds", budget)
	}

	// Resolved against the project's path, which is what git's upward
	// discovery then resolves to the containing repository.
	if git.gotDir != proj.Path {
		t.Errorf("read ran against %q, want the project path %q", git.gotDir, proj.Path)
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want the port's answer passed through", got.Branch)
	}
}

// TestRepoStatePassesErrorsThrough checks the absence sentinel survives the use
// case, so the UI can tell "no repository here" from "the read failed" with
// errors.Is rather than by string-matching.
func TestRepoStatePassesErrorsThrough(t *testing.T) {
	git := &fakeGitReader{err: app.ErrNotARepository}
	_, err := newAppWithGit(&fakeScanner{}, &fakeRunner{}, git).RepoState(
		context.Background(), domain.Project{Path: "/w/plain"})

	if !errors.Is(err, app.ErrNotARepository) {
		t.Errorf("RepoState err = %v, want it to wrap app.ErrNotARepository", err)
	}
}

// TestDiscoverProjectsReadsNoGitState pins the lazy-read decision at the layer
// that would otherwise make it expensive: discovery walks the whole workspace,
// and if it read git state per project, thirty projects would mean ninety
// subprocesses before the TUI opened.
func TestDiscoverProjectsReadsNoGitState(t *testing.T) {
	git := &fakeGitReader{}
	scanner := &fakeScanner{projects: []domain.Project{
		{Name: "alpha", Path: "/w/alpha"},
		{Name: "bravo", Path: "/w/bravo"},
	}}

	if _, err := newAppWithGit(scanner, &fakeRunner{}, git).DiscoverProjects(context.Background(), "/w"); err != nil {
		t.Fatalf("DiscoverProjects: %v", err)
	}

	if git.calls != 0 {
		t.Errorf("DiscoverProjects issued %d git reads, want 0 — reads are lazy on drill-in", git.calls)
	}
}

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

// TestCheckoutBranch checks the checkout descriptor and the three arguments
// the use case refuses.
func TestCheckoutBranch(t *testing.T) {
	project := domain.Project{Name: "alpha", Path: "/w/alpha"}
	onMain := domain.RepoState{
		Head:     domain.HeadOnBranch,
		Branch:   "main",
		Branches: []string{"main", "feature"},
	}
	application := newApp(&fakeScanner{}, &fakeRunner{})

	got, err := application.CheckoutBranch(context.Background(), project, onMain, "feature")
	if err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	assertCommand(t, got, []string{"git", "checkout", "feature"}, "/w/alpha")

	// The branch already checked out is not special-cased: git says "Already
	// on 'main'" and that is a better answer than one MkX invents.
	if _, err := application.CheckoutBranch(context.Background(), project, onMain, "main"); err != nil {
		t.Errorf("CheckoutBranch onto the current branch: %v, want a command", err)
	}

	refusals := []struct {
		name   string
		state  domain.RepoState
		branch string
	}{
		{
			name:   "a branch the state does not list",
			state:  onMain,
			branch: "nope",
		},
		{
			name:   "a repository with no commits",
			state:  domain.RepoState{Head: domain.HeadUnborn},
			branch: "main",
		},
		{
			name:   "an empty branch name",
			state:  onMain,
			branch: "",
		},
	}

	for _, tt := range refusals {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := application.CheckoutBranch(context.Background(), project, tt.state, tt.branch)
			if err == nil {
				t.Fatalf("got the command %+v, want a refusal", cmd)
			}
			if len(cmd.Argv) != 0 {
				t.Errorf("a refusal still carried a command: %+v", cmd)
			}
		})
	}
}

// TestCheckoutBranchDoesNotPreValidateTheCheckout is the distinction ADR-M003
// draws, pinned so it cannot erode into "helpfully" refusing.
//
// Argument validation — is this a branch this state knows about — belongs in
// the use case. Checkout policy does not: MkX must not refuse on a dirty tree,
// must not stash, and must not offer --force. A dirty state that still yields a
// command is what proves the first without the second, and git remains the sole
// judge of whether the checkout can proceed.
func TestCheckoutBranchDoesNotPreValidateTheCheckout(t *testing.T) {
	project := domain.Project{Name: "alpha", Path: "/w/alpha"}
	dirty := domain.RepoState{
		Head:     domain.HeadOnBranch,
		Branch:   "main",
		Dirty:    true,
		Branches: []string{"main", "feature"},
	}

	got, err := newApp(&fakeScanner{}, &fakeRunner{}).
		CheckoutBranch(context.Background(), project, dirty, "feature")
	if err != nil {
		t.Fatalf("a dirty tree was refused: %v — ADR-M003 leaves that judgement to git", err)
	}
	assertCommand(t, got, []string{"git", "checkout", "feature"}, "/w/alpha")

	for _, arg := range got.Argv {
		if arg == "--force" || arg == "-f" || arg == "stash" {
			t.Errorf("the command carries %q: %+v", arg, got.Argv)
		}
	}
}

// A detached HEAD has no current branch, and switching to one is a legitimate
// escape rather than a state to refuse.
func TestCheckoutBranchFromADetachedHead(t *testing.T) {
	detached := domain.RepoState{Head: domain.HeadDetached, Branches: []string{"main"}}

	got, err := newApp(&fakeScanner{}, &fakeRunner{}).CheckoutBranch(
		context.Background(), domain.Project{Name: "alpha", Path: "/w/alpha"}, detached, "main")
	if err != nil {
		t.Fatalf("CheckoutBranch from a detached head: %v", err)
	}
	assertCommand(t, got, []string{"git", "checkout", "main"}, "/w/alpha")
}

// TestReadmePath checks the use case passes the project's directory — not its
// name — through to the scanner, and reports "" for a project with no README.
func TestReadmePath(t *testing.T) {
	scanner := &fakeScanner{readmes: map[string]string{
		"/w/alpha": "/w/alpha/README.md",
	}}
	application := newApp(scanner, &fakeRunner{})

	got := application.ReadmePath(context.Background(), domain.Project{Name: "alpha", Path: "/w/alpha"})
	if got != "/w/alpha/README.md" {
		t.Errorf("ReadmePath: got %q, want %q", got, "/w/alpha/README.md")
	}

	if got := application.ReadmePath(context.Background(), domain.Project{Name: "beta", Path: "/w/beta"}); got != "" {
		t.Errorf("ReadmePath for a project with no README: got %q, want %q", got, "")
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
