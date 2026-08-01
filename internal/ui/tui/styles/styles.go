// Package styles holds every Lipgloss style MkX renders with, as named
// semantic variables. Views reference these rather than defining their own,
// per the TUI Go Conventions baseline.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	// colors
	colorBlue   = lipgloss.Color("33")
	colorCyan   = lipgloss.Color("86")
	colorGreen  = lipgloss.Color("42")
	colorRed    = lipgloss.Color("196")
	colorYellow = lipgloss.Color("220")
	colorDimmed = lipgloss.Color("241")
	colorSubtle = lipgloss.Color("236")
	colorWhite  = lipgloss.Color("255")
	colorBorder = lipgloss.Color("238")

	// Header is the title in the top bar.
	Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorBlue).
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	// HeaderCount is the position indicator in the top bar.
	HeaderCount = lipgloss.NewStyle().
			Foreground(colorDimmed).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	// ColumnHeader is the table's column-name row.
	ColumnHeader = lipgloss.NewStyle().
			Foreground(colorBlue).
			Bold(true).
			Background(lipgloss.Color("238"))

	// SelectedRow is the row under the cursor.
	SelectedRow = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true).
			Background(lipgloss.Color("24"))

	// NormalRow is an odd table row.
	NormalRow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250"))

	// AltRow is an even table row, banded against NormalRow.
	AltRow = lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Background(lipgloss.Color("234"))

	// Cursor is the ▸ marker on the selected row.
	Cursor = lipgloss.NewStyle().
		Foreground(colorCyan).
		Bold(true)

	// Border is the table frame.
	Border = lipgloss.NewStyle().
		Foreground(colorBorder)

	// FilterBar is the `Filter: {text}` line below the top border. Cyan, per
	// the @UI Patterns colour conventions, which give cyan to filter text.
	FilterBar = lipgloss.NewStyle().
			Foreground(colorCyan)

	// The git segment in the target view's header. Four styles for seven
	// states, and the split is deliberate: @UI Patterns' colour conventions
	// give red to failure, green to success, yellow to attention-not-failure,
	// and dimmed to the merely muted. TAE-58 requires unknown, absent and
	// clean to be distinguishable from each other, so those three get three
	// different colours rather than three different words in one colour.
	//
	// All four carry the header's background, so the segment reads as part of
	// the top bar rather than as text floating over it.
	//
	// Blue is deliberately unused here — it belongs to the adjacent header
	// title, and reusing it would visually merge the two.

	// GitClean is a repository with no uncommitted changes.
	GitClean = lipgloss.NewStyle().
			Foreground(colorGreen).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	// GitAttention is a repository wanting a second look but not in error —
	// an uncommitted change, or a detached head.
	GitAttention = lipgloss.NewStyle().
			Foreground(colorYellow).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	// GitUnknown is a read that failed or timed out. Red, because a failed
	// read is a failure — and because it must not be mistakable for the
	// dimmed "there is no repository here", which is not.
	GitUnknown = lipgloss.NewStyle().
			Foreground(colorRed).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	// GitPending is the muted set: a read in flight, a project in no
	// repository, and a repository with no commits yet. None of the three is
	// an error condition, and none is rendered as one.
	GitPending = lipgloss.NewStyle().
			Foreground(colorDimmed).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	// HintKey is the key name in the bottom hint bar.
	HintKey = lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true)

	// HintAction is the action label in the bottom hint bar.
	HintAction = lipgloss.NewStyle().
			Foreground(colorDimmed)

	// HintBar is the bottom bar itself.
	HintBar = lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Padding(0, 1)

	// Flash is a transient success message in the hint bar.
	Flash = lipgloss.NewStyle().
		Foreground(colorGreen).
		Bold(true)

	// FlashError is a transient failure message in the hint bar.
	FlashError = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	// ModalBorder is the rounded frame around a modal overlay. Blue, per the
	// UI Patterns colour conventions, so it separates from the dimmed view
	// behind it.
	ModalBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Padding(0, 1)

	// ModalTitle is the modal header's left-hand title.
	ModalTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorBlue)

	// ModalContext is the modal header's right-aligned context — the view
	// name, a "Step 1/3" counter, and so on.
	ModalContext = lipgloss.NewStyle().
			Foreground(colorDimmed)

	// ModalBody is the modal's content area.
	ModalBody = lipgloss.NewStyle().
			Foreground(colorWhite)

	// ModalRule is the horizontal rule separating a modal's header, body and
	// hint bar.
	ModalRule = lipgloss.NewStyle().
			Foreground(colorBorder)

	// Dimmed renders the underlying view while a modal is open: a single
	// muted foreground, with the row backgrounds dropped so the modal keeps
	// the eye.
	Dimmed = lipgloss.NewStyle().
		Foreground(colorDimmed)
)
