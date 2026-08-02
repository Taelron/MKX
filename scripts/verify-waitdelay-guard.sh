#!/usr/bin/env bash
#
# Proves the bounded-read test actually fails without the fix.
#
# TAE-62's deliverable is a read that does NOT hang, and a green suite does not
# demonstrate that — only that nothing hung this time. So this removes
# cmd.WaitDelay from a throwaway copy of the repository and asserts the test
# goes red, by name. Your working tree is never touched.
#
# The stronger half of the check is what must stay GREEN. The test drives two
# shim shapes and only one of them reproduces the defect: `exec sleep` leaves no
# descendant and is bounded by the kill alone, while a bare `sleep` leaves one
# holding the captured stdout pipe. If the sabotage reddened both, the test
# would be failing for some reason other than the mechanism — a broken copy, a
# compile error, an over-tight budget — and would be worthless as evidence. So
# the asymmetry is asserted, not just the failure.
#
# The pristine run comes first for the same reason. This test spawns processes
# and measures wall-clock time; if it cannot pass in this environment before the
# sabotage, a red afterwards says nothing about WaitDelay.
#
# Run via: make verify-waitdelay-guard

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PKG="./internal/adapter/gitx/"
TARGET="internal/adapter/gitx/gitx.go"
TEST="TestRunBoundsAKilledProcessWhoseDescendantHoldsStdout"

# The sentinel comment above the assignment in run(). The sabotage deletes from
# it through the cmd.WaitDelay line it introduces.
SENTINEL="// waitdelay-guard:"

failures=0

# fresh_copy <name> — a pristine copy of the working tree, minus git and build output.
fresh_copy() {
	local dest="$WORK/$1"
	mkdir -p "$dest"
	# ./mkx, not mkx: an unanchored pattern matches every path component of
	# that name, which would drop cmd/mkx/ along with the built binary. The
	# gitx package would still compile without it, so the copy would look fine
	# and the guard would go on reporting a verdict about a truncated tree.
	tar -C "$REPO_ROOT" --exclude=.git --exclude=dist --exclude=./mkx -cf - . | tar -C "$dest" -xf -
	echo "$dest"
}

# strip_waitdelay <dir> — delete the sentinel comment block and the assignment
# that follows it.
#
# The copy must actually change, and afterwards it must contain no cmd.WaitDelay
# at all. A sentinel that no longer matches — because the comment was reworded,
# or run() was restructured — would otherwise leave the bound in place, the test
# would pass, and this script would report success while checking nothing. That
# is the exact false confidence it exists to rule out, so a no-op sabotage is a
# failure here, not a silent skip.
strip_waitdelay() {
	local dir="$1"
	local file="$dir/$TARGET"

	awk -v s="$SENTINEL" '
		index($0, s) { skip = 1; next }
		skip && /cmd\.WaitDelay/ { skip = 0; next }
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

	if grep -q 'cmd\.WaitDelay' "$file"; then
		printf 'FAIL  %s still sets cmd.WaitDelay after the sabotage — the bound was not removed\n' "$TARGET"
		failures=$((failures + 1))
		return 1
	fi
}

echo "Sabotaging a throwaway copy under $WORK"
echo

# --- 1. Pristine: the reproduction has to work here at all ------------------
d="$(fresh_copy pristine)"
if out="$(cd "$d" && go test -count=1 -v -run "$TEST" "$PKG" 2>&1)"; then
	printf 'PASS  pristine copy — the bounded-read test is green before any sabotage\n'
	printf '%s\n' "$out" | grep -E '^\s+run_bounded_test\.go' | sed 's/^[[:space:]]*/      /'
else
	printf 'FAIL  pristine copy — the bounded-read test does not pass before the sabotage.\n'
	printf '      A red run after the sabotage would prove nothing about WaitDelay.\n'
	printf '%s\n' "$out" | sed 's/^/        /'
	failures=$((failures + 1))
fi
echo

# --- 2. WaitDelay removed: the descendant case must go red, alone -----------
d="$(fresh_copy strip-waitdelay)"
if strip_waitdelay "$d"; then
	out="$(cd "$d" && go test -count=1 -v -run "$TEST" "$PKG" 2>&1)"
	rc=$?

	if [ "$rc" -eq 0 ]; then
		printf 'FAIL  WaitDelay removed — the suite stayed GREEN.\n'
		printf '      The bounded-read test does not detect an unbounded read; it is not evidence.\n'
		failures=$((failures + 1))
	else
		red_ok=1
		green_ok=1
		grep -qF -- "--- FAIL: $TEST/bare-sleep" <<<"$out" || red_ok=0
		grep -qF -- "--- PASS: $TEST/exec-sleep" <<<"$out" || green_ok=0

		if [ "$red_ok" -eq 1 ] && [ "$green_ok" -eq 1 ]; then
			printf 'PASS  WaitDelay removed — the test discriminates\n'
			printf '      red:   %s/bare-sleep   (a descendant holds the pipe)\n' "$TEST"
			printf '      green: %s/exec-sleep   (nothing survives the kill)\n' "$TEST"
			# The t.Fatalf line is printed BEFORE the --- FAIL: marker, not
			# after it, so this looks backwards from the marker.
			grep -B6 -F -- "--- FAIL: $TEST/bare-sleep" <<<"$out" |
				grep -E '^\s+run_bounded_test\.go' | sed 's/^[[:space:]]*/         /'
		else
			[ "$red_ok" -eq 0 ] && printf 'FAIL  %s/bare-sleep did not go red; the suite failed for some other reason\n' "$TEST"
			[ "$green_ok" -eq 0 ] && printf 'FAIL  %s/exec-sleep did not stay green; the test is not isolating the mechanism\n' "$TEST"
			printf '%s\n' "$out" | sed 's/^/        /'
			failures=$((failures + 1))
		fi
	fi
fi
echo

if [ "$failures" -ne 0 ]; then
	echo "$failures check(s) failed — the read bound is not proven."
	exit 1
fi

echo "The bounded-read test goes red without WaitDelay, and only for the case that needs it."
