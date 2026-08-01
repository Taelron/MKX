#!/usr/bin/env bash
#
# Builds a throwaway workspace exercising every git state MkX can display, so
# the visual half of TAE-58's verification can be run by eye in one place.
#
# It is built under a temp directory rather than inside this repository for one
# specific reason: the "no repo" state is unreachable from anywhere inside a
# git repository, because git's upward .git discovery finds this one. A project
# has to sit outside every repository for MkX to report it has none.
#
# Nothing here touches your working tree. The workspace is left behind
# deliberately — the point is to run mkx in it — and is yours to delete.
#
# Run via: make demo-git

set -euo pipefail

WORK="$(mktemp -d -t mkx-git-demo-XXXXXX)"

# Local to this script, so the demo repositories do not depend on the machine's
# git identity and do not inherit a global hooks or template directory.
git_init() {
	git init -q "$1"
	git -C "$1" config user.email "demo@example.invalid"
	git -C "$1" config user.name "mkx demo"
	git -C "$1" config commit.gpgsign false
}

makefile() {
	cat >"$1/Makefile" <<-'EOF'
		.PHONY: hello slow

		hello: ## Print a greeting
			@echo "hello from $$(basename $$(pwd))"

		slow: ## Sleep briefly, to watch the header re-read on return
			@sleep 2 && echo done
	EOF
}

# clean — a repository with a commit and nothing outstanding.
mkdir -p "$WORK/clean"
makefile "$WORK/clean"
git_init "$WORK/clean"
git -C "$WORK/clean" add -A
git -C "$WORK/clean" commit -qm "initial"

# dirty — the same, plus an untracked file. Untracked rather than modified on
# purpose: `git status --porcelain` reports it, and a reader that only looked at
# tracked changes would call this clean.
mkdir -p "$WORK/dirty"
makefile "$WORK/dirty"
git_init "$WORK/dirty"
git -C "$WORK/dirty" add -A
git -C "$WORK/dirty" commit -qm "initial"
echo "scratch" >"$WORK/dirty/untracked.txt"

# detached — HEAD on a commit rather than a branch. `git rev-parse
# --abbrev-ref HEAD` prints the literal "HEAD" here, exit 0.
mkdir -p "$WORK/detached"
makefile "$WORK/detached"
git_init "$WORK/detached"
git -C "$WORK/detached" add -A
git -C "$WORK/detached" commit -qm "initial"
git -C "$WORK/detached" checkout -q --detach

# unborn — initialised, never committed. Prints the same literal "HEAD" as
# detached above, and differs only in the exit code (128). This is the pair the
# classifier has to tell apart.
mkdir -p "$WORK/unborn"
makefile "$WORK/unborn"
git_init "$WORK/unborn"

# norepo — a Makefile and no repository. An absence, not an error.
mkdir -p "$WORK/norepo"
makefile "$WORK/norepo"

# broken — .git is a file holding text that is not a gitfile. git reports
# "fatal: invalid gitfile format", which matches neither the not-a-repository
# rule nor the unborn rule, so the read degrades to unknown. This is how a
# failed read is forced with no test hooks in production code.
#
# The content matters: a .git file starting with "gitdir: " and a bad path
# yields "fatal: not a git repository" instead, which classifies as ABSENT.
# Verified against git 2.47.3.
mkdir -p "$WORK/broken"
makefile "$WORK/broken"
printf 'this is not a gitfile\n' >"$WORK/broken/.git"

cat <<EOF

Demo workspace: $WORK

Enter each project (↑↓ then Enter) and check the header segment:

  clean       main ✓ clean        (green)
  detached    detached ✓ clean    (yellow)
  dirty       main ● dirty        (yellow)
  norepo      no repo             (dimmed)
  unborn      no commits          (dimmed)
  broken      git unknown         (red)

Then confirm, in that order:

  1. The project list shows NO git column and no pause on startup.
  2. "no repo", "git unknown" and "main ✓ clean" are three visibly
     different things — not three greys.
  3. In 'dirty', run the 'slow' target and confirm the header re-reads on
     return rather than showing the state it had before.
  4. In 'dirty', press R for the README and confirm the header re-reads too.
  5. In 'dirty', press g (git pull; it will fail, there is no remote) and
     confirm the header re-reads on return.

Run it with:

  cd $WORK && $(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/mkx

Delete it with:

  rm -rf $WORK

EOF
