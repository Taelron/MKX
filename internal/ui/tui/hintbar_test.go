package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The width table. TAE-60's deliverable is a bar that degrades legibly instead
// of wrapping, and "legibly" is not something a boolean captures — so every
// subtest below logs the bar it produced. `make verify-hintbar-widths` prints
// eight real bars to read against the plan's computed table, rather than a
// green tick that says only that nothing crashed.
//
// Why these widths:
//
//	200  a normal terminal — degradation must never trigger here
//	 80  the supported minimum per @UI Patterns; the target bar is 78 of 78
//	 79  the exact boundary — one column short, the first entry drops
//	 60  a plausible split pane
//	 40  the width the issue names as the stress case
//	 20  the pinned set no longer fits: Esc/Back yields, ?/Help survives
//	 10  the marker yields; ?/Help alone
//	  2  the hard clamp — two columns of terminal are two columns of padding
//
// 10 and 2 are below the 20 columns at which renderHintBar substitutes 80, so
// they exercise fitHintBar directly. They pin the floor of the pure function
// regardless of what the render path does with narrow terminals.
var hintBarWidths = []int{200, 80, 79, 60, 40, 20, 10, 2}

// The target view's bar at each width, exactly as the approved plan computed
// it. Pinned because a degradation that is merely *a* degradation is not the
// deliverable: which entries yield, in which order, and where the marker lands
// are the product decision. A diff here is a changed decision, and should read
// as one.
var targetBarByWidth = map[int]string{
	200: "↑↓/Navigate  //Search  Enter/Run  b/Branch  g/Pull  R/Readme  Esc/Back  ?/Help",
	80:  "↑↓/Navigate  //Search  Enter/Run  b/Branch  g/Pull  R/Readme  Esc/Back  ?/Help",
	79:  "↑↓/Navigate  //Search  Enter/Run  b/Branch  g/Pull  +1  Esc/Back  ?/Help",
	60:  "↑↓/Navigate  //Search  Enter/Run  +3  Esc/Back  ?/Help",
	40:  "↑↓/Navigate  +5  Esc/Back  ?/Help",
	20:  "+7  ?/Help",
	10:  "?/Help",
	2:   "",
}

var overflowToken = regexp.MustCompile(`\+(\d+)`)

// barAt returns k's bar, unstyled, as it reaches the screen at terminal width
// w.
//
// At and above 20 columns that means the real render path, m.renderHintBar —
// not fitHintBar behind its back. Which is load-bearing twice over: these
// subtests then cover the wiring as well as the fit, and verify-hintbar-guard.sh
// can revert renderHintBar in a throwaway copy and require exactly these
// subtests to go red.
//
// Below 20 columns renderHintBar substitutes 80, matching the view body above
// it, so w=10 and w=2 call the fitter directly. They pin the pure function's
// floor rather than a rendering path the product has.
func barAt(k keymap, w int, flash string) string {
	if w < 20 {
		return ansi.Strip(fitHintBar(k.barEntries(), w-2, flash))
	}

	m := testModel()
	m.width = w
	m.flash = flash
	// Trimmed of styles.HintBar's Padding(0,1) and of the Width(w) fill. A
	// wrapped bar keeps its newline through this, which is the point.
	return strings.TrimSpace(ansi.Strip(m.renderHintBar(k)))
}

// TestHintBarDegradesAcrossTheWidthTable is the issue's core claim, asserted at
// every width for both views.
func TestHintBarDegradesAcrossTheWidthTable(t *testing.T) {
	for name, k := range viewKeymaps() {
		entries := k.barEntries()

		for _, w := range hintBarWidths {
			t.Run(fmt.Sprintf("%s/w=%d", name, w), func(t *testing.T) {
				bar := barAt(k, w, "")
				t.Logf("w=%-3d %2d cols │%s│", w, lipgloss.Width(bar), bar)

				assertOneLine(t, bar)
				assertFitsWithin(t, bar, w)
				assertHelpSurvives(t, bar, w)
				assertMarkerCountIsHonest(t, bar, entries, w)
				assertPinnedYieldLast(t, bar, entries)
				assertDroppablesGoRightToLeft(t, bar, entries)
			})
		}
	}
}

// TestTargetBarMatchesTheComputedTable pins the exact output the plan derived.
func TestTargetBarMatchesTheComputedTable(t *testing.T) {
	k := targetKeymap()

	for _, w := range hintBarWidths {
		t.Run(fmt.Sprintf("w=%d", w), func(t *testing.T) {
			want := targetBarByWidth[w]
			got := barAt(k, w, "")
			t.Logf("w=%-3d %2d cols │%s│", w, lipgloss.Width(got), got)

			if got != want {
				t.Errorf("the target bar degraded differently from the approved plan\n"+
					"got  %q\nwant %q\n"+
					"If the change to the entries or to the fit order is deliberate, "+
					"recompute this row rather than relaxing the assertion.", got, want)
			}
		})
	}
}

// TestFullViewHeightIsWidthInvariant is what the issue is actually about.
//
// A wrapped bar still renders. The failure is silent, and it surfaces as a view
// that is one row taller than every height-sensitive test expects — for a
// reason no line in those failures points at. So the assertion is not "the bar
// is one line" but "the view is the same number of lines at every width", which
// is the property the rest of the suite depends on.
func TestFullViewHeightIsWidthInvariant(t *testing.T) {
	cases := map[string]func(Model) Model{
		"targets":  func(m Model) Model { m.view = viewTargets; return m },
		"projects": func(m Model) Model { m.view = viewProjects; m.projectCursor = 0; return m },
	}

	for name, setView := range cases {
		t.Run(name, func(t *testing.T) {
			var want int
			for _, w := range hintBarWidths {
				m := setView(testModel())
				m.width = w
				m.height = 24

				got := strings.Count(m.View(), "\n") + 1
				t.Logf("w=%-3d %d lines", w, got)

				if want == 0 {
					want = got
					continue
				}
				if got != want {
					t.Errorf("the view rendered %d lines at width %d and %d lines at width %d;\n"+
						"a width-dependent height means something wrapped, and every "+
						"height-sensitive test in this package is calibrated against the "+
						"width-independent count", got, w, want, hintBarWidths[0])
				}
			}
		})
	}
}

// A long flash is the live half of TAE-60, not the latent half.
//
// The issue was filed against a bar that would wrap once another binding was
// added. But renderHintBar appended the flash before applying the width, and
// two flash sources are unbounded — runFailedMsg's msg.err.Error() and
// gitPullFinishedMsg's "Pull failed: %v". A git error long enough wraps the bar
// on a terminal far wider than 80, today, with no new binding involved.
//
// 116 columns is where the falsification in the issue description was measured.
func TestALongFlashDoesNotWrapTheBar(t *testing.T) {
	// A real git failure, not a synthetic one: this is the shape that reaches
	// m.flash through "Pull failed: %v".
	const gitErr = "Pull failed: fatal: unable to access " +
		"'https://github.example.com/org/repository.git/': " +
		"Could not resolve host: github.example.com"

	for _, w := range []int{116, 80, 60} {
		t.Run(fmt.Sprintf("w=%d", w), func(t *testing.T) {
			m := testModel()
			m.width = w
			m.flash = gitErr

			bar := ansi.Strip(m.renderHintBar(targetKeymap()))
			t.Logf("w=%-3d %2d cols │%s│", w, lipgloss.Width(bar), bar)

			if n := strings.Count(bar, "\n"); n != 0 {
				t.Errorf("a %d-column flash wrapped the bar to %d lines at width %d",
					lipgloss.Width(gitErr), n+1, w)
			}
			if got := lipgloss.Width(bar); got > w {
				t.Errorf("the bar is %d columns wide at width %d", got, w)
			}
			// The flash outranks the hints, but never the escape hatch: the
			// user must still be able to reach the list the error cost them.
			if !strings.Contains(bar, "?/Help") {
				t.Errorf("a long flash pushed ?/Help out of the bar: %q", bar)
			}
			// And the error is still readable — truncated, not omitted.
			if !strings.Contains(bar, "Pull failed:") {
				t.Errorf("the flash itself was dropped rather than truncated: %q", bar)
			}
		})
	}
}

// A flash carrying a newline must not become a second row.
//
// lipgloss.Width reports the widest line, not the total, so an unflattened
// multi-line flash measures short, fits every candidate untouched, and is
// returned with the newline still in it — a two-row bar on a 200-column
// terminal. Same silent height change as the wrapping this issue was filed
// about, reached through the message rather than the width.
//
// Nothing produces one today: gitx takes firstLine() of captured stderr and the
// %q verbs elsewhere escape newlines. This pins the guarantee to the fit rather
// than to every present and future flash source.
func TestAFlashCarryingANewlineDoesNotBecomeASecondRow(t *testing.T) {
	for _, flash := range []string{"first\nsecond", "first\r\nsecond", "trailing\n"} {
		t.Run(fmt.Sprintf("%q", flash), func(t *testing.T) {
			m := testModel()
			m.width = 200
			m.height = 24
			m.flash = flash

			bar := ansi.Strip(m.renderHintBar(targetKeymap()))
			t.Logf("│%s│", bar)

			if n := strings.Count(bar, "\n"); n != 0 {
				t.Errorf("a flash carrying a newline rendered the bar on %d rows", n+1)
			}

			plain := testModel()
			plain.width, plain.height = 200, 24
			if got, want := strings.Count(m.View(), "\n"), strings.Count(plain.View(), "\n"); got != want {
				t.Errorf("the view is %d lines with the flash and %d without it", got+1, want+1)
			}
		})
	}
}

// The flash claims width after the pinned entries and before the droppable
// ones. A flash too long to sit beside the hints costs the hints, not the bar's
// single line and not the exits.
//
// Measured at 116 columns rather than 80: at the supported minimum the target
// bar is already 78 of 78, so *any* flash costs a hint there and the two halves
// of this claim could not be told apart. 116 is where a run receipt is free and
// a git error is not.
func TestAFlashClaimsWidthAheadOfTheDroppableHints(t *testing.T) {
	k := targetKeymap()

	short := barAt(k, 116, "✓ 2s")
	long := barAt(k, 116, strings.Repeat("x", 60))
	t.Logf("short flash │%s│", short)
	t.Logf("long flash  │%s│", long)

	if strings.Contains(short, "+") {
		t.Errorf("a four-column flash forced a degradation at 116 columns: %q", short)
	}
	if !strings.Contains(long, "+") {
		t.Errorf("a sixty-column flash cost the bar nothing at 116 columns: %q", long)
	}
	for _, want := range []string{"Esc/Back", "?/Help"} {
		if !strings.Contains(long, want) {
			t.Errorf("a long flash dropped the pinned %s: %q", want, long)
		}
	}
}

// The fit is exact rather than conservative: at every width the bar is as full
// as it can be. One more entry would not fit — checked by rendering the bar one
// column narrower and requiring it to lose something.
func TestTheFitIsExactAtEveryWidth(t *testing.T) {
	for name, k := range viewKeymaps() {
		for _, w := range hintBarWidths {
			if w <= 2 {
				continue // below the clamp there is nothing left to under-fill
			}
			t.Run(fmt.Sprintf("%s/w=%d", name, w), func(t *testing.T) {
				bar := barAt(k, w, "")
				wider := barAt(k, w+1, "")

				if lipgloss.Width(wider) > w-2 && lipgloss.Width(bar) == lipgloss.Width(wider) {
					t.Errorf("the bar at %d columns is the same as at %d, "+
						"but the wider one does not fit here: %q", w, w+1, bar)
				}
			})
		}
	}
}

// ------------------------------------------------------------ the assertions

func assertOneLine(t *testing.T, bar string) {
	t.Helper()
	if n := strings.Count(bar, "\n"); n != 0 {
		t.Errorf("the bar rendered on %d lines, want 1:\n%s", n+1, bar)
	}
}

func assertFitsWithin(t *testing.T, bar string, w int) {
	t.Helper()
	// w-2 is the content width: styles.HintBar carries Padding(0,1).
	if got := lipgloss.Width(bar); got > w-2 {
		t.Errorf("the bar is %d columns wide, and a %d-column terminal leaves %d "+
			"once the bar's padding is taken: %q", got, w, w-2, bar)
	}
}

func assertHelpSurvives(t *testing.T, bar string, w int) {
	t.Helper()
	if w <= 2 {
		// The floor. Two columns of terminal are two columns of padding, so
		// there is no content column for ?/Help to survive into. What is
		// asserted here is that the bar yields the columns rather than
		// overflowing them.
		if bar != "" {
			t.Errorf("the bar rendered %q with no content columns available", bar)
		}
		return
	}
	if !strings.HasPrefix("?/Help", strings.TrimSuffix(bar, "…")) && !strings.Contains(bar, "?/Help") {
		t.Errorf("?/Help did not survive the degradation at width %d: %q\n"+
			"It is the escape hatch — a degradation that removes the way to discover "+
			"what was removed is the wrong degradation.", w, bar)
	}
}

// The marker must count what is actually missing. A marker that drifts from the
// truth is worse than no marker: it is a number the user cannot act on.
func assertMarkerCountIsHonest(t *testing.T, bar string, entries []hintEntry, w int) {
	t.Helper()

	missing := 0
	for _, e := range entries {
		if !strings.Contains(bar, e.text()) {
			missing++
		}
	}

	m := overflowToken.FindStringSubmatch(bar)
	if m == nil {
		if missing == 0 || bar == "" {
			return
		}
		// A missing marker is legal in exactly one place: the floor, where the
		// marker and the last survivor cannot both fit. The marker yields
		// there because a lone `+7` tells the user nothing they can act on
		// while a lone `?/Help` does. Anywhere it would have fitted, its
		// absence is the silent degradation this issue exists to remove — so
		// the excuse is checked rather than assumed.
		token := fmt.Sprintf("+%d", missing)
		if cost := lipgloss.Width(bar) + len("  ") + len(token); cost <= w-2 {
			t.Errorf("%d entries were dropped with no +N marker to say so, "+
				"and %s would have fitted (%d of %d columns): %q",
				missing, token, cost, w-2, bar)
		}
		return
	}

	got, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unparsable overflow marker %q in %q", m[0], bar)
	}
	if got != missing {
		t.Errorf("the bar says +%d but %d entries are actually missing: %q", got, missing, bar)
	}
}

// Pinned entries yield only once every droppable one has. Deriving priority
// from bar order would drop Esc/Back off the target view at 79 columns while
// keeping R/Readme; this is the assertion that would catch that.
func assertPinnedYieldLast(t *testing.T, bar string, entries []hintEntry) {
	t.Helper()
	if bar == "" {
		return
	}

	droppableSurvives := false
	for _, e := range entries {
		if !e.pinned && strings.Contains(bar, e.text()) {
			droppableSurvives = true
		}
	}
	if !droppableSurvives {
		return
	}
	for _, e := range entries {
		if e.pinned && !strings.Contains(bar, e.text()) {
			t.Errorf("the pinned %s was dropped while droppable entries survived: %q",
				e.text(), bar)
		}
	}
}

// Droppable entries yield right to left, so what survives is always a prefix of
// the declared order — the order @UI Patterns prescribes, read left to right.
func assertDroppablesGoRightToLeft(t *testing.T, bar string, entries []hintEntry) {
	t.Helper()
	if bar == "" {
		return
	}

	dropped := false
	for _, e := range entries {
		if e.pinned {
			continue
		}
		if strings.Contains(bar, e.text()) {
			if dropped {
				t.Errorf("%s survived while an entry to its left did not; "+
					"droppable entries yield right to left: %q", e.text(), bar)
			}
			continue
		}
		dropped = true
	}
}
