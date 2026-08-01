package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestSplice(t *testing.T) {
	tests := []struct {
		name string
		bg   string
		fg   string
		x, y int
		want string
	}{
		{
			name: "modal narrower than background",
			bg:   "..........\n..........\n..........",
			fg:   "AB\nCD",
			x:    4,
			y:    1,
			want: "..........\n....AB....\n....CD....",
		},
		{
			name: "modal at the left edge",
			bg:   "..........\n..........",
			fg:   "XY",
			x:    0,
			y:    0,
			want: "XY........\n..........",
		},
		{
			name: "background line shorter than x is padded",
			bg:   "ab\n..........",
			fg:   "MM",
			x:    5,
			y:    0,
			want: "ab   MM\n..........",
		},
		{
			name: "modal wider than the background line",
			bg:   "..\n..",
			fg:   "WIDE",
			x:    0,
			y:    0,
			want: "WIDE\n..",
		},
		{
			name: "rows past the bottom are dropped, not appended",
			bg:   "..........",
			fg:   "A\nB\nC",
			x:    2,
			y:    0,
			want: "..A.......",
		},
		{
			name: "negative offsets clamp to the origin",
			bg:   "....\n....",
			fg:   "Z",
			x:    -3,
			y:    -2,
			want: "Z...\n....",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splice(tt.bg, tt.fg, tt.x, tt.y); got != tt.want {
				t.Errorf("splice() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// A CJK background is the case CutWc gets wrong; assert the visible width
// survives the splice unchanged.
//
// Every offset, not just one: at an odd x both edges of the modal land inside a
// double-width rune, and the two truncators disagree about what to do with a
// straddler. An even-offset-only test passes while the row silently grows by a
// column.
func TestSpliceWideRuneBackground(t *testing.T) {
	bg := strings.Repeat("日", 10) // 20 display columns

	for x := 0; x <= 8; x++ {
		got := splice(bg, "AB", x, 0)

		if w := lipgloss.Width(got); w != 20 {
			t.Errorf("x=%d: spliced width = %d, want 20 (%q)", x, w, ansi.Strip(got))
		}
		if !strings.Contains(got, "AB") {
			t.Errorf("x=%d: spliced line lost the foreground: %q", x, got)
		}
	}
}

// The whole point of the width discipline: a modal spliced over a wide-rune
// background must stay rectangular, or its border steps sideways row by row.
func TestSpliceKeepsAWideRuneBackgroundRectangular(t *testing.T) {
	row := strings.Repeat("日", 10)
	bg := strings.Join([]string{row, row, row, row}, "\n")
	fg := "┌──┐\n│ab│\n└──┘"

	out := splice(bg, fg, 3, 0)

	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w != 20 {
			t.Errorf("row %d width = %d, want 20: %q", i, w, ansi.Strip(line))
		}
	}
}

// The modal must not inherit the dimmed background's colour, and the
// background either side of the splice must keep its own.
func TestSpliceANSIBackground(t *testing.T) {
	// Written as raw SGR rather than via lipgloss: lipgloss suppresses colour
	// when the test binary's output is not a terminal, which would make this
	// assertion silently vacuous.
	bg := "\x1b[31m" + strings.Repeat(".", 20) + "\x1b[0m"

	got := splice(bg, "MM", 8, 0)

	if w := lipgloss.Width(got); w != 20 {
		t.Errorf("spliced width = %d, want 20", w)
	}
	const want = "........MM.........."
	if plain := ansi.Strip(got); plain != want {
		t.Errorf("stripped result = %q, want %q", plain, want)
	}
	before, _, _ := strings.Cut(got, "MM")
	if !strings.HasSuffix(before, ansiReset) {
		t.Errorf("foreground is not insulated from the background's colour: %q", before)
	}
}

func TestDimBackgroundStripsAttributesAndPreservesShape(t *testing.T) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Background(lipgloss.Color("24")).
		Bold(true)
	view := style.Render("row one") + "\n" + style.Render("row two")

	got := dimBackground(view)

	if lines := strings.Count(got, "\n"); lines != 1 {
		t.Errorf("dimBackground changed the line count: got %d newlines, want 1", lines)
	}
	for i, line := range strings.Split(got, "\n") {
		if w := lipgloss.Width(line); w != 7 {
			t.Errorf("line %d width = %d, want 7", i, w)
		}
	}
	if plain := ansi.Strip(got); plain != "row one\nrow two" {
		t.Errorf("dimBackground changed the text: %q", plain)
	}
	if strings.Contains(got, "\x1b[1m") {
		t.Error("dimBackground kept the bold attribute from the source view")
	}
}
