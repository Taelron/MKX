package tui

// The batched-input tests.
//
// Every behavioural test here drives Model.Update with a tea.KeyMsg literal
// carrying all of its runes at once, written inline. That is deliberate and it
// is the point: this defect was found by a harness that batched where a human
// would not, and a helper that builds a key per rune cannot reproduce it. There
// is no helper in this file, and the literals stay literals.
//
// What is asserted is almost entirely behaviour that must *not* happen — no
// checkout, no pull, no target run, no quit — so each test also pins the
// positive half where there is one, to keep "nothing happened" from passing for
// the wrong reason.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/app"
	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// ------------------------------------------------------- the discriminator

// TestIsBatch pins the discriminator itself, including the case that rules out
// the simpler rule: a bracketed paste of a single rune is a batch. Length alone
// would let a pasted `b` open the branch picker.
func TestIsBatch(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyMsg
		want bool
	}{
		{name: "bracketed paste, many runes",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/boot"), Paste: true},
			want: true},
		{name: "bracketed paste, one rune",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Paste: true},
			want: true},
		{name: "bracketed paste carrying a carriage return",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("make\r"), Paste: true},
			want: true},
		{name: "raw replay, many runes",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")},
			want: true},
		{name: "a typed rune",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")},
			want: false},
		{name: "enter",
			msg:  tea.KeyMsg{Type: tea.KeyEnter},
			want: false},
		{name: "space",
			msg:  tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}},
			want: false},
		{name: "alt-modified rune",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g"), Alt: true},
			want: false},
		{name: "alt-modified multi-rune, which the library never emits",
			msg:  tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("gg"), Alt: true},
			want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBatch(tt.msg); got != tt.want {
				t.Errorf("isBatch(%+v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

// ------------------------------------------------------- the sink that opens

// TestPastedFilterStringOpensFilterModeAndTypesTheRest is the headline
// criterion: pasting `/boot` leaves filter mode active with `boot` typed.
//
// [red] before the fix: Key.String() is "[/boot]" for a bracketed paste, which
// matches no binding, so dispatch discarded it.
func TestPastedFilterStringOpensFilterModeAndTypesTheRest(t *testing.T) {
	m := filterModel()

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/boot"), Paste: true})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if !got.filter.active {
		t.Error("a pasted `/boot` did not activate filter mode")
	}
	if got.filter.text != "boot" {
		t.Errorf("filter text = %q, want %q", got.filter.text, "boot")
	}
	if cmd != nil {
		t.Error("a batch issued a command; a batch never returns one")
	}
}

// TestReplayedFilterStringDoesTheSame is the same paste as it arrives from a
// multiplexer replaying buffered bytes: no bracketing, so Paste is false.
//
// [red] before the fix: String() is the literal "/boot", which matches no
// binding either.
func TestReplayedFilterStringDoesTheSame(t *testing.T) {
	m := filterModel()

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/boot")})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if !got.filter.active {
		t.Error("a replayed `/boot` did not activate filter mode")
	}
	if got.filter.text != "boot" {
		t.Errorf("filter text = %q, want %q", got.filter.text, "boot")
	}
	if cmd != nil {
		t.Error("a batch issued a command; a batch never returns one")
	}
}

// TestAnEmptyPasteIsASilentNoOp covers an empty clipboard, which arrives as a
// bracketed paste carrying no runes. There is no lead rune to look up and no
// batch to ignore, so it is silent rather than a flash — and the guard that
// makes it silent is also what keeps the lead-rune index from panicking.
func TestAnEmptyPasteIsASilentNoOp(t *testing.T) {
	m := filterModel()

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(""), Paste: true})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if cmd != nil {
		t.Error("an empty paste issued a command")
	}
	if got.flash != "" {
		t.Errorf("an empty paste set the flash %q; there was no input to ignore", got.flash)
	}
	if got.filter != (filterState{}) {
		t.Errorf("an empty paste changed the filter to %+v", got.filter)
	}
}

// ---------------------------------------------- the five colliding key names

// TestBatchSpellingEnterDoesNotRunATarget is the collision found during
// planning, and it is the worst of them: a raw batch's String() is its literal
// text, so a replay spelling `enter` *is* Enter to whole-string dispatch.
//
// [red] before the fix, and loudly: the selected target runs.
func TestBatchSpellingEnterDoesNotRunATarget(t *testing.T) {
	var ran string
	m := filterModel()
	m.app = app.New(fakeScanner{}, recordingRunner{ran: &ran}, fakeGitReader{})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})

	if ran != "" {
		t.Errorf("a batch spelling `enter` ran the target %q", ran)
	}
	if cmd != nil {
		t.Error("a batch spelling `enter` issued a command")
	}
}

// TestBatchSpellingEnterInTheBranchPickerDoesNotCheckOut is why tier 1 is in
// scope. A checkout is only reachable behind a modal, so a guard at the view
// tier alone would leave the mutation this issue names as strictly worse than
// dropping the input.
//
// [red] before the fix: the picker's Enter binding fires confirmBranch and the
// terminal is handed to `git checkout`.
func TestBatchSpellingEnterInTheBranchPickerDoesNotCheckOut(t *testing.T) {
	opened := openPicker(t, pickerModel(domain.RepoState{
		Head:     domain.HeadOnBranch,
		Branch:   "main",
		Branches: []string{"main", "feature"},
	}))

	after, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("enter")})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if cmd != nil {
		t.Error("a batch spelling `enter` handed the terminal to a checkout through the picker")
	}
	if !got.modal.active {
		t.Error("a batch closed the picker; inside a modal a batch is a no-op")
	}
	if got.modal.input.cursor != opened.modal.input.cursor {
		t.Errorf("a batch moved the picker cursor: %d → %d",
			opened.modal.input.cursor, got.modal.input.cursor)
	}
}

// TestBatchSpellingCtrlCDoesNotQuit — the same collision, reachable from every
// view. [red] before the fix: mkx exits.
func TestBatchSpellingCtrlCDoesNotQuit(t *testing.T) {
	for _, v := range []view{viewProjects, viewTargets} {
		m := filterModel()
		m.view = v

		if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+c")}); cmd != nil {
			t.Errorf("view %v: a batch spelling `ctrl+c` issued a command; it quits", v)
		}
	}
}

// ---------------------------------------------- the three mutating lead runes

// TestBatchBeginningWithBDoesNotOpenTheBranchPicker guards both halves at once.
// The modal assertion is a regression guard against the naive per-rune fix,
// which would make a pasted `branch` open the picker; the flash is what fails
// today, because the batch is currently discarded in silence.
func TestBatchBeginningWithBDoesNotOpenTheBranchPicker(t *testing.T) {
	base := pickerModel(domain.RepoState{
		Head:     domain.HeadOnBranch,
		Branch:   "main",
		Branches: []string{"main", "feature"},
	})

	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("branch")},
		{Type: tea.KeyRunes, Runes: []rune("branch"), Paste: true},
	} {
		after, cmd := base.Update(msg)
		got, ok := after.(Model)
		if !ok {
			t.Fatalf("Update returned %T, not a Model", after)
		}

		if got.modal.active {
			t.Errorf("%q (Paste=%v) opened the branch picker", string(msg.Runes), msg.Paste)
		}
		if cmd != nil {
			t.Errorf("%q (Paste=%v) issued a command", string(msg.Runes), msg.Paste)
		}
		// [red] before the fix.
		if got.flash == "" {
			t.Errorf("%q (Paste=%v) was discarded in silence; an ignored batch is legible in a view",
				string(msg.Runes), msg.Paste)
		}
	}
}

// TestBatchBeginningWithGDoesNotPull — `g` hands the terminal to git pull.
func TestBatchBeginningWithGDoesNotPull(t *testing.T) {
	m := filterModel()

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("gpull")})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if cmd != nil {
		t.Error("a batch beginning with `g` issued a command; it pulls")
	}
	// [red] before the fix.
	if got.flash == "" {
		t.Error("a batch beginning with `g` was discarded in silence")
	}
}

// TestBatchBeginningWithCapitalRDoesNotOpenTheReadme — `R` is a full-screen
// handover to the pager.
func TestBatchBeginningWithCapitalRDoesNotOpenTheReadme(t *testing.T) {
	m := filterModel()

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Readme")})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if cmd != nil {
		t.Error("a batch beginning with `R` issued a command; it opens the README")
	}
	// [red] before the fix.
	if got.flash == "" {
		t.Error("a batch beginning with `R` was discarded in silence")
	}
}

// TestBatchCarryingEnterDoesNotRunATarget covers the runes only a bracketed
// paste can carry. A raw replay never contains a control character —
// detectOneMsg breaks the run on one — so a batch beginning with or ending in
// Enter is expressible only as a paste.
//
// The third case is the one that proves the stripping rather than the guard: it
// leads with `/`, so the sink does open, and the carriage return must not land
// in the filter text.
func TestBatchCarryingEnterDoesNotRunATarget(t *testing.T) {
	tests := []struct {
		name       string
		runes      string
		wantFilter string
	}{
		{name: "a batch beginning with Enter", runes: "\rmake"},
		{name: "a batch ending in Enter", runes: "make\r"},
		{name: "a sink-opening batch ending in Enter", runes: "/make\r", wantFilter: "make"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ran string
			m := filterModel()
			m.app = app.New(fakeScanner{}, recordingRunner{ran: &ran}, fakeGitReader{})

			after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.runes), Paste: true})
			got, ok := after.(Model)
			if !ok {
				t.Fatalf("Update returned %T, not a Model", after)
			}

			if ran != "" {
				t.Errorf("%q ran the target %q", tt.runes, ran)
			}
			if cmd != nil {
				t.Errorf("%q issued a command", tt.runes)
			}
			// [red] before the fix, for the sink-opening case: no filter opened
			// at all, so there was no text to strip the CR from.
			if got.filter.text != tt.wantFilter {
				t.Errorf("%q left filter text %q, want %q — control runes are stripped",
					tt.runes, got.filter.text, tt.wantFilter)
			}
		})
	}
}

// ---------------------------------------------------- the view with no sink

// TestBatchInTheProjectViewIsIgnored proves the project view needs no code of
// its own: it declares no `/`, so the same one rule ignores the batch and the
// flash it derives from the registry names no key to press.
//
// [red] before the fix: no flash at all.
func TestBatchInTheProjectViewIsIgnored(t *testing.T) {
	m := filterModel()
	m.view = viewProjects
	m.projectCursor = 0

	after, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/boot"), Paste: true})
	got, ok := after.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", after)
	}

	if cmd != nil {
		t.Error("a batch in the project view issued a command")
	}
	if got.view != viewProjects {
		t.Errorf("a batch changed the view to %v", got.view)
	}
	if got.projectCursor != m.projectCursor {
		t.Errorf("a batch moved the project cursor: %d → %d", m.projectCursor, got.projectCursor)
	}
	if got.filter != (filterState{}) {
		t.Errorf("a batch opened a filter in a view that has none: %+v", got.filter)
	}
	if got.flash == "" {
		t.Fatal("a batch in the project view was discarded in silence")
	}
	if strings.Contains(got.flash, "/") {
		t.Errorf("the project view's flash names `/` (%q); it binds no such key", got.flash)
	}
}

// -------------------------------------------------- the registry invariant

// TestOnlyTheFilterBindingOpensATextSink is the deny-by-default claim, pinned
// across every keymap in the product, modal keymaps included.
//
// The modal keymaps are covered even though tier 1 never calls dispatchBatch
// today, and deliberately so: the flag is what a future modal with a text field
// would tick to become batch-reachable, so silently acquiring one is exactly
// what this test exists to catch. A keymap is checked here before it is wired,
// not after.
//
// It fails in both directions: a binding that stops declaring the flag fails
// here, and so does a second binding that starts. Adding a text sink is
// therefore a deliberate act with a test to edit, not something that arrives in
// a future PR by accident.
func TestOnlyTheFilterBindingOpensATextSink(t *testing.T) {
	keymaps := map[string]keymap{
		"projects":      projectKeymap(),
		"targets":       targetKeymap(),
		"help":          helpKeymap(),
		"branch picker": branchPickerKeymap(),
		"notice":        noticeKeymap(),
	}

	got := map[string][]string{}
	for name, k := range keymaps {
		for _, b := range k {
			if b.opensTextSink {
				got[name] = append(got[name], b.keys...)
			}
		}
	}

	want := map[string][]string{"targets": {"/"}}

	for name, keys := range got {
		if strings.Join(keys, ",") != strings.Join(want[name], ",") {
			t.Errorf("%s keymap declares opensTextSink on %v, want %v", name, keys, want[name])
		}
	}
	for name, keys := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("%s keymap declares no opensTextSink binding, want %v", name, keys)
		}
	}

	// The lookup the batch path actually uses, not just the field.
	if b, ok := targetKeymap().textSink(); !ok || b.display != "/" {
		t.Errorf("targetKeymap().textSink() = (%q, %v), want (\"/\", true)", b.display, ok)
	}
	if _, ok := projectKeymap().textSink(); ok {
		t.Error("projectKeymap() reports a text sink; it binds no `/`")
	}
}

// ------------------------------------------------------- flash legibility

// TestAnIgnoredBatchFlashSurvivesARun is the legibility criterion in the state
// where it was false: renderTargetList overwrote the flash from lastRun on
// every render, so after any target run the batch flash was invisible.
//
// [red] before the fix.
func TestAnIgnoredBatchFlashSurvivesARun(t *testing.T) {
	m := filterModel()
	m.lastRun = &runResult{ExitCode: 0}
	m.flash = "Pasted input ignored — press / to filter"

	rendered := ansi.Strip(m.renderTargetList())

	if !strings.Contains(rendered, "Pasted input ignored") {
		t.Error("the run receipt overwrote an explicit flash; an explicit flash is newer")
	}
	if strings.Contains(rendered, "✓") {
		t.Error("the run receipt rendered alongside the flash; the flash slot holds one message")
	}

	// With no flash set, the receipt is still what fills the slot.
	m.flash = ""
	if !strings.Contains(ansi.Strip(m.renderTargetList()), "✓") {
		t.Error("the run receipt stopped rendering when nothing else claimed the flash")
	}
}

// ------------------------------------------------- the single-rune boundary

// TestASinglePastedRuneDoesNotFireItsBinding is the case the length-alone
// discriminator gets wrong. A typed `b` must still open the branch picker; a
// pasted `b` must not.
func TestASinglePastedRuneDoesNotFireItsBinding(t *testing.T) {
	base := pickerModel(domain.RepoState{
		Head:     domain.HeadOnBranch,
		Branch:   "main",
		Branches: []string{"main", "feature"},
	})

	typed, _ := base.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if got := typed.(Model); !got.modal.active {
		t.Error("a typed `b` no longer opens the branch picker; the single-key path must be untouched")
	}

	pasted, cmd := base.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Paste: true})
	got, ok := pasted.(Model)
	if !ok {
		t.Fatalf("Update returned %T, not a Model", pasted)
	}
	if got.modal.active {
		t.Error("a pasted `b` opened the branch picker; the discriminator is Paste || len>1, not length alone")
	}
	if cmd != nil {
		t.Error("a pasted `b` issued a command")
	}
	if got.flash == "" {
		t.Error("a pasted `b` was discarded in silence")
	}
}
