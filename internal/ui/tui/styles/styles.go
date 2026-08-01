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
)
