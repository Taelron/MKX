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
#
# -b main for the same reason: the checklist below names `main` in what the
# header must read, and the conflict fixture checks out `main` by name. Without
# it the branch is whatever init.defaultBranch happens to be on this machine,
# and the checklist quietly stops matching the screen.
git_init() {
	git init -q -b main "$1"
	git -C "$1" config user.email "demo@example.invalid"
	git -C "$1" config user.name "mkx demo"
	git -C "$1" config commit.gpgsign false
}

# The targets exist to make invalidation OBSERVABLE. A target that merely takes
# a while proves nothing: the header re-reads to the same value, and the only
# visible difference is a sub-millisecond "…" no human can catch. These three
# change git state, so the header must come back showing something different —
# which is a check that can actually fail.
# printf rather than a heredoc, and not by preference: `<<-` strips leading
# TABS, which is exactly the character make requires at the start of a recipe
# line. A tab-indented heredoc here produces "missing separator" on every
# target — silently, because nothing runs these until a human does.
makefile() {
	{
		printf '.PHONY: hello dirty-it clean-it switch-branch\n\n'
		printf 'hello: ## Print a greeting\n\t@echo "hello from $$(basename $$(pwd))"\n\n'
		printf 'dirty-it: ## Leave an untracked file - header must flip to dirty\n\t@touch scratch.txt && echo "created scratch.txt"\n\n'
		printf 'clean-it: ## Remove it again - header must flip back to clean\n\t@rm -f scratch.txt && echo "removed scratch.txt"\n\n'
		printf 'switch-branch: ## Move to another branch - header must show the new name\n\t@git checkout -q -B demo-branch && echo "now on demo-branch"\n'
	} >"$1/Makefile"
}

# A README in every project, so `R` is a real tea.Exec handover. Without one,
# ReadmePath returns "" and viewReadme short-circuits to a not-found message —
# no handover at all, so pressing R would verify nothing about invalidation.
# The first non-heading line also becomes the project's Description in the list.
readme() {
	cat >"$1/README.md" <<-EOF
		# $(basename "$1")

		Demo project for the mkx git-state verification pass.
	EOF
}

# clean — a repository with a commit and nothing outstanding.
mkdir -p "$WORK/clean"
makefile "$WORK/clean"
readme "$WORK/clean"
git_init "$WORK/clean"
git -C "$WORK/clean" add -A
git -C "$WORK/clean" commit -qm "initial"
# Branches to switch between, and enough of them to overflow the picker's
# window: it shows seven rows, so nine branches means two are hidden and the
# "… 2 more" line has to appear. A repository with three branches would leave
# the windowing entirely unexercised by eye.
for branch in feat/api release/2.1 wip/spike hotfix/urgent docs/readme \
	chore/deps experiment/one experiment/two; do
	git -C "$WORK/clean" branch "$branch"
done

# dirty — the same, plus an untracked file. Untracked rather than modified on
# purpose: `git status --porcelain` reports it, and a reader that only looked at
# tracked changes would call this clean.
mkdir -p "$WORK/dirty"
makefile "$WORK/dirty"
readme "$WORK/dirty"
git_init "$WORK/dirty"
git -C "$WORK/dirty" add -A
git -C "$WORK/dirty" commit -qm "initial"
echo "scratch" >"$WORK/dirty/untracked.txt"

# conflict — the fixture for the step that matters: a checkout git will REFUSE.
#
# The `dirty` project above will not do. It is dirtied with an *untracked* file,
# and git switches branches perfectly happily with untracked files present — a
# refusal step built on it is a step that cannot fail, which is worse than no
# step at all.
#
# To make git actually refuse, the working tree needs an uncommitted
# modification to a file whose content DIFFERS between the two branches. Then
# switching would have to overwrite the edit, and git stops with "Your local
# changes to the following files would be overwritten by checkout".
mkdir -p "$WORK/conflict"
makefile "$WORK/conflict"
readme "$WORK/conflict"
git_init "$WORK/conflict"
printf 'the main version\n' >"$WORK/conflict/shared.txt"
git -C "$WORK/conflict" add -A
git -C "$WORK/conflict" commit -qm "initial"
git -C "$WORK/conflict" checkout -q -b other
printf 'the other version\n' >"$WORK/conflict/shared.txt"
git -C "$WORK/conflict" commit -qam "diverge shared.txt"
git -C "$WORK/conflict" checkout -q main
printf 'an uncommitted local edit\n' >"$WORK/conflict/shared.txt"

# detached — HEAD on a commit rather than a branch. `git rev-parse
# --abbrev-ref HEAD` prints the literal "HEAD" here, exit 0.
mkdir -p "$WORK/detached"
makefile "$WORK/detached"
readme "$WORK/detached"
git_init "$WORK/detached"
git -C "$WORK/detached" add -A
git -C "$WORK/detached" commit -qm "initial"
git -C "$WORK/detached" checkout -q --detach

# unborn — initialised, never committed. Prints the same literal "HEAD" as
# detached above, and differs only in the exit code (128). This is the pair the
# classifier has to tell apart.
mkdir -p "$WORK/unborn"
makefile "$WORK/unborn"
readme "$WORK/unborn"
git_init "$WORK/unborn"

# norepo — a Makefile and no repository. An absence, not an error.
mkdir -p "$WORK/norepo"
makefile "$WORK/norepo"
readme "$WORK/norepo"

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
readme "$WORK/broken"
printf 'this is not a gitfile\n' >"$WORK/broken/.git"

cat <<EOF

Demo workspace: $WORK

Enter each project (↑↓ then Enter) and check the header segment:

  clean       main ✓ clean        (green)
  conflict    main ● dirty        (yellow)
  detached    detached ✓ clean    (yellow)
  dirty       main ● dirty        (yellow)
  norepo      no repo             (dimmed)
  unborn      no commits          (dimmed)
  broken      git unknown         (red)

Then confirm, in that order:

  1. The project list shows NO git column, no git state of any kind, and
     no pause on startup.
  2. "no repo", "git unknown" and "main ✓ clean" are three visibly
     different things — not three greys.

  3. INVALIDATION, the one that can actually fail. Enter 'clean':

       header reads          main ✓ clean
       run 'dirty-it'   →    main ● dirty        (state changed under it)
       run 'clean-it'   →    main ✓ clean        (and back)
       run 'switch-branch' → demo-branch ✓ clean (branch re-read too)

     If any of those still shows the previous value, RepoState was not
     invalidated on return from the handover. This is the shared handover()
     path, so it covers 'g' (git pull) as well as running a target.

  4. README handover. In 'clean', press R, then Esc back.

     Its invalidation cannot be made visually distinct — viewing a README
     cannot change git state, so the re-read returns the same value by
     construction. That the invalidation happens at all is covered by
     TestHandoverWithNoInnerMessageStillInvalidates. What you are checking
     here is the return path: the README renders, and you come back to a
     correct, NON-BLANK header, not a stale or empty one. readme.go
     returned a bare nil before this change and now returns a wrapper, so
     the return path is what changed.

  5. Caching. Enter 'clean', Esc, enter it again — the header shows its
     value immediately with no "…" flicker, because the entry is cached.
     After step 3's re-read, the brief "…" is visible again.

BRANCH SWITCHING (b). Five steps; the second is the one that matters.

  1. Enter 'clean' and press b. The picker lists the local branches, 'main'
     carries (current), and the cursor starts on it. Nine branches, seven
     rows, so the last line reads "… 2 more" — the box does not grow and
     the hint bar and bottom border are still there. Select another branch
     and press Enter:

       git's own output appears in the terminal
       Enter                →  back to mkx
       header reads            the branch you selected

  2. THE ONE THAT MATTERS. Enter 'conflict' — header reads main ● dirty.
     Press b, select 'other', press Enter.

       git REFUSES, in its own words:
         "Your local changes to the following files would be overwritten
          by checkout: shared.txt"

       Enter                →  back to mkx
       header MUST still read  main ● dirty

     If the header reads 'other', the TUI has claimed a branch that was
     never checked out. That is the worst outcome this feature can produce
     and the branch is not shippable — stop and report it rather than
     working around it. Nothing on screen may name the branch you selected:
     not the header, not a flash message.

  3. Enter 'detached' and press b. The picker opens normally and NO row is
     marked (current) — a detached HEAD is on no branch. Select one and it
     is a legitimate escape: the header comes back naming that branch.

  4. Enter 'norepo' and press b. A notice modal, reading "not in a git
     repository". Esc closes it. The key is never dead.

  5. Enter 'unborn' and press b — "no commits yet". Enter 'broken' and
     press b — "could not be read". Together with step 4 that is three
     visibly different sentences for three different reasons.

Run it with:

  cd $WORK && $(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/mkx

Delete it with:

  rm -rf $WORK

EOF
