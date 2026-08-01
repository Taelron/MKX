package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// helpView opens the help overlay over the named view and returns the full
// rendered screen.
func helpView(t *testing.T, v view) string {
	t.Helper()

	m := testModel()
	m.view = v

	// A resolved git state, so the header carries a settled segment rather
	// than the in-flight "…". TestHelpFitsAtEightyByTwentyFour uses "…" as its
	// truncation sentinel, and a header that is merely still loading is not a
	// truncated help row. Seeding here keeps that guard checking exactly what
	// it was written to check.
	m = m.setGitEntry(m.projects[m.selectedProject].Name, gitEntry{
		status: gitOK,
		state:  domain.RepoState{Head: domain.HeadOnBranch, Branch: "main"},
	})

	opened, _ := m.viewKeymap().dispatch("?")(m, "?")
	if !opened.modal.active {
		t.Fatal("`?` did not open the help overlay")
	}
	return opened.View()
}

// Help lists the keys of the view it was opened over, not a global reference.
// Derived from the view's own keymap, so this checks the wiring rather than
// restating it.
func TestHelpIsContextSensitive(t *testing.T) {
	projects := ansi.Strip(helpView(t, viewProjects))
	targets := ansi.Strip(helpView(t, viewTargets))

	if !strings.Contains(projects, "Open the project's target list") {
		t.Error("the project view's help is missing Enter/Targets")
	}
	if strings.Contains(projects, "Run the selected target") {
		t.Error("the project view's help documents the target view's Enter")
	}
	if strings.Contains(projects, "Back to the project list") {
		t.Error("the project view's help documents Esc/Back, which the root view has no use for")
	}

	if !strings.Contains(targets, "Run the selected target") {
		t.Error("the target view's help is missing Enter/Run")
	}
	if !strings.Contains(targets, "Back to the project list") {
		t.Error("the target view's help is missing Esc/Back")
	}
	if strings.Contains(targets, "Open the project's target list") {
		t.Error("the target view's help documents the project view's Enter")
	}
}

// The removal, seen from the surface the user actually reads.
func TestHelpDocumentsNoRunKeyBesidesEnter(t *testing.T) {
	body := renderHelpBody(targetKeymap())

	for _, row := range strings.Split(body, "\n") {
		if strings.HasPrefix(row, "r ") || strings.HasPrefix(row, "r—") {
			t.Errorf("help still documents an `r` key: %q", row)
		}
	}
	if !strings.Contains(body, "Run the selected target") {
		t.Error("help does not document Enter as the run key")
	}
}

// q is deliberately absent from the target view's hint bar, so help is the only
// place it is documented.
func TestHelpDocumentsTheHelpOnlyQuitKey(t *testing.T) {
	if !strings.Contains(ansi.Strip(helpView(t, viewTargets)), "Quit mkx") {
		t.Error("the target view's help is missing q/Quit")
	}
}

// The modal advertises Esc/Close  ?/Close rather than the baseline's
// Enter/Confirm  Esc/Cancel: info content has nothing to confirm, and a bar
// advertising a no-op Enter is worse than one that omits it.
func TestHelpModalHintBar(t *testing.T) {
	if got := ansi.Strip(helpKeymap().hintBar()); got != "Esc/Close  ?/Close" {
		t.Errorf("help modal hint bar = %q, want %q", got, "Esc/Close  ?/Close")
	}
}

// The load-bearing 80×24 check. Step 13 of the PR's visual sequence is the
// aesthetic counterpart; this is the one that can fail a build.
func TestHelpFitsAtEightyByTwentyFour(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    view
	}{{"projects", viewProjects}, {"targets", viewTargets}} {
		t.Run(tc.name, func(t *testing.T) {
			out := helpView(t, tc.v)
			lines := strings.Split(out, "\n")

			if len(lines) > 24 {
				t.Errorf("rendered %d lines at height 24", len(lines))
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > 80 {
					t.Errorf("line %d is %d columns wide at width 80: %q", i, w, ansi.Strip(line))
				}
			}

			// Nothing actually truncates. A future verbose help string fails
			// the suite instead of silently clipping.
			if strings.Contains(ansi.Strip(out), "…") {
				t.Error("a help row was truncated at 80×24; shorten it or raise the width cap")
			}
		})
	}
}

// Every key the hint bar advertises is also documented in help, for the real
// view keymaps rather than a fixture.
func TestViewHelpIsASupersetOfTheViewHintBar(t *testing.T) {
	for name, k := range viewKeymaps() {
		rows := renderHelpBody(k)
		for _, b := range k {
			if !b.inBar {
				continue
			}
			if !strings.Contains(rows, b.display) {
				t.Errorf("%s view: hint bar shows %q but help does not document it", name, b.display)
			}
		}
	}
}
