BINARY   := mkx
FIXTURES := testdata/fixtures

.PHONY: build test verify verify-guards rebaseline-golden tidy-check run

build: ## Compile the mkx binary
	go build -o $(BINARY) ./cmd/mkx

test: ## Run the full test suite
	go test ./...

verify: ## Check discovery behaviour against the committed golden
	go test -v -run TestCharacterization ./internal/app/

verify-guards: ## Prove the golden guards fail — sabotages a throwaway copy, never this tree
	./scripts/verify-oracle-guards.sh

rebaseline-golden: ## Re-anchor the golden to current behaviour — reviewed behaviour changes only
	go test -count=1 -v -run '^TestCharacterization$$' ./internal/app/ -rebaseline

tidy-check: ## Fail if go.mod/go.sum are not tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

run: build ## Launch the TUI against testdata/fixtures
	cd $(FIXTURES) && $(CURDIR)/$(BINARY)
