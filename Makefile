BINARY   := mkx
FIXTURES := testdata/fixtures

.PHONY: build test verify verify-parser verify-gitx verify-tui verify-guards verify-batch-guard rebaseline-golden tidy-check run demo-git

build: ## Compile the mkx binary
	go build -o $(BINARY) ./cmd/mkx

test: ## Run the full test suite
	go test ./...

verify: ## Check discovery behaviour against the committed golden
	# -count=1 is load-bearing: without it a cached result replays an earlier
	# pass, and the oracle reports green without having compared anything.
	go test -v -count=1 -run TestCharacterization ./internal/app/

verify-parser: ## Run the pure parser table tests
	go test -v -count=1 ./internal/adapter/makex/

verify-gitx: ## Run the pure git classifier and parser table tests
	# No git process is spawned by these: they drive the classifiers from
	# captured strings, so they pass identically on a box with no git.
	go test -v -count=1 ./internal/adapter/gitx/

verify-tui: ## Run the TUI keymap, modal and overlay tests by name
	go test -v -count=1 ./internal/ui/tui/

verify-guards: ## Prove the golden guards fail — sabotages a throwaway copy, never this tree
	./scripts/verify-oracle-guards.sh

verify-batch-guard: ## Prove the batched-input tests fail when either guard is removed
	./scripts/verify-batch-guard.sh

rebaseline-golden: ## Re-anchor the golden to current behaviour — reviewed behaviour changes only
	go test -count=1 -v -run '^TestCharacterization$$' ./internal/app/ -rebaseline

tidy-check: ## Fail if go.mod/go.sum are not tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

run: build ## Launch the TUI against testdata/fixtures
	cd $(FIXTURES) && $(CURDIR)/$(BINARY)

demo-git: build ## Build a throwaway workspace covering every git display state
	./scripts/demo-git.sh
