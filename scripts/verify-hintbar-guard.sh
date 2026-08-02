#!/usr/bin/env bash
#
# Proves the hint bar width table actually fails without the fix.
#
# TAE-60's deliverable is a bar that does NOT wrap, and a green suite does not
# demonstrate that — only that nothing wrapped this time. So this reverts
# renderHintBar in a throwaway copy of the repository to what it was before the
# fix, and asserts the width table goes red, by subtest name. Your working tree
# is never touched.
#
# The stronger half of the check is what must stay GREEN. The naive render is
# not wrong at every width: at 80 columns the target bar is exactly 78 of 78
# content columns, so it fits there even unfitted, and w=80 must keep passing.
# If the sabotage reddened every width, the table would be failing for some
# reason other than the mechanism — a broken copy, a compile error, a bad
# sentinel — and would be worthless as evidence. So the asymmetry is asserted,
# not just the failure.
#
# That asymmetry is also the shape of the bug itself. It was invisible at the
# supported minimum and appeared one column below it, which is why it survived
# to be filed as a latent issue rather than a live one.
#
# The pristine run comes first for the same reason: if the table cannot pass in
# this environment before the sabotage, a red afterwards says nothing.
#
# Run via: make verify-hintbar-guard

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PKG="./internal/ui/tui/"
TARGET="internal/ui/tui/model.go"
TEST="TestHintBarDegradesAcrossTheWidthTable"

# The sentinel comment inside renderHintBar. The sabotage replaces everything
# from it through the return that follows.
SENTINEL="// hintbar-guard:"

# The subtests that must go red, and the one that must not. Both views' bars fit
# at 80 unfitted; only the target view is narrow enough to wrap below it, and it
# is the view the issue was measured on.
RED_SUBTESTS=("targets/w=79" "targets/w=60" "targets/w=40")
GREEN_SUBTESTS=("targets/w=80" "projects/w=80")

failures=0

# fresh_copy <name> — a pristine copy of the working tree, minus git and build output.
fresh_copy() {
	local dest="$WORK/$1"
	mkdir -p "$dest"
	# ./mkx, not mkx: an unanchored pattern matches every path component of
	# that name, which would drop cmd/mkx/ along with the built binary.
	tar -C "$REPO_ROOT" --exclude=.git --exclude=dist --exclude=./mkx -cf - . | tar -C "$dest" -xf -
	echo "$dest"
}

# unfit_the_bar <dir> — put renderHintBar back to the pre-TAE-60 body: the whole
# bar assembled at its natural width, the flash appended untruncated, and the
# result handed to lipgloss at the terminal's width to word-wrap.
#
# The copy must actually change. A sentinel that no longer matches — because the
# comment was reworded, or renderHintBar was restructured — would otherwise
# leave the fix in place, the table would pass, and this script would report
# success while checking nothing. That is the exact false confidence it exists
# to rule out, so a no-op sabotage is a failure here, not a silent skip.
unfit_the_bar() {
	local dir="$1"
	local file="$dir/$TARGET"

	cat >"$WORK/naive.go.frag" <<'FRAG'
	bar := k.hintBar()
	if m.flash != "" {
		bar += "  " + styles.Flash.Render(m.flash)
	}
	return styles.HintBar.Width(w).Render(bar)
FRAG

	awk -v s="$SENTINEL" -v frag="$WORK/naive.go.frag" '
		index($0, s) {
			skip = 1
			while ((getline line < frag) > 0) print line
			close(frag)
			next
		}
		skip && /return styles\.HintBar/ { skip = 0; next }
		skip { next }
		{ print }
	' "$file" >"$file.tmp" || {
		printf 'FAIL  the sabotage step itself errored\n'
		failures=$((failures + 1))
		return 1
	}

	if cmp -s "$file" "$file.tmp"; then
		printf 'FAIL  the sentinel %s matched nothing in %s — this script has stopped checking\n' \
			"$SENTINEL" "$TARGET"
		failures=$((failures + 1))
		rm -f "$file.tmp"
		return 1
	fi

	mv "$file.tmp" "$file"

	# The call, not the name: renderHintBar's doc comment names fitHintBar to
	# point at where the claim order lives, and that comment sits above the
	# sentinel and survives the sabotage.
	if grep -qF 'fitHintBar(' "$file"; then
		printf 'FAIL  %s still calls fitHintBar after the sabotage — the fit was not removed\n' "$TARGET"
		failures=$((failures + 1))
		return 1
	fi
}

echo "Sabotaging a throwaway copy under $WORK"
echo

# --- 1. Pristine: the table has to pass here at all -------------------------
d="$(fresh_copy pristine)"
if out="$(cd "$d" && go test -count=1 -v -run "$TEST" "$PKG" 2>&1)"; then
	printf 'PASS  pristine copy — the width table is green before any sabotage\n'
	printf '%s\n' "$out" | grep -E 'hintbar_test\.go:[0-9]+: w=' | sed 's/^[[:space:]]*/      /'
else
	printf 'FAIL  pristine copy — the width table does not pass before the sabotage.\n'
	printf '      A red run afterwards would prove nothing about the fit.\n'
	printf '%s\n' "$out" | sed 's/^/        /'
	failures=$((failures + 1))
fi
echo

# --- 2. The fit removed: the narrow widths must go red, and only those -------
d="$(fresh_copy unfit)"
if unfit_the_bar "$d"; then
	out="$(cd "$d" && go test -count=1 -v -run "$TEST" "$PKG" 2>&1)"
	rc=$?

	if [ "$rc" -eq 0 ]; then
		printf 'FAIL  the fit removed — the suite stayed GREEN.\n'
		printf '      The width table does not detect a wrapping hint bar; it is not evidence.\n'
		failures=$((failures + 1))
	else
		bad=0
		for sub in "${RED_SUBTESTS[@]}"; do
			if grep -qF -- "--- FAIL: $TEST/$sub" <<<"$out"; then
				printf '      red:   %s\n' "$sub"
			else
				printf 'FAIL  %s did not go red; the table does not catch a wrapped bar there\n' "$sub"
				bad=1
			fi
		done
		for sub in "${GREEN_SUBTESTS[@]}"; do
			if grep -qF -- "--- PASS: $TEST/$sub" <<<"$out"; then
				printf '      green: %s\n' "$sub"
			else
				printf 'FAIL  %s did not stay green; the sabotage reddened a width that fits\n' "$sub"
				printf '      unfitted, so the table is not isolating the mechanism.\n'
				bad=1
			fi
		done

		if [ "$bad" -eq 0 ]; then
			printf 'PASS  the fit removed — the table discriminates\n'
			# What w=79 actually said. `go test` groups every --- FAIL: marker
			# after all the log output, so this reads the subtest's own === RUN
			# block rather than the lines around its marker.
			printf '%s\n' "$out" |
				awk -v r="=== RUN   $TEST/targets/w=79" '
					$0 == r { on = 1; next }
					on && /^=== / { exit }
					on && /hintbar_test\.go:[0-9]+:/ { print "         " $0 }
				' | sed 's/         [[:space:]]*/         /'
		else
			printf '%s\n' "$out" | grep -E '^\s+--- (PASS|FAIL): ' | sed 's/^/        /'
			failures=$((failures + 1))
		fi
	fi
fi
echo

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed — the hint bar fit is not proven."
	exit 1
fi

echo "The width table goes red without the fit, and stays green where the bar fits without it."
