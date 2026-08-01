#!/usr/bin/env bash
#
# Proves the golden-oracle guards actually fail.
#
# A passing test suite does not demonstrate that a guard works — only that it
# did not fire. Each case below sabotages the oracle in a throwaway copy of the
# repository and asserts the suite goes red. Your working tree is never touched.
#
# Run via: make verify-guards

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PKG="./internal/app/"
GOLDEN="testdata/fixtures.golden"
ORACLE="internal/app/characterization_test.go"

failures=0

# fresh_copy <name> — a pristine copy of the working tree, minus git and build output.
fresh_copy() {
	local dest="$WORK/$1"
	mkdir -p "$dest"
	tar -C "$REPO_ROOT" --exclude=.git --exclude=dist --exclude=mkx -cf - . | tar -C "$dest" -xf -
	echo "$dest"
}

# expect_red <label> <dir> — the suite in dir must fail.
expect_red() {
	local label="$1" dir="$2" out
	if out="$(cd "$dir" && go test -count=1 "$PKG" 2>&1)"; then
		printf 'FAIL  %s\n      suite stayed GREEN — the guard did not trip\n' "$label"
		failures=$((failures + 1))
	else
		printf 'PASS  %s\n      %s\n' "$label" "$(printf '%s' "$out" | grep -E '_test\.go:[0-9]+:' | head -1 | sed 's/^[[:space:]]*//')"
	fi
}

# expect_absent <label> <path> — path must not have been recreated.
expect_absent() {
	local label="$1" path="$2"
	if [ -e "$path" ]; then
		printf 'FAIL  %s\n      the golden was rewritten by a plain test run\n' "$label"
		failures=$((failures + 1))
	else
		printf 'PASS  %s\n      the golden was not recreated\n' "$label"
	fi
}

echo "Sabotaging throwaway copies under $WORK"
echo

# --- 1. Missing golden: must fail, must not rewrite, must fail again ---------
d="$(fresh_copy delete-golden)"
rm "$d/$GOLDEN"
expect_red    "missing golden, run 1" "$d"
expect_absent "missing golden, no rewrite" "$d/$GOLDEN"
expect_red    "missing golden, run 2 (the old silent pass)" "$d"
echo

# --- 2. Empty golden: malformed, must fail ----------------------------------
d="$(fresh_copy empty-golden)"
: > "$d/$GOLDEN"
expect_red "empty golden" "$d"
echo

# --- 3. Oracle renamed ------------------------------------------------------
d="$(fresh_copy rename-oracle)"
sed -i 's/^func TestCharacterization(/func TestCharacterizationRenamed(/' "$d/$ORACLE"
expect_red "oracle renamed" "$d"
echo

# --- 4. Oracle skipped ------------------------------------------------------
d="$(fresh_copy skip-oracle)"
sed -i 's|^func TestCharacterization(t \*testing.T) {|&\n\tt.Skip("temporarily disabled")|' "$d/$ORACLE"
expect_red "oracle skipped" "$d"
echo

# --- 5. Oracle excluded by a build constraint -------------------------------
d="$(fresh_copy buildtag-oracle)"
{ echo "//go:build never"; echo; cat "$d/$ORACLE"; } > "$d/$ORACLE.tmp"
mv "$d/$ORACLE.tmp" "$d/$ORACLE"
expect_red "oracle excluded by //go:build" "$d"
echo

# --- 6. Oracle file deleted outright ----------------------------------------
d="$(fresh_copy delete-oracle)"
rm "$d/$ORACLE"
expect_red "oracle file deleted" "$d"
echo

if [ "$failures" -ne 0 ]; then
	echo "$failures guard(s) failed to trip — the oracle is not protected."
	exit 1
fi

echo "All guards tripped as required."
