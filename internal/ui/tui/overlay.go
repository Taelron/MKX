package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

// dimBackground re-renders an already-rendered view so it reads as background
// behind a modal.
//
// Each line is stripped of its own ANSI attributes and re-rendered with a
// single dim foreground. Dropping the zebra and cursor backgrounds is
// deliberate: they would compete with the modal for attention, and @UI Patterns
// asks for "muted colors, or an overlay effect" without mandating a mechanism.
// The table keeps its shape as a grey ghost.
func dimBackground(view string) string {
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = styles.Dimmed.Render(ansi.Strip(line))
	}
	return strings.Join(lines, "\n")
}

// splice overwrites fg's lines onto bg starting at column x, row y, and returns
// the result. The background's line count is never changed: fg lines that would
// fall past the bottom are dropped rather than extending the view.
//
// Widths are measured with lipgloss.Width throughout so ANSI sequences are
// discounted — the same discipline borderedRow follows.
func splice(bg, fg string, x, y int) string {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		row := y + i
		if row >= len(bgLines) {
			break
		}
		bgLine := bgLines[row]

		// The background's first x columns, padded out when the line is short.
		//
		// TruncateWc/TruncateLeftWc rather than CutWc: in x/ansi v0.10.2 CutWc's
		// left-cut path is bound to a right-truncating helper and misbehaves on
		// wide runes.
		left := ansi.TruncateWc(bgLine, x, "")
		if pad := x - lipgloss.Width(left); pad > 0 {
			left += strings.Repeat(" ", pad)
		}
		if strings.ContainsRune(left, ansiEscape) {
			// The cut may have severed the line mid-sequence; close it so the
			// background's colour cannot bleed into the modal.
			left += ansiReset
		}

		right := ansi.TruncateLeftWc(bgLine, x+lipgloss.Width(fgLine), "")

		bgLines[row] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}

const (
	ansiEscape = '\x1b'
	ansiReset  = "\x1b[0m"
)
