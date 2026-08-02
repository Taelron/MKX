package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

// binding is one key-triggered action, declared once and projected three ways:
// into the hint bar, into the help overlay, and into key dispatch.
//
// Carrying the handler here is what closes the drift the issue names. A key
// documented in help but no longer handled, or handled but undocumented, is
// not expressible — there is one declaration, not three lists to keep in step.
type binding struct {
	// keys are every key string that triggers this action, as tea.KeyMsg
	// reports them: {"up", "k"}.
	keys []string
	// display is how the key reads to the user: "↑↓".
	display string
	// label is the hint bar text: "Navigate".
	label string
	// help is the fuller sentence the help overlay shows: "Move the cursor up".
	help string
	// inBar marks a binding that appears in the hint bar. Help-only bindings
	// (q/Quit in the target view) clear it.
	inBar bool
	// barPinned marks a bar entry the fitter may never drop when the bar does
	// not fit: the view's exits, Esc/Back and ?/Help. Everything else yields to
	// width, left to right, behind a +N marker.
	//
	// The zero value is droppable, so a binding added later degrades by default
	// rather than silently claiming survival — the same reasoning opensTextSink
	// carries above.
	//
	// Two tiers rather than a priority int: the data supports "the user can
	// always leave and can always reach the full list" and nothing finer.
	// Priority is not derivable from bar order either — dropping right to left
	// would take Esc/Back off the target view at 79 columns while keeping
	// R/Readme. See hintbar.go.
	barPinned bool
	// opensTextSink marks the one kind of binding a batch may fire: one whose
	// whole effect is to switch the view into a mode that captures keystrokes
	// as text. Today that is `/` and nothing else. The zero value is false, so
	// a binding added later is inert to pasted input until someone
	// deliberately says otherwise.
	//
	// It is not "safe to fire from a paste". `?` is safe to fire and opens no
	// sink, so ticking it would make a pasted `?xyz` open help and drop `xyz`.
	// Naming the field for the capability means it can only be ticked where
	// the rest of the batch has somewhere to go. See batch.go.
	opensTextSink bool
	// handler runs the action. It receives the key that triggered it, so one
	// handler can serve several keys.
	handler func(Model, string) (Model, tea.Cmd)
}

// keymap is a view's complete set of bindings — the single source for its hint
// bar, its help overlay and its key dispatch.
type keymap []binding

// hintEntry is one deduped hint bar entry, in display order. It is what the
// bar is assembled from, so the fitter can drop and count entries rather than
// slicing a finished string — see fitHintBar in hintbar.go.
type hintEntry struct {
	display string
	label   string
	pinned  bool
}

// text is the entry as the user reads it, unstyled: "↑↓/Navigate". Widths are
// measured from this rather than from render(), so a measurement is the same
// under a TTY and under `go test`, where lipgloss emits no escapes.
func (e hintEntry) text() string { return e.display + "/" + e.label }

// render is the entry as the bar shows it: the key styled, the /label dimmed.
func (e hintEntry) render() string {
	return styles.HintKey.Render(e.display) + styles.HintAction.Render("/"+e.label)
}

// hintBarSep separates hint bar entries, per the @UI Patterns format.
const hintBarSep = "  "

// barEntries returns the inBar bindings as hint entries, in declaration order.
//
// Entries are deduped on display+label: ↑/k and ↓/j are four keys and two
// handlers, but one visible ↑↓/Navigate.
//
// hintBar and the width fitter both build on this, so there is one dedup path
// and the full-width bar and a degraded one can never disagree about what the
// view's entries are.
func (k keymap) barEntries() []hintEntry {
	var entries []hintEntry
	seen := make(map[string]bool)

	for _, b := range k {
		if !b.inBar {
			continue
		}
		key := b.display + "/" + b.label
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, hintEntry{display: b.display, label: b.label, pinned: b.barPinned})
	}

	return entries
}

// hintBar renders every bar entry as styled "display/label" pairs separated by
// two spaces, per the @UI Patterns hint bar format.
//
// This is the bar at its natural width, with nothing dropped — the prescribed
// format, which TestHintBarsMatchTheBaselineFormats pins byte for byte. What
// actually reaches the screen goes through fitHintBar, which may drop entries
// when the terminal is too narrow to hold them all.
func (k keymap) hintBar() string {
	entries := k.barEntries()

	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, e.render())
	}

	return strings.Join(parts, hintBarSep)
}

// helpRows renders every binding — not just the ones in the hint bar — as
// "display — help" rows with the key column aligned, per the @UI Patterns help
// overlay format. Deduped the same way as hintBar.
func (k keymap) helpRows() []string {
	type row struct{ display, help string }

	var rows []row
	seen := make(map[string]bool)
	width := 0

	for _, b := range k {
		key := b.display + "/" + b.label
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row{display: b.display, help: b.help})
		if w := lipgloss.Width(b.display); w > width {
			width = w
		}
	}

	out := make([]string, 0, len(rows))
	for _, r := range rows {
		pad := width - lipgloss.Width(r.display)
		out = append(out, r.display+strings.Repeat(" ", pad)+"  —  "+r.help)
	}
	return out
}

// bindingFor returns the binding that claims key, or false when none does. The
// first match wins, so an earlier binding shadows a later one.
//
// dispatch wants only the handler; the batch path needs the whole binding, to
// read opensTextSink before it fires anything. One lookup serves both.
func (k keymap) bindingFor(key string) (binding, bool) {
	for _, b := range k {
		if slices.Contains(b.keys, key) {
			return b, true
		}
	}
	return binding{}, false
}

// dispatch returns the handler for key, or nil when no binding claims it.
func (k keymap) dispatch(key string) func(Model, string) (Model, tea.Cmd) {
	if b, ok := k.bindingFor(key); ok {
		return b.handler
	}
	return nil
}

// textSink returns this keymap's text-sink binding, if it declares one.
//
// It answers "does this view have a sink at all, and which key opens it" —
// which is what ignoredBatchFlash needs to name a key in the message a view
// shows when it drops a batch. dispatchBatch does not use it: that path
// already has a key in hand, so it looks the lead rune up with bindingFor and
// reads opensTextSink off the binding it gets back.
func (k keymap) textSink() (binding, bool) {
	for _, b := range k {
		if b.opensTextSink {
			return b, true
		}
	}
	return binding{}, false
}
