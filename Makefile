BINARY   := mkx
FIXTURES := testdata/fixtures

# Install location. Both are overridable, so no machine's own layout is baked
# into the repo: `make install PREFIX=/usr/local` or `make install BINDIR=/tmp/x`.
# ?= rather than := so BINDIR keeps tracking PREFIX when only PREFIX is given.
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build install test verify verify-parser verify-gitx verify-tui verify-hintbar-widths verify-guards verify-batch-guard verify-waitdelay-guard verify-hintbar-guard rebaseline-golden tidy-check run demo-git demo-hung-git

build: ## Compile the mkx binary
	go build -o $(BINARY) ./cmd/mkx

install: build ## Install the binary to $(BINDIR) — override PREFIX or BINDIR to put it elsewhere
	# Depends on build rather than copying whatever ./mkx happens to be. An
	# install target that ships a stale binary is worse than none: the copy on
	# PATH is the one that gets run, and nothing about it says how old it is.
	install -d $(BINDIR)
	install -m 0755 $(BINARY) $(BINDIR)/$(BINARY)
	@echo "installed $(BINDIR)/$(BINARY)"
	@case ":$$PATH:" in \
		*":$(BINDIR):"*) ;; \
		*) echo "warning: $(BINDIR) is not on your PATH, so \`$(BINARY)\` will not resolve to it" ;; \
	esac
	@command -v $(BINARY) >/dev/null 2>&1 && \
		[ "$$(command -v $(BINARY))" != "$(BINDIR)/$(BINARY)" ] && \
		echo "warning: \`$(BINARY)\` resolves to $$(command -v $(BINARY)), not the copy just installed" || true

test: ## Run the full test suite
	go test ./...

verify: ## Check discovery behaviour against the committed golden
	# -count=1 is load-bearing: without it a cached result replays an earlier
	# pass, and the oracle reports green without having compared anything.
	go test -v -count=1 -run TestCharacterization ./internal/app/

verify-parser: ## Run the pure parser table tests
	go test -v -count=1 ./internal/adapter/makex/

verify-gitx: ## Run the git classifier tests and the bounded-read test
	# The classifier and parser tests spawn nothing: they drive the classifiers
	# from captured strings, so they pass identically on a box with no git. The
	# bounded-read test (unix only) is the one exception, and it does not
	# invoke the real binary either — it puts a shell shim named `git` on PATH.
	go test -v -count=1 ./internal/adapter/gitx/

verify-tui: ## Run the TUI keymap, modal and overlay tests by name
	go test -v -count=1 ./internal/ui/tui/

verify-hintbar-widths: ## Print the hint bar at every width in the table and assert it degrades rather than wraps
	# -v is load-bearing here, not decoration: the subtests log the bar they
	# rendered, and `go test` discards that output on a pass. The point of this
	# target is to be READ — the degradation has to be legible, which is not
	# something a green tick can show. Add new hintbar_test.go tests to this
	# list, or they run only under verify-tui.
	go test -v -count=1 -run '^(TestHintBarDegradesAcrossTheWidthTable|TestTargetBarMatchesTheComputedTable|TestFullViewHeightIsWidthInvariant|TestALongFlashDoesNotWrapTheBar|TestAFlashClaimsWidthAheadOfTheDroppableHints|TestTheFitIsExactAtEveryWidth|TestAFlashCarryingANewlineDoesNotBecomeASecondRow|TestTargetHintBarFitsAtTheSupportedMinimum)$$' ./internal/ui/tui/

verify-guards: ## Prove the golden guards fail — sabotages a throwaway copy, never this tree
	./scripts/verify-oracle-guards.sh

verify-batch-guard: ## Prove the batched-input tests fail when either guard is removed
	./scripts/verify-batch-guard.sh

verify-waitdelay-guard: ## Prove the bounded-read test fails when the read bound is removed
	./scripts/verify-waitdelay-guard.sh

verify-hintbar-guard: ## Prove the width table fails when the hint bar fit is removed, and only below 80
	./scripts/verify-hintbar-guard.sh

rebaseline-golden: ## Re-anchor the golden to current behaviour — reviewed behaviour changes only
	go test -count=1 -v -run '^TestCharacterization$$' ./internal/app/ -rebaseline

tidy-check: ## Fail if go.mod/go.sum are not tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

run: build ## Launch the TUI against testdata/fixtures
	cd $(FIXTURES) && $(CURDIR)/$(BINARY)

demo-git: build ## Build a throwaway workspace covering every git display state
	./scripts/demo-git.sh

demo-hung-git: build ## Build a throwaway workspace whose git never answers, to watch a read degrade
	./scripts/demo-hung-git.sh
