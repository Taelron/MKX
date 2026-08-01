package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// filterModel is testModel's project with a third target whose name does not
// match but whose description does — the case that proves matching reaches
// past Name.
func filterModel() Model {
	m := testModel()
	m.projects[0].Targets = []domain.Target{
		{Name: "build", Description: "Compile the binary"},
		{Name: "test", Description: "Run tests"},
		{Name: "verify", Description: "Check discovery against the golden"},
	}
	m.targetCursor = 0
	return m
}

func names(ts []domain.Target) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}

// ---------------------------------------------------------------- matching

func TestFilteredMatching(t *testing.T) {
	targets := filterModel().projects[0].Targets
	fields := func(tg domain.Target) []string { return []string{tg.Name, tg.Description} }

	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "name substring", text: "ver", want: []string{"verify"}},
		{name: "description substring, name does not match", text: "golden", want: []string{"verify"}},
		{name: "lowercase needle against mixed-case field", text: "compile", want: []string{"build"}},
		{name: "uppercase needle", text: "COMPILE", want: []string{"build"}},
		{name: "empty text returns everything", text: "", want: []string{"build", "test", "verify"}},
		{name: "no match returns nothing", text: "zzz", want: nil},
		{name: "matches several", text: "e", want: []string{"build", "test", "verify"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(filtered(targets, tt.text, fields))
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("filtered(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

// Substring, not fuzzy: the characters of "bld" appear in "build" in order,
// and must not match.
func TestFilteredIsSubstringNotFuzzy(t *testing.T) {
	targets := filterModel().projects[0].Targets
	got := filtered(targets, "bld", func(tg domain.Target) []string {
		return []string{tg.Name, tg.Description}
	})
	if len(got) != 0 {
		t.Errorf("filtered(%q) = %v, want no matches — matching is substring, not fuzzy", "bld", names(got))
	}
}

// The helper is generic over its item type; the field extractor is what a
// caller supplies. Asserted so a future refactor to a concrete
// filteredTargets() shows up here.
func TestFilteredIsGenericOverTheItemType(t *testing.T) {
	got := filtered([]string{"alpha", "beta"}, "et", func(s string) []string { return []string{s} })
	if len(got) != 1 || got[0] != "beta" {
		t.Errorf("filtered over []string = %v, want [beta]", got)
	}
}

// ------------------------------------------------------- the state machine

func TestSlashActivatesFilterMode(t *testing.T) {
	m := filterModel()
	m.targetCursor = 2

	got, cmd := targetKeymap().dispatch("/")(m, "/")

	if !got.filter.active {
		t.Error("`/` did not activate filter mode")
	}
	if got.filter.text != "" {
		t.Errorf("`/` left filter text %q, want empty", got.filter.text)
	}
	if got.targetCursor != 0 {
		t.Errorf("`/` left the cursor at %d, want 0", got.targetCursor)
	}
	if cmd != nil {
		t.Error("`/` issued a command")
	}
}

// `/` starts from scratch rather than resuming the previous filter.
func TestSlashResetsAPreviousFilter(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: false, text: "ver"}

	got, _ := targetKeymap().dispatch("/")(m, "/")

	if got.filter.text != "" {
		t.Errorf("`/` resumed the previous filter (%q), want a fresh empty one", got.filter.text)
	}
}

func TestBackspaceDeletesOneRune(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: "vér"}

	got := m.backspaceFilter()

	if got.filter.text != "vé" {
		t.Errorf("backspace = %q, want %q — deletion is rune-aware, not byte-aware", got.filter.text, "vé")
	}
}

func TestBackspaceOnEmptyTextIsANoOp(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: ""}

	if got := m.backspaceFilter(); got.filter.text != "" {
		t.Errorf("backspace on empty text = %q, want empty", got.filter.text)
	}
}

// The invariant that spans two structs: every text change resets the cursor.
func TestCursorResetsOnEveryTextChange(t *testing.T) {
	tests := []struct {
		name   string
		change func(Model) Model
	}{
		{name: "append", change: func(m Model) Model { return m.appendFilterRunes([]rune("v")) }},
		{name: "backspace", change: func(m Model) Model { return m.backspaceFilter() }},
		{name: "clear", change: func(m Model) Model { return m.clearFilter() }},
		{name: "activate", change: func(m Model) Model { return m.activateFilter() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := filterModel()
			m.filter = filterState{active: true, text: "te"}
			m.targetCursor = 2

			if got := tt.change(m); got.targetCursor != 0 {
				t.Errorf("%s left the cursor at %d, want 0", tt.name, got.targetCursor)
			}
		})
	}
}

// The three Esc states, which is the whole reason Esc is not a single branch.
func TestEscByFilterState(t *testing.T) {
	tests := []struct {
		name       string
		start      filterState
		wantView   view
		wantFilter filterState
	}{
		{
			name:       "filter mode active: clear the text, close the mode, stay",
			start:      filterState{active: true, text: "ver"},
			wantView:   viewTargets,
			wantFilter: filterState{},
		},
		{
			name:       "mode closed but text still set: clear the text, stay",
			start:      filterState{active: false, text: "ver"},
			wantView:   viewTargets,
			wantFilter: filterState{},
		},
		{
			name:       "active with empty text: close the mode, stay",
			start:      filterState{active: true, text: ""},
			wantView:   viewTargets,
			wantFilter: filterState{},
		},
		{
			name:       "no filter in effect: back to the project list",
			start:      filterState{},
			wantView:   viewProjects,
			wantFilter: filterState{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := filterModel()
			m.filter = tt.start

			got, cmd := targetKeymap().dispatch("esc")(m, "esc")

			if got.view != tt.wantView {
				t.Errorf("view = %v, want %v", got.view, tt.wantView)
			}
			if got.filter != tt.wantFilter {
				t.Errorf("filter = %+v, want %+v", got.filter, tt.wantFilter)
			}
			if cmd != nil {
				t.Error("esc issued a command")
			}
		})
	}
}

func TestEnterWhileFilteringClosesTheModeAndRunsNothing(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: "ver"}

	got, cmd := targetKeymap().dispatch("enter")(m, "enter")

	if cmd != nil {
		t.Error("enter ran a target while filter mode was active; it is the mode exit")
	}
	if got.filter.active {
		t.Error("enter left filter mode active")
	}
	if got.filter.text != "ver" {
		t.Errorf("enter cleared the filter text (%q); the matched results stay visible", got.filter.text)
	}
}

// recordingRunner captures the target name the app was asked to build a
// command for. Nothing is executed: tea.Exec only wraps the command in a
// message, and the process would run in the Bubble Tea runtime, not here.
type recordingRunner struct{ ran *string }

func (recordingRunner) Discover(context.Context, string) ([]domain.Target, error) {
	return nil, nil
}

func (r recordingRunner) TargetCommand(_ context.Context, dir, name string) domain.Command {
	*r.ran = name
	return domain.Command{Argv: []string{"make", name}, WorkDir: dir}
}

// The bug this issue would otherwise ship: with a filter in effect, cursor 0 is
// the first *match*, not proj.Targets[0].
func TestEnterRunsTheFilteredTargetNotTheUnfilteredOne(t *testing.T) {
	var ran string
	m := filterModel()
	m.app = app.New(fakeScanner{}, recordingRunner{ran: &ran})
	m.filter = filterState{active: false, text: "ver"} // matches only "verify"
	m.targetCursor = 0

	_, cmd := targetKeymap().dispatch("enter")(m, "enter")
	if cmd == nil {
		t.Fatal("enter issued no command with a filter in effect")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("the command produced no message")
	}

	if ran != "verify" {
		t.Errorf("enter ran %q, want %q — the cursor indexes the filtered slice, not proj.Targets", ran, "verify")
	}

	// A cursor past the end of the filtered slice must not run anything.
	ran = ""
	m.targetCursor = 1
	if _, cmd := targetKeymap().dispatch("enter")(m, "enter"); cmd != nil {
		t.Errorf("enter ran %q with the cursor past the last match", ran)
	}
}

func TestDownIsBoundAgainstTheFilteredLength(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: false, text: "ver"} // one match
	m.targetCursor = 0

	got, _ := targetKeymap().dispatch("down")(m, "down")

	if got.targetCursor != 0 {
		t.Errorf("cursor walked to %d past the only match; down must bound against the filtered length", got.targetCursor)
	}

	// Unfiltered, the same key still moves.
	m.filter = filterState{}
	if got, _ := targetKeymap().dispatch("down")(m, "down"); got.targetCursor != 1 {
		t.Errorf("unfiltered cursor = %d, want 1", got.targetCursor)
	}
}

func TestFilterIsZeroedOnEnteringTheTargetView(t *testing.T) {
	m := filterModel()
	m.view = viewProjects
	m.filter = filterState{active: true, text: "ver"}

	got, _ := projectKeymap().dispatch("enter")(m, "enter")

	if got.filter != (filterState{}) {
		t.Errorf("entering the target view kept filter %+v, want the zero value", got.filter)
	}
}

// --------------------------------------------------------- dispatch tiers

// key builds the tea.KeyMsg the terminal actually produces, which is what the
// capture switches on — msg.Type, not msg.String().
func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// Tier 1 still precedes tier 2: with a modal up, a rune does not reach the
// filter even when filter mode is active.
func TestModalOutranksFilterCapture(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: ""}
	opened, _ := targetKeymap().dispatch("?")(m, "?")
	if !opened.modal.active {
		t.Fatal("failed to open the modal")
	}

	after, _ := opened.Update(runeKey("g"))
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if got.filter.text != "" {
		t.Errorf("a rune typed into the filter (%q) through an open modal", got.filter.text)
	}
	if !got.modal.active {
		t.Error("the modal closed on a key it does not bind")
	}
}

// The rune-capture consequence, asserted rather than assumed: while filtering,
// `g` types instead of pulling.
func TestPrintableKeysTypeIntoTheFilterRatherThanFiring(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: ""}

	for _, r := range []string{"g", "R", "q", "j", "k", "?", "/"} {
		after, cmd := m.Update(runeKey(r))
		got, ok := after.(Model)
		if !ok {
			t.Fatalf("Update returned %T, not a Model", after)
		}
		if got.filter.text != r {
			t.Errorf("key %q: filter text = %q, want %q", r, got.filter.text, r)
		}
		if cmd != nil {
			t.Errorf("key %q: fired its binding while filtering", r)
		}
		if got.modal.active {
			t.Errorf("key %q: opened a modal while filtering", r)
		}
	}
}

func TestFilterCaptureAppendsSpaceButNotAltRunes(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: "run"}

	// Space arrives as KeySpace, not KeyRunes: descriptions contain spaces, so
	// a bare KeyRunes test would silently drop them.
	after, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	got := after.(Model)
	if got.filter.text != "run " {
		t.Errorf("space gave %q, want %q", got.filter.text, "run ")
	}

	// alt+g is not text. Key.String() would report "alt+g", which a
	// string-length test would have accepted.
	after, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g"), Alt: true})
	got = after.(Model)
	if got.filter.text != "run" {
		t.Errorf("alt+g gave %q, want the text unchanged at %q", got.filter.text, "run")
	}
}

func TestBackspaceReachesTheFilterThroughUpdate(t *testing.T) {
	m := filterModel()
	m.filter = filterState{active: true, text: "ver"}

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	got := after.(Model)

	if got.filter.text != "ve" {
		t.Errorf("backspace gave %q, want %q", got.filter.text, "ve")
	}
	if cmd != nil {
		t.Error("backspace issued a command")
	}
}

// Non-printable keys fall through tier 2 to the view's bindings and keep
// working while filtering.
func TestNavigationAndExitKeysStillDispatchWhileFiltering(t *testing.T) {
	base := filterModel()
	base.filter = filterState{active: true, text: ""}

	t.Run("down navigates", func(t *testing.T) {
		after, _ := base.Update(tea.KeyMsg{Type: tea.KeyDown})
		if got := after.(Model); got.targetCursor != 1 {
			t.Errorf("down while filtering left the cursor at %d, want 1", got.targetCursor)
		}
	})

	t.Run("up navigates", func(t *testing.T) {
		m := base
		m.targetCursor = 1
		after, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		if got := after.(Model); got.targetCursor != 0 {
			t.Errorf("up while filtering left the cursor at %d, want 0", got.targetCursor)
		}
	})

	t.Run("enter closes filter mode", func(t *testing.T) {
		after, cmd := base.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if got := after.(Model); got.filter.active {
			t.Error("enter while filtering left the mode active")
		}
		if cmd != nil {
			t.Error("enter while filtering issued a command")
		}
	})

	t.Run("esc clears", func(t *testing.T) {
		m := base
		m.filter = filterState{active: true, text: "ver"}
		after, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		got := after.(Model)
		if got.filter != (filterState{}) {
			t.Errorf("esc while filtering left filter %+v", got.filter)
		}
		if got.view != viewTargets {
			t.Error("esc left the view instead of clearing the filter first")
		}
	})

	t.Run("ctrl+c quits", func(t *testing.T) {
		_, cmd := base.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Error("ctrl+c did not quit while filtering")
		}
	})
}

// ----------------------------------------------------------- projections

// The check that this issue did not undo TAE-54: `/` is declared once, and the
// hint bar and help overlay pick it up without a second list.
func TestSlashProjectsIntoTheHintBarAndHelp(t *testing.T) {
	k := targetKeymap()

	if !strings.Contains(strings.Join(k.helpRows(), "\n"), "Filter by name or description") {
		t.Error("the `/` binding is missing from the target view's help rows")
	}

	var slashBindings int
	for _, b := range k {
		if b.display == "/" {
			slashBindings++
		}
	}
	if slashBindings != 1 {
		t.Errorf("the target keymap declares `/` %d times, want exactly 1", slashBindings)
	}
}

// Out of scope, asserted so it stays that way: the projects view has no filter.
func TestProjectsViewHasNoFilter(t *testing.T) {
	k := projectKeymap()

	if k.dispatch("/") != nil {
		t.Error("`/` is bound in the projects view; filtering the project list is out of scope")
	}
	for _, b := range k {
		if b.display == "/" || b.label == "Search" {
			t.Errorf("the projects keymap carries a Search binding (%q)", b.label)
		}
	}
}
