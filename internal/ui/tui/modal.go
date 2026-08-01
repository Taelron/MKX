package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

// modalContent identifies which content type a modal is showing. The frame —
// centring, border, dimming, key interception — is shared; only the content
// area varies.
type modalContent int

const (
	modalNone modalContent = iota
	modalHelp
	// The M2 branch picker lands here, and costs one enum value, one
	// renderModalBody case, one keymap func and one showModal call.
)

// modalInput is the cursor and text state interactive content types need. Info
// content (the help overlay) leaves it zero.
type modalInput struct {
	cursor int
	text   string
}

// modalState is the modal overlay's whole state. Its zero value is inactive,
// so a Model needs no wiring to start without a modal.
type modalState struct {
	active  bool
	content modalContent
	title   string
	// context is right-aligned in the header: the view name, "Step 1/3", …
	context string
	// keys are the modal's own bindings. They intercept all dispatch while it
	// is active, and they render as the modal's hint bar — which is why the
	// help overlay can show Esc/Close  ?/Close where an interactive content
	// type shows the prescribed Enter/Confirm  Esc/Cancel.
	keys  keymap
	input modalInput
}

// showModal opens a modal over the current view.
func (m Model) showModal(c modalContent, title, context string, in modalInput, k keymap) Model {
	m.modal = modalState{
		active:  true,
		content: c,
		title:   title,
		context: context,
		keys:    k,
		input:   in,
	}
	return m
}

// closeModal returns to the underlying view, which was never modified while
// the modal was up.
func (m Model) closeModal() Model {
	m.modal = modalState{}
	return m
}

// modalMaxInnerWidth is the widest content area a modal may use: ~60% of the
// terminal, less the border (2) and the horizontal padding (2).
func (m Model) modalMaxInnerWidth() int {
	w := m.width
	if w < 20 {
		w = 80
	}
	return max(w*60/100-4, modalMinInnerWidth)
}

const modalMinInnerWidth = 20

// clampWidth truncates s to w display columns, marking the cut with an
// ellipsis. Every line inside a modal goes through it: lipgloss word-wraps
// rather than truncates when a line exceeds the style's width, which would
// silently change the box's height.
func clampWidth(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.TruncateWc(s, w, "…")
}

// buildModalBox renders the frame around a body:
//
//	╭─────────────────────────────────╮
//	│ Title                   Context │
//	│─────────────────────────────────│
//	│ [body]                          │
//	│─────────────────────────────────│
//	│ key/Action  key/Action          │
//	╰─────────────────────────────────╯
//
// The inner width is the widest of header, body and hint bar, capped at
// maxInner. Body lines wider than that truncate with an ellipsis; there is
// deliberately no height clamp, because modals never scroll — content that
// cannot fit is the wrong pattern.
func buildModalBox(title, context, body, hints string, maxInner int) string {
	header := title
	if context != "" {
		header = title + "  " + context
	}

	bodyLines := strings.Split(body, "\n")

	inner := lipgloss.Width(header)
	for _, line := range bodyLines {
		inner = max(inner, lipgloss.Width(line))
	}
	inner = max(inner, lipgloss.Width(hints))
	inner = min(max(inner, modalMinInnerWidth), maxInner)

	// Header and hint bar are clamped to inner just as body lines are. Without
	// this, lipgloss word-wraps the over-wide line inside the border — a long
	// project name in the header would split across two rows and push
	// everything below it down. A deeply nested project path reaches this at
	// 80 columns, where inner is only 44.
	title = clampWidth(title, inner)
	if context != "" {
		if room := inner - lipgloss.Width(title) - 2; room < 1 {
			context = ""
		} else {
			context = clampWidth(context, room)
		}
	}
	hints = clampWidth(hints, inner)

	// Header: title left, context hard right.
	styledHeader := styles.ModalTitle.Render(title)
	if context != "" {
		gap := max(inner-lipgloss.Width(title)-lipgloss.Width(context), 1)
		styledHeader += strings.Repeat(" ", gap) + styles.ModalContext.Render(context)
	}

	rule := styles.ModalRule.Render(strings.Repeat("─", inner))

	for i, line := range bodyLines {
		bodyLines[i] = styles.ModalBody.Render(clampWidth(line, inner))
	}

	parts := make([]string, 0, len(bodyLines)+4)
	parts = append(parts, styledHeader, rule)
	parts = append(parts, bodyLines...)
	parts = append(parts, rule, hints)

	// Width is set on the padded content area, so the outer box comes to
	// inner + 2 padding + 2 border columns.
	return styles.ModalBorder.Width(inner + 2).Render(strings.Join(parts, "\n"))
}

// renderModalBody renders the content area for the active content type. Each
// new modal adds exactly one case here.
func (m Model) renderModalBody() string {
	switch m.modal.content {
	case modalHelp:
		// The underlying view's keymap, not the modal's own — that is what
		// makes help context-sensitive.
		return renderHelpBody(m.viewKeymap())
	}
	return ""
}

// renderModal composites the modal over an already-rendered view: dim the
// background, centre the box, splice it on.
//
// Lipgloss v1 has no Canvas/Layer, and lipgloss.Place centres onto a
// whitespace canvas that would overwrite the dimmed background, so the offsets
// are computed explicitly. @UI Patterns specifies the outcome — a centred,
// bordered modal over a visibly dimmed view — and leaves the mechanic to the
// product.
func (m Model) renderModal(base string) string {
	box := buildModalBox(
		m.modal.title,
		m.modal.context,
		m.renderModalBody(),
		m.modal.keys.hintBar(),
		m.modalMaxInnerWidth(),
	)

	x := max((m.width-lipgloss.Width(box))/2, 0)
	y := max((m.height-lipgloss.Height(box))/2, 0)

	return splice(dimBackground(base), box, x, y)
}
