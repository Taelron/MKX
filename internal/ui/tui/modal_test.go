package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestBuildModalBoxStructure(t *testing.T) {
	box := buildModalBox("Help", "Targets", "one\ntwo", "Esc/Close", 44)
	lines := strings.Split(ansi.Strip(box), "\n")

	// border, header, rule, 2 body, rule, hints, border
	if len(lines) != 8 {
		t.Fatalf("box has %d lines, want 8:\n%s", len(lines), ansi.Strip(box))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Errorf("top border is not rounded: %q", lines[0])
	}
	if !strings.HasPrefix(lines[7], "╰") || !strings.HasSuffix(lines[7], "╯") {
		t.Errorf("bottom border is not rounded: %q", lines[7])
	}
	if !strings.Contains(lines[1], "Help") || !strings.Contains(lines[1], "Targets") {
		t.Errorf("header is missing the title or the context: %q", lines[1])
	}
	if !strings.Contains(lines[6], "Esc/Close") {
		t.Errorf("hint bar row is missing the hints: %q", lines[6])
	}
}

func TestBuildModalBoxIsRectangular(t *testing.T) {
	box := buildModalBox("Help", "Targets", "short\na much longer body line", "Esc/Close", 44)

	lines := strings.Split(box, "\n")
	want := lipgloss.Width(lines[0])
	for i, line := range lines {
		if got := lipgloss.Width(line); got != want {
			t.Errorf("line %d width = %d, want %d: %q", i, got, want, ansi.Strip(line))
		}
	}
}

// The context is right-aligned in the header, per the UI Patterns box
// structure.
func TestBuildModalBoxRightAlignsTheContext(t *testing.T) {
	box := buildModalBox("Help", "Targets", "body", "Esc/Close", 44)
	header := strings.Split(ansi.Strip(box), "\n")[1]

	if !strings.HasSuffix(strings.TrimRight(header, " │"), "Targets") {
		t.Errorf("context is not flush right in the header: %q", header)
	}
}

func TestBuildModalBoxCapsTheInnerWidth(t *testing.T) {
	const maxInner = 44
	box := buildModalBox("Help", "", strings.Repeat("x", 200), "Esc/Close", maxInner)

	// inner + 2 padding + 2 border
	if got := lipgloss.Width(box); got != maxInner+4 {
		t.Errorf("box width = %d, want %d", got, maxInner+4)
	}
	if !strings.Contains(ansi.Strip(box), "…") {
		t.Error("an over-wide body line was not truncated with an ellipsis")
	}
}

func TestBuildModalBoxHasAMinimumWidth(t *testing.T) {
	box := buildModalBox("H", "", "x", "y", 44)

	if got := lipgloss.Width(box); got != modalMinInnerWidth+4 {
		t.Errorf("narrow box width = %d, want %d", got, modalMinInnerWidth+4)
	}
}

func TestModalMaxInnerWidthIsSixtyPercent(t *testing.T) {
	// 80 columns → 48 outer, 44 inner.
	if got := (Model{width: 80}).modalMaxInnerWidth(); got != 44 {
		t.Errorf("modalMaxInnerWidth() at 80 columns = %d, want 44", got)
	}
	// An unset width falls back to 80, as the list renderers do.
	if got := (Model{}).modalMaxInnerWidth(); got != 44 {
		t.Errorf("modalMaxInnerWidth() with no size = %d, want 44", got)
	}
}

func TestShowModalAndCloseModal(t *testing.T) {
	m := Model{}

	m = m.showModal(modalHelp, "Help", "Projects", modalInput{}, keymap{
		{keys: []string{"esc"}, display: "Esc", label: "Close", inBar: true},
	})
	if !m.modal.active {
		t.Fatal("showModal did not activate the modal")
	}
	if m.modal.content != modalHelp || m.modal.title != "Help" || m.modal.context != "Projects" {
		t.Errorf("showModal did not carry the content type, title and context: %+v", m.modal)
	}

	m = m.closeModal()
	if m.modal.active {
		t.Error("closeModal left the modal active")
	}
	if m.modal.content != modalNone {
		t.Errorf("closeModal left content = %v, want modalNone", m.modal.content)
	}
}

// The composite: the box sits centred over a background that is still visible
// but no longer carries its own attributes.
func TestRenderModalCompositesOverADimmedBackground(t *testing.T) {
	base := strings.TrimSuffix(strings.Repeat(strings.Repeat("·", 80)+"\n", 23), "\n")

	m := Model{width: 80, height: 24}
	m = m.showModal(modalHelp, "Help", "Targets", modalInput{}, keymap{
		{keys: []string{"esc"}, display: "Esc", label: "Close", inBar: true},
	})

	out := ansi.Strip(m.renderModal(base))
	lines := strings.Split(out, "\n")

	if len(lines) != 23 {
		t.Fatalf("composite has %d lines, want the background's 23", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 80 {
			t.Errorf("line %d width = %d, want 80", i, got)
		}
	}

	// The background survives either side of the box on the rows it covers.
	var boxRow int
	for i, line := range lines {
		if strings.Contains(line, "╭") {
			boxRow = i
		}
	}
	if boxRow == 0 {
		t.Fatal("no top border found in the composite")
	}
	if !strings.HasPrefix(lines[boxRow], "·") || !strings.HasSuffix(lines[boxRow], "·") {
		t.Errorf("the box overwrote the background either side of itself: %q", lines[boxRow])
	}

	// Centred: equal background either side, to within a column.
	left := len(lines[boxRow]) - len(strings.TrimLeft(lines[boxRow], "·"))
	right := len(lines[boxRow]) - len(strings.TrimRight(lines[boxRow], "·"))
	if left-right > 1 || right-left > 1 {
		t.Errorf("box is not centred: %d columns left, %d right", left, right)
	}
}

func TestViewRendersTheBaseWhenNoModalIsActive(t *testing.T) {
	m := Model{width: 80, height: 24}

	if strings.Contains(m.View(), "╭") {
		t.Error("View() drew a modal frame with no modal active")
	}
}
