package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func noopHandler(m Model, _ string) (Model, tea.Cmd) { return m, nil }

// navKeymap is the shape the dedupe rules exist for: two handlers, four keys,
// one visible entry.
func navKeymap() keymap {
	return keymap{
		{keys: []string{"up", "k"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor up", inBar: true, handler: noopHandler},
		{keys: []string{"down", "j"}, display: "↑↓", label: "Navigate",
			help: "Move the cursor down", inBar: true, handler: noopHandler},
		{keys: []string{"enter"}, display: "Enter", label: "Run",
			help: "Run the selected target", inBar: true, handler: noopHandler},
		{keys: []string{"q", "ctrl+c"}, display: "q", label: "Quit",
			help: "Quit mkx", handler: noopHandler},
	}
}

func TestHintBarFormat(t *testing.T) {
	got := ansi.Strip(navKeymap().hintBar())
	want := "↑↓/Navigate  Enter/Run"

	if got != want {
		t.Errorf("hintBar() = %q, want %q", got, want)
	}
}

func TestHintBarOmitsHelpOnlyBindings(t *testing.T) {
	if got := ansi.Strip(navKeymap().hintBar()); strings.Contains(got, "Quit") {
		t.Errorf("hintBar() included the inBar:false q/Quit binding: %q", got)
	}
}

func TestHelpRowsIncludeHelpOnlyBindings(t *testing.T) {
	rows := navKeymap().helpRows()

	var found bool
	for _, r := range rows {
		if strings.Contains(r, "Quit mkx") {
			found = true
		}
	}
	if !found {
		t.Errorf("helpRows() dropped the help-only q binding: %v", rows)
	}
}

// Dedupe is what removes the need for an asymmetric suppression flag on the
// down-arrow binding.
func TestDedupeOnDisplayAndLabel(t *testing.T) {
	k := navKeymap()

	if got := strings.Count(ansi.Strip(k.hintBar()), "↑↓/Navigate"); got != 1 {
		t.Errorf("hintBar() rendered ↑↓/Navigate %d times, want 1", got)
	}

	var navRows int
	for _, r := range k.helpRows() {
		if strings.HasPrefix(r, "↑↓") {
			navRows++
		}
	}
	if navRows != 1 {
		t.Errorf("helpRows() rendered %d ↑↓ rows, want 1", navRows)
	}
}

func TestHelpRowsAlignTheKeyColumn(t *testing.T) {
	rows := navKeymap().helpRows()
	if len(rows) == 0 {
		t.Fatal("helpRows() returned nothing")
	}

	// Display columns, not byte offsets — "↑↓" is two columns and six bytes.
	column := func(row string) int {
		prefix, _, _ := strings.Cut(row, "—")
		return lipgloss.Width(prefix)
	}

	want := column(rows[0])
	for i, r := range rows {
		if got := column(r); got != want {
			t.Errorf("row %d puts the separator at column %d, want %d: %q", i, got, want, r)
		}
	}
}

func TestDispatchReturnsTheHandlerForEveryKey(t *testing.T) {
	k := navKeymap()

	for _, b := range k {
		for _, key := range b.keys {
			if k.dispatch(key) == nil {
				t.Errorf("dispatch(%q) = nil, want the %q handler", key, b.label)
			}
		}
	}
}

func TestDispatchReturnsNilForAnUnboundKey(t *testing.T) {
	if navKeymap().dispatch("z") != nil {
		t.Error(`dispatch("z") returned a handler for an unbound key`)
	}
}

// The drift check the registry exists to make impossible, stated as an
// invariant over an arbitrary keymap.
func TestHelpIsASupersetOfTheHintBar(t *testing.T) {
	k := navKeymap()
	rows := strings.Join(k.helpRows(), "\n")

	for _, b := range k {
		if !b.inBar {
			continue
		}
		if !strings.Contains(rows, b.display) {
			t.Errorf("hint bar advertises %q but help does not document it", b.display)
		}
	}
}
