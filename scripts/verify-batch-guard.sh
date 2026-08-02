#!/usr/bin/env bash
#
# Proves the batched-input guards actually fail.
#
# TAE-61's whole deliverable is behaviour that must not happen: no batch fires a
# checkout, a pull or a target run. A green suite does not demonstrate that —
# only that nothing tripped. So each case below removes one of handleKey's two
# batch guards in a throwaway copy of the repository and asserts the batch tests
# go red, by name. Your working tree is never touched.
#
# The two guards are sabotaged independently, and that is the point. A single
# sabotage point would leave one of them unproven: tier 3 keeps a batch away
# from the view's keymap, tier 1 keeps it away from an open modal's, and only
# tier 1 stands between a replayed `enter` and a git checkout. Removing them
# together would let either one carry the other's evidence.
#
# Run via: make verify-batch-guard

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

PKG="./internal/ui/tui/"
MODEL="internal/ui/tui/model.go"

# The sentinel comments in handleKey. Each opens the guard it names; the
# sabotage below deletes from the sentinel through the guard's closing brace.
TIER1_SENTINEL="// batch-guard: tier 1."
TIER3_SENTINEL="// batch-guard: tier 3."

failures=0

# fresh_copy <name> — a pristine copy of the working tree, minus git and build output.
fresh_copy() {
	local dest="$WORK/$1"
	mkdir -p "$dest"
	tar -C "$REPO_ROOT" --exclude=.git --exclude=dist --exclude=mkx -cf - . | tar -C "$dest" -xf -
	echo "$dest"
}

# strip_guard <label> <dir> <sentinel> — delete the sentinel comment block and
# the guard that follows it, up to and including its closing brace.
#
# The copy must actually change. A sentinel that no longer matches — because the
# comment was reworded, or handleKey was restructured — would otherwise leave the
# guard in place, the tests would pass, and this script would report success
# while checking nothing. That is the exact false confidence it exists to rule
# out, so a no-op sabotage is a failure here, not a silent skip.
strip_guard() {
	local label="$1" dir="$2" sentinel="$3"
	local file="$dir/$MODEL"

	awk -v s="$sentinel" '
		index($0, s) { skip = 1; next }
		skip && /^[[:space:]]*}[[:space:]]*$/ { skip = 0; next }
		skip { next }
		{ print }
	' "$file" > "$file.tmp" || { printf 'FAIL  %s\n      the sabotage step itself errored\n' "$label"; failures=$((failures + 1)); return 1; }

	if cmp -s "$file" "$file.tmp"; then
		printf 'FAIL  %s\n      the sentinel %s matched nothing in %s — this script has stopped checking\n' \
			"$label" "$sentinel" "$MODEL"
		failures=$((failures + 1))
		rm -f "$file.tmp"
		return 1
	fi

	mv "$file.tmp" "$file"
}

# expect_red <label> <dir> <test-name>... — the suite in dir must fail, and each
# named test must be among the failures.
#
# A nonzero exit is not enough. A copy broken by the sabotage, a module-cache
# hiccup or a compile error also exits nonzero, and accepting that would let this
# script report success while no guard ever fired. So each case names the tests
# whose failure is the evidence, and only those count as a pass.
expect_red() {
	local label="$1" dir="$2"
	shift 2
	local out missing=0

	if out="$(cd "$dir" && go test -count=1 "$PKG" 2>&1)"; then
		printf 'FAIL  %s\n      suite stayed GREEN — the guard did not trip\n' "$label"
		failures=$((failures + 1))
		return
	fi

	for name in "$@"; do
		if ! printf '%s' "$out" | grep -qF -- "--- FAIL: $name"; then
			printf 'FAIL  %s\n      %s did not go red; the suite failed for some other reason\n' "$label" "$name"
			missing=1
		fi
	done

	if [ "$missing" -ne 0 ]; then
		printf '%s\n' "$out" | sed 's/^/        /'
		failures=$((failures + 1))
		return
	fi

	printf 'PASS  %s\n' "$label"
	for name in "$@"; do
		printf '      red: %s\n' "$name"
		printf '%s' "$out" | grep -A2 -F -- "--- FAIL: $name" | grep -E '^\s+batch_test\.go' | head -2 | sed 's/^[[:space:]]*/           /'
	done
}

echo "Sabotaging throwaway copies under $WORK"
echo

# --- 1. Tier 3 removed: a batch reaches the view's keymap -------------------
#
# The defect this issue was filed for, plus the view-level half of the collision
# found during planning: a replayed `enter` runs the selected Make target.
d="$(fresh_copy strip-tier-3)"
if strip_guard "tier 3 sabotage" "$d" "$TIER3_SENTINEL"; then
	expect_red "tier 3 removed — a batch reaches the view keymap" "$d" \
		TestPastedFilterStringOpensFilterModeAndTypesTheRest \
		TestReplayedFilterStringDoesTheSame \
		TestBatchSpellingEnterDoesNotRunATarget \
		TestBatchSpellingCtrlCDoesNotQuit \
		TestBatchBeginningWithBDoesNotOpenTheBranchPicker \
		TestBatchBeginningWithGDoesNotPull \
		TestBatchBeginningWithCapitalRDoesNotOpenTheReadme \
		TestBatchInTheProjectViewIsIgnored \
		TestASinglePastedRuneDoesNotFireItsBinding
fi
echo

# --- 2. Tier 1 removed: a batch reaches an open modal's keymap --------------
#
# The mutation only tier 1 stands between: with the branch picker open, a raw
# replay spelling `enter` reaches confirmBranch and hands the terminal to
# `git checkout`. Nothing at tier 3 can see this, which is why it is sabotaged
# on its own.
d="$(fresh_copy strip-tier-1)"
if strip_guard "tier 1 sabotage" "$d" "$TIER1_SENTINEL"; then
	expect_red "tier 1 removed — a batch reaches the modal keymap" "$d" \
		TestBatchSpellingEnterInTheBranchPickerDoesNotCheckOut
fi
echo

if [ "$failures" -ne 0 ]; then
	echo "$failures guard(s) failed to trip — batched input is not guarded."
	exit 1
fi

echo "Both batch guards tripped as required."
