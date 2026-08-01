package tui

import (
	"strings"
)

// renderHelpBody renders the help overlay's content: the keys available in the
// view the overlay was opened over, as "key — action" rows.
//
// It takes the view's own keymap rather than a global reference, which is what
// makes help context-sensitive per @UI Patterns. Nothing else derives the rows,
// so a key can neither appear here without a handler nor be handled without
// appearing here.
//
// Rows are not clamped: buildModalBox owns the width, and truncating twice
// would just hide which one did it.
func renderHelpBody(k keymap) string {
	return strings.Join(k.helpRows(), "\n")
}
