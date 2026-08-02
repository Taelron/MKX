package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Gaetan-Jaminon/mkx/internal/ui/tui/styles"
)

// fitHintBar renders a view's hint entries into avail display columns on
// exactly one line.
//
// The bar used to be handed to lipgloss at the terminal's width and left there.
// lipgloss word-wraps rather than truncates, so a bar that did not fit became
// two rows — which changes the view's rendered height and breaks
// height-sensitive tests for reasons that look unrelated to width. That is
// TAE-60. A bar must degrade legibly instead, and legibly means the user can
// still see that something was dropped and still reach the full list.
//
// avail is the bar's content width: the terminal width less styles.HintBar's
// Padding(0,1). Callers pass w-2; the function knows nothing about Model.
//
// # The claim order
//
// Width is claimed in this order, and each claimant only sees what the ones
// above it left:
//
//  1. pinned entries — the view's exits, Esc/Back and ?/Help
//  2. the flash — truncated with an ellipsis if it alone exceeds what remains
//  3. droppable entries, left to right
//  4. the +N marker, at the position of the first omitted entry
//
// So a long `Pull failed: <git error>` costs the user their hints rather than
// wrapping the bar, and never costs them the ? that lists the hints again. The
// hints are recoverable; the error text is not. That trade is why the flash
// outranks the droppables — and it is the point at which MkX rendering the
// flash *inside* the bar stops being incidental and becomes structural. See
// @MkX Design Notes, "The flash message renders inside the hint bar".
//
// # Why the marker's cost is inside the fit
//
// The largest surviving-entry count is chosen exactly: every candidate below is
// measured with its marker already in it, rather than appending the marker to a
// set that was sized without it. Sizing first and marking after would overflow
// the very width the fit was for.
//
// # The floor
//
// Below the width that holds the pinned set the marker yields, then every
// entry but the last pinned one, and finally clampWidth cuts what is left. The
// result is one line at every width, including widths the product does not
// support — renderHintBar substitutes 80 columns below 20, matching the rest of
// the view, so the narrow end here is a floor rather than a rendering path.
func fitHintBar(entries []hintEntry, avail int, flash string) string {
	if avail <= 0 {
		// Two columns of terminal are two columns of padding: there is no
		// content column left to degrade into.
		return ""
	}

	// Flattened before it is measured, because lipgloss.Width reports the
	// widest line rather than the total: a flash carrying a newline measures
	// short, fits every candidate, and is returned still carrying it. The bar
	// then renders on two rows at any terminal width — the same silent height
	// change this function exists to prevent, reached through the message
	// instead of the width.
	//
	// No flash source can produce one today: gitx already takes firstLine() of
	// captured stderr, and the %q verbs elsewhere escape a newline rather than
	// emitting it. This keeps the guarantee a property of the fit rather than
	// of every present and future caller remembering.
	flash = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(flash)

	if len(entries) == 0 {
		return clampWidth(styles.Flash.Render(flash), avail)
	}

	// Candidate 1: everything, with the flash untouched. The prescribed formats
	// are byte-identical here, and no +N token appears — which is what keeps
	// TestHintBarsMatchTheBaselineFormats meaningful at and above 80 columns.
	keep := make([]bool, len(entries))
	for i := range keep {
		keep[i] = true
	}
	if s, w := assembleHintBar(entries, keep, false, flash); w <= avail {
		return s
	}

	// Something will be omitted, so a marker is now part of every candidate.
	// The flash is truncated to what the pinned entries and that marker leave,
	// before the droppables get to claim anything.
	flash = clampFlash(entries, avail, flash)

	var pinned, droppable []int
	for i, e := range entries {
		if e.pinned {
			pinned = append(pinned, i)
		} else {
			droppable = append(droppable, i)
		}
	}

	// Candidates 2..n: keep every pinned entry, and the droppables left to
	// right, shedding one more from the right each round. Widths decrease
	// monotonically, so the first that fits is the widest that fits.
	for n := len(droppable) - 1; n >= 0; n-- {
		keep = keepSet(len(entries), pinned, droppable[:n])
		if s, w := assembleHintBar(entries, keep, true, flash); w <= avail {
			return s
		}
	}

	// Below that the pinned set itself no longer fits. It sheds from the left,
	// so the last pinned entry — ?/Help, the way back to everything dropped —
	// is the last to go. A degradation that removes the means of discovering
	// what was removed is the wrong degradation.
	for n := len(pinned) - 1; n >= 1; n-- {
		keep = keepSet(len(entries), pinned[len(pinned)-n:], nil)
		if s, w := assembleHintBar(entries, keep, true, flash); w <= avail {
			return s
		}
	}

	// The marker is the last thing to yield before the survivor itself: a lone
	// `+7` says nothing a user can act on, while a lone `?/Help` does.
	last := len(entries) - 1
	if len(pinned) > 0 {
		last = pinned[len(pinned)-1]
	}
	keep = keepSet(len(entries), []int{last}, nil)
	if s, w := assembleHintBar(entries, keep, false, ""); w <= avail {
		return s
	}

	// The hard clamp. One line, ending in an ellipsis, at any width at all.
	s, _ := assembleHintBar(entries, keep, false, "")
	return clampWidth(s, avail)
}

// keepSet builds the survivor mask for n entries from the indices that survive.
func keepSet(n int, groups ...[]int) []bool {
	keep := make([]bool, n)
	for _, g := range groups {
		for _, i := range g {
			keep[i] = true
		}
	}
	return keep
}

// assembleHintBar lays the surviving entries out in display order and returns
// the styled bar with its display width.
//
// The width is measured from the unstyled text, not from the styled string:
// lipgloss emits no escape sequences under `go test`, where there is no TTY, so
// measuring the render would measure two different things in two contexts.
//
// marker asks for the +N overflow token at the position of the first omitted
// entry — one token however many separate runs are missing, because N counts
// entries rather than gaps.
func assembleHintBar(entries []hintEntry, keep []bool, marker bool, flash string) (string, int) {
	omitted := 0
	for _, k := range keep {
		if !k {
			omitted++
		}
	}

	styled := make([]string, 0, len(entries)+2)
	plain := make([]string, 0, len(entries)+2)
	placed := false

	for i, e := range entries {
		if keep[i] {
			styled = append(styled, e.render())
			plain = append(plain, e.text())
			continue
		}
		if marker && !placed {
			placed = true
			token := overflowMarker(omitted)
			styled = append(styled, styles.HintOverflow.Render(token))
			plain = append(plain, token)
		}
	}

	if flash != "" {
		styled = append(styled, styles.Flash.Render(flash))
		plain = append(plain, flash)
	}

	return strings.Join(styled, hintBarSep), lipgloss.Width(strings.Join(plain, hintBarSep))
}

// overflowMarker is the token standing in for the entries that did not fit.
func overflowMarker(n int) string { return fmt.Sprintf("+%d", n) }

// clampFlash truncates the flash to the columns left over once the pinned
// entries and the overflow marker have taken theirs — step 2 of the claim
// order, run before any droppable entry is considered.
//
// The marker is sized for the largest count this bar could show, so the
// reservation can never be short by a digit. A flash with no room left is
// dropped rather than rendered as a bare ellipsis, which would occupy a column
// while saying nothing.
func clampFlash(entries []hintEntry, avail int, flash string) string {
	if flash == "" {
		return ""
	}

	// The bar the flash is being fitted against: the marker, every pinned
	// entry, then the flash. That is tokens+1 separators.
	tokens := 1 // the marker
	room := avail - lipgloss.Width(overflowMarker(len(entries)))
	for _, e := range entries {
		if !e.pinned {
			continue
		}
		tokens++
		room -= lipgloss.Width(e.text())
	}
	room -= tokens * len(hintBarSep)

	if room < 1 {
		return ""
	}
	return clampWidth(flash, room)
}
