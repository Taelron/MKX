package tui

import (
	"strings"

	"github.com/Gaetan-Jaminon/mkx/internal/domain"
)

// filtered returns the items whose visible fields, concatenated and lowercased,
// contain text. Matching is substring, not fuzzy, per @UI Patterns.
//
// The extractor closure is what keeps this one helper rather than a
// filteredTargets() per view: a caller names the fields its rows actually show,
// and nothing here knows what a Target is.
func filtered[T any](items []T, text string, fields func(T) []string) []T {
	if text == "" {
		return items
	}

	needle := strings.ToLower(text)
	out := make([]T, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(strings.Join(fields(item), " ")), needle) {
			out = append(out, item)
		}
	}
	return out
}

// filterState is filter mode's whole state. Its zero value is "no filter in
// effect", so a Model needs no wiring to start unfiltered.
//
// active and text are independent: Enter closes the mode while keeping the
// text, which is the state that keeps the list filtered and the bar on screen.
type filterState struct {
	active bool
	text   string
}

// The mutators live on Model, not on filterState, because the "cursor resets on
// every text change" invariant spans two structs — m.filter.text and
// m.targetCursor. setFilterText is the single place that enforces it, and every
// other mutator routes through it.

// activateFilter enters filter mode from scratch: empty text, cursor at the
// top. `/` never resumes a previous filter.
func (m Model) activateFilter() Model {
	m.filter.active = true
	return m.setFilterText("")
}

// setFilterText is the one place filter text changes, and therefore the one
// place the cursor resets.
func (m Model) setFilterText(s string) Model {
	m.filter.text = s
	m.targetCursor = 0
	return m
}

// appendFilterRunes appends rs to the filter text, dropping control runes.
//
// The stripping is here rather than at either call site because both of them
// go through this one function: the tier-2 rune capture and the batch path in
// batch.go. A bracketed paste carries every rune between the markers, so a
// two-line clipboard entry brings \r and \n with it, and routed into the
// filter bar they render as garbage that matches nothing.
//
// Space is 0x20 and survives — target descriptions are full of them, and the
// capture accepts KeySpace precisely so a filter can contain one.
func (m Model) appendFilterRunes(rs []rune) Model {
	var b strings.Builder
	b.WriteString(m.filter.text)
	for _, r := range rs {
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return m.setFilterText(b.String())
}

// backspaceFilter deletes the last rune, not the last byte: a multi-byte rune
// would otherwise be cut into invalid UTF-8. On empty text it is a no-op.
func (m Model) backspaceFilter() Model {
	rs := []rune(m.filter.text)
	if len(rs) == 0 {
		return m
	}
	return m.setFilterText(string(rs[:len(rs)-1]))
}

// clearFilter drops both the mode and the text — Esc's effect in either of the
// two states where a filter is in effect.
func (m Model) clearFilter() Model {
	m.filter.active = false
	return m.setFilterText("")
}

// filteredTargets is the single computation point for the visible target list.
// The renderer, the cursor bound check and the enter handler all call it, so no
// call site branches on "is a filter active" and no derived field can go stale.
func (m Model) filteredTargets(proj domain.Project) []domain.Target {
	return filtered(proj.Targets, m.filter.text, func(t domain.Target) []string {
		return []string{t.Name, t.Description}
	})
}
