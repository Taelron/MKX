package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Batched key input: runes that arrived in a single terminal read.
//
// The rule, stated once and enforced in one place:
//
//	A batch may only ever open a text sink. Its first rune only is looked up
//	in the active keymap, and the handler runs only if that binding declares
//	that it opens a text sink. The remaining runes then go to the sink as
//	literal text. A batch whose first rune opens no sink fires nothing and
//	sets a flash saying so. A batch never returns a tea.Cmd. While a modal is
//	up, a batch is a no-op.
//
// There are two batch shapes and both are live:
//
//	bracketed  a real paste into a bracketed-paste terminal. Paste is true,
//	           and the batch carries every rune between the markers — \r, \n,
//	           tabs and spaces included, at any length, including one.
//	raw        a multiplexer replaying buffered bytes. Paste is false, and the
//	           batch is a contiguous run of printable non-space runes, because
//	           bubbletea's detectOneMsg breaks the run on a control character
//	           or a space.
//
// The raw shape is why this file exists at all rather than trusting the
// library. Key.String() brackets a pasted batch, so `[/boot]` matches no
// binding and a bracketed paste can never activate a shortcut. A raw batch is
// not bracketed: its String() is the literal text, so a replay spelling
// `enter`, `esc`, `up`, `down` or `ctrl+c` *is* that key to whole-string
// dispatch. In the branch picker that means a replayed `enter` performs a
// checkout. Structurally keeping a batch away from dispatch is what closes
// that, and it closes it for every binding at once rather than for a list of
// them that a future binding could fall off.
//
// Known and not closed: a raw replay of "make\n" arrives as KeyRunes("make")
// and then a separate KeyEnter. The batch becomes filter text; the bare Enter
// is indistinguishable from a real keypress and still runs the selected
// target. Bracketed paste — the default, and what Zellij forwards — prevents
// it, because there the \r stays a rune inside the batch. Closing the residual
// case needs a timing heuristic that would misfire on a fast typist and cannot
// be tested without injecting a clock.

// isBatch reports whether msg carries runes that arrived together.
//
// The discriminator is Paste *or* length, not length alone: a bracketed paste
// of a single `b` is still pasted input, and length alone would let it open
// the branch picker. The !Alt term mirrors the filter capture — an
// alt-modified rune is a chord, and the library never emits more than one rune
// after Alt.
func isBatch(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes && !msg.Alt && (msg.Paste || len(msg.Runes) > 1)
}

// dispatchBatch is the whole of what a batch may do.
//
// Tier 3 in handleKey is its only caller today. Tier 1 does not reach here at
// all: with a modal up a batch is a hard no-op, because no modal has a text
// field for the remaining runes to land in.
//
// It still takes the keymap as a parameter rather than reading m.viewKeymap()
// itself, and that is the one line of preparation this file makes for a modal
// that grows one. Such a modal would tick opensTextSink on its own binding and
// point tier 1 here; nothing in this function would change. Reading the view
// keymap internally would hard-wire the assumption that only a view can hold a
// sink, which is the assumption most likely to stop being true.
func (m Model) dispatchBatch(k keymap, msg tea.KeyMsg) (Model, tea.Cmd) {
	if len(msg.Runes) == 0 {
		// An empty bracketed paste — an empty clipboard. Nothing to lead with,
		// and the lead-rune index below would panic.
		return m, nil
	}

	lead := string(msg.Runes[0])

	b, found := k.bindingFor(lead)
	if !found || !b.opensTextSink {
		m.flash = k.ignoredBatchFlash()
		return m, nil
	}

	// The returned command is deliberately discarded, and that is the third of
	// the three layers that make "no batch fires a checkout, a pull, or a
	// target run" a property of this signature rather than of a review: even a
	// binding wrongly marked as a sink cannot hand the terminal to a process.
	//
	// It has a cost worth writing down. A future opensTextSink binding whose
	// handler legitimately needs to return a command — a sink that must fetch
	// something before it can accept text — would have that command silently
	// dropped. Today `/` sets state and returns nil, so the discard costs
	// nothing. Whoever adds the second text sink revisits this on purpose
	// rather than discovering it as a bug.
	m, _ = b.handler(m, lead)

	// The binding claimed to open a sink; this is where that claim is checked.
	// A binding that says it opens one and does not is a registry bug, and
	// without this the remaining runes would go nowhere with no signal.
	//
	// This is the one line that knows the sink *is* filter mode. With exactly
	// one sink in the product a generic sink interface would be speculative;
	// the second sink makes this the place to generalise.
	if !m.filter.active {
		m.flash = k.ignoredBatchFlash()
		return m, nil
	}

	return m.appendFilterRunes(msg.Runes[1:]), nil
}

// ignoredBatchFlash is what a view says when it drops a batch, derived from the
// registry rather than written per view.
//
// A view that declares a text sink names its key, so the target view reads
// "Pasted input ignored — press / to filter" and the project view, which binds
// no `/`, reads "Pasted input ignored". Neither view holds a line of
// batch-specific code.
func (k keymap) ignoredBatchFlash() string {
	if b, ok := k.textSink(); ok {
		return "Pasted input ignored — press " + b.display + " to filter"
	}
	return "Pasted input ignored"
}
