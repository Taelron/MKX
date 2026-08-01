package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// fakeScanner and fakeRunner stand in for the filesystem and Make. The key
// bindings only need an App that answers without touching either.
type fakeScanner struct{ readme string }

func (f fakeScanner) Scan(context.Context, string) ([]domain.Project, error) {
	return nil, nil
}
func (f fakeScanner) ReadmePath(context.Context, string) string { return f.readme }

type fakeRunner struct{}

func (fakeRunner) Discover(context.Context, string) ([]domain.Target, error) { return nil, nil }
func (fakeRunner) TargetCommand(_ context.Context, dir, name string) domain.Command {
	return domain.Command{Argv: []string{"make", name}, WorkDir: dir}
}

// testModel is a Model on the target list of a project with two targets,
// cursor parked on the second.
func testModel() Model {
	return Model{
		app:     app.New(fakeScanner{readme: "/tmp/README.md"}, fakeRunner{}),
		rootCtx: context.Background(),
		projects: []domain.Project{{
			Name: "alpha",
			Path: "/tmp/alpha",
			Targets: []domain.Target{
				{Name: "build", Description: "Compile"},
				{Name: "test", Description: "Run tests"},
			},
		}},
		view:            viewTargets,
		selectedProject: 0,
		targetCursor:    1,
		width:           80,
		height:          24,
	}
}

func viewKeymaps() map[string]keymap {
	return map[string]keymap{
		"projects": projectKeymap(),
		"targets":  targetKeymap(),
	}
}

// A documented key that lost its handler fails here.
func TestEveryBindingDispatches(t *testing.T) {
	for name, k := range viewKeymaps() {
		for _, b := range k {
			for _, key := range b.keys {
				if k.dispatch(key) == nil {
					t.Errorf("%s view: dispatch(%q) = nil for the %q binding", name, key, b.label)
				}
			}
		}
	}
}

// `r` used to run the selected target. @UI Patterns reserves it for refresh,
// so it is unbound — asserted, so a future re-bind is a deliberate act.
func TestRIsUnbound(t *testing.T) {
	for name, k := range viewKeymaps() {
		if k.dispatch("r") != nil {
			t.Errorf("%s view: `r` is bound again; @UI Patterns reserves it for refresh", name)
		}
		for _, b := range k {
			if b.display == "r" {
				t.Errorf("%s view: a binding still displays as `r` (%q)", name, b.label)
			}
		}
	}
}

func TestHintBarsMatchTheBaselineFormats(t *testing.T) {
	tests := []struct {
		name string
		k    keymap
		want string
	}{
		{
			name: "projects",
			k:    projectKeymap(),
			want: "↑↓/Navigate  Enter/Targets  g/Pull  R/Readme  q/Quit  ?/Help",
		},
		{
			name: "targets",
			k:    targetKeymap(),
			want: "↑↓/Navigate  Enter/Run  g/Pull  R/Readme  Esc/Back  ?/Help",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ansi.Strip(tt.k.hintBar()); got != tt.want {
				t.Errorf("hintBar() = %q\nwant           %q", got, tt.want)
			}
		})
	}
}

// q is reachable and documented in the target view, but does not claim one of
// the bar's last two slots, which the baseline gives to Esc/Back and ?/Help.
func TestQuitIsHelpOnlyInTheTargetView(t *testing.T) {
	k := targetKeymap()

	if strings.Contains(ansi.Strip(k.hintBar()), "Quit") {
		t.Error("target hint bar carries q/Quit; it should be help-only")
	}
	if k.dispatch("q") == nil {
		t.Error("q is not dispatched in the target view")
	}
	if !strings.Contains(strings.Join(k.helpRows(), "\n"), "Quit mkx") {
		t.Error("q/Quit is missing from the target view's help rows")
	}
}

func TestQuestionMarkOpensHelpRatherThanTheReadme(t *testing.T) {
	for name, k := range viewKeymaps() {
		h := k.dispatch("?")
		if h == nil {
			t.Fatalf("%s view: `?` is unbound", name)
		}

		got, cmd := h(testModel(), "?")
		if !got.modal.active {
			t.Errorf("%s view: `?` did not open a modal", name)
		}
		if got.modal.content != modalHelp {
			t.Errorf("%s view: `?` opened content %v, want modalHelp", name, got.modal.content)
		}
		if cmd != nil {
			t.Errorf("%s view: `?` issued a command; the README handover moved to R", name)
		}
	}
}

func TestCapitalRShowsTheReadme(t *testing.T) {
	for name, k := range viewKeymaps() {
		h := k.dispatch("R")
		if h == nil {
			t.Fatalf("%s view: `R` is unbound", name)
		}

		got, cmd := h(testModel(), "R")
		if cmd == nil {
			t.Errorf("%s view: `R` issued no command", name)
		}
		if got.modal.active {
			t.Errorf("%s view: `R` opened a modal; the README is a full-screen handover", name)
		}
	}
}

// The freeze claim, tested rather than argued: with a modal up, no key reaches
// the underlying view.
func TestModalFreezesTheUnderlyingView(t *testing.T) {
	before := testModel()

	opened, _ := projectKeymap().dispatch("?")(before, "?")
	if !opened.modal.active {
		t.Fatal("failed to open the modal")
	}

	m := tea.Model(opened)
	for _, key := range []string{"j", "k", "down", "up", "g", "enter", "q", "R", "esc-not-a-key", "x"} {
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd != nil {
			t.Errorf("key %q returned a command through the modal", key)
		}
	}

	after, ok := m.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", m)
	}
	if !after.modal.active {
		t.Error("the modal closed on a key it does not bind")
	}
	if after.targetCursor != before.targetCursor {
		t.Errorf("targetCursor moved: %d → %d", before.targetCursor, after.targetCursor)
	}
	if after.view != before.view {
		t.Errorf("view changed: %v → %v", before.view, after.view)
	}
	if after.selectedProject != before.selectedProject {
		t.Errorf("selectedProject changed: %d → %d", before.selectedProject, after.selectedProject)
	}
	if after.lastRun != before.lastRun {
		t.Error("lastRun changed behind the modal")
	}
}

func TestEscAndQuestionMarkCloseTheModal(t *testing.T) {
	for _, key := range []string{"esc", "?"} {
		before := testModel()
		opened, _ := targetKeymap().dispatch("?")(before, "?")

		h := opened.modal.keys.dispatch(key)
		if h == nil {
			t.Fatalf("%q does not close the help overlay", key)
		}

		closed, cmd := h(opened, key)
		if closed.modal.active {
			t.Errorf("%q left the modal active", key)
		}
		if cmd != nil {
			t.Errorf("%q issued a command while closing", key)
		}
		if closed.targetCursor != before.targetCursor || closed.view != before.view {
			t.Errorf("%q disturbed the underlying view", key)
		}
	}
}

// Handlers are individually reachable now, so they guard rather than index
// blindly into an empty project list.
func TestBindingsAreSafeWithNoProjects(t *testing.T) {
	for name, k := range viewKeymaps() {
		for _, b := range k {
			for _, key := range b.keys {
				if key == "q" || key == "ctrl+c" {
					continue // returns tea.Quit; nothing to guard
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							t.Errorf("%s view: key %q panicked with no projects: %v", name, key, r)
						}
					}()
					k.dispatch(key)(Model{width: 80, height: 24}, key)
				}()
			}
		}
	}
}
