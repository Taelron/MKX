#!/usr/bin/env bash
#
# Builds a throwaway workspace whose `git` never answers, so the read bound can
# be watched degrading on screen.
#
# This is the half of TAE-62 the unit test cannot show. The test proves the
# mechanism — run() returns within deadline + WaitDelay — but ADR-M003's promise
# is about what the user gets: a header that degrades to unknown, bounded, with
# the TUI interactive the whole time. That is three claims about a screen, and
# they are verified by looking at one.
#
# The shim is the shape TAE-62 was isolated to. A bare `sleep` rather than
# `exec sleep` is the entire point: the shell is killed at the deadline and the
# backgrounded sleep is not, so it goes on holding the stdout pipe MkX is
# capturing into. With `exec` there is no descendant and the deadline alone was
# always enough — that case never reproduced the defect.
#
# Nothing here touches your working tree, and no real git repository is
# involved: the shim intercepts every git invocation, so there is nothing for a
# repository to tell MkX. The workspace is left behind deliberately — the point
# is to run mkx in it — and is yours to delete.
#
# Run via: make demo-hung-git

set -euo pipefail

WORK="$(mktemp -d -t mkx-hung-git-XXXXXX)"
MKX="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/mkx"

mkdir -p "$WORK/bin"

# The shim, named `git` and first on PATH. It answers nothing, ever: whatever
# MkX asks, it backgrounds a sleep that inherits stdout and then waits to be
# killed. `wait` rather than a second sleep so the shell has no work of its own
# to be interrupted in.
#
# 300 seconds is long enough that the descendant outlives any plausible session
# with this workspace. Killing it early is what the cleanup line at the bottom
# of the checklist is for.
cat >"$WORK/bin/git" <<'EOF'
#!/bin/sh
sleep 300 &
wait
EOF
chmod +x "$WORK/bin/git"

# Three projects, so there is somewhere to move the cursor to and the read can
# be watched happening more than once. Reads are lazy — issued on Enter into a
# project, per tui/git.go — so each Enter is a fresh observation.
#
# printf rather than a heredoc, and not by preference: `<<-` strips leading
# TABS, which is exactly the character make requires at the start of a recipe
# line. A tab-indented heredoc here produces "missing separator" on every
# target — silently, because nothing runs these until a human does.
for name in alpha beta gamma; do
	mkdir -p "$WORK/$name"
	{
		printf '.PHONY: hello build test lint deploy\n\n'
		printf 'hello: ## Print a greeting\n\t@echo "hello from %s"\n\n' "$name"
		printf 'build: ## Pretend to build\n\t@echo "built %s"\n\n' "$name"
		printf 'test: ## Pretend to test\n\t@echo "tested %s"\n\n' "$name"
		printf 'lint: ## Pretend to lint\n\t@echo "linted %s"\n\n' "$name"
		printf 'deploy: ## Pretend to deploy\n\t@echo "deployed %s"\n' "$name"
	} >"$WORK/$name/Makefile"
done

cat <<EOF

Workspace: $WORK
A shim named 'git' is first on PATH there. It never answers.

Run it with:

  cd $WORK && PATH="$WORK/bin:\$PATH" $MKX

What to look for
----------------

  1. Press Enter on 'alpha'. The header segment reads  …  while the read is
     in flight.

     It MUST change to  git unknown  about two and a half seconds later:
     the 2s read deadline, then up to 500ms for cmd.Wait to give up on the
     pipe the descendant is holding. Count it — "eventually" is the defect,
     not the fix.

     If it sits on  …  and never resolves, TAE-62 is back. That is the whole
     failure mode: the deadline fires, git is killed, and the read goes on
     waiting on a pipe a killed process cannot close.

  2. While it still reads  …  , press / and type. Filtering must respond
     immediately, and Esc must clear it. The read is on a tea.Cmd, so a
     read in flight has never blocked the event loop — this confirms that
     the bound did not change it.

  3. Esc back to the project list, then Enter on 'beta'. Same sequence
     again, and the same ~2.5s. The bound is per read, not per session.

  4. Esc, then Enter on 'alpha' again. The header reads  git unknown
     immediately: the failed read is cached for the generation, so there is
     no second wait.

Delete it with:

  rm -rf $WORK

The sleeps the shim leaves behind exit on their own within five minutes, or:

  pkill -f 'sleep 300'

EOF
