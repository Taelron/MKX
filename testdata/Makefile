.PHONY: hello prompt long-running fail multiline no-desc greet confirm

hello: ## Print a greeting
	@echo "Hello from mkx test fixtures!"

prompt: ## Ask for user input
	@read -p "Enter your name: " name && echo "Hello, $$name!"

long-running: ## Simulate a long task (5s)
	@echo "Starting long task..."
	@for i in 1 2 3 4 5; do echo "Step $$i/5..."; sleep 1; done
	@echo "Done!"

fail: ## Always exits with error
	@echo "This target will fail."
	@exit 1

multiline: ## Print lots of output
	@for i in $$(seq 1 50); do echo "Line $$i: The quick brown fox jumps over the lazy dog"; done

no-desc:
	@echo "This target has no description."

NAME ?= world
greet: ## Greet someone (usage: make greet NAME=Alice)
	@echo "Hello, $(NAME)!"

confirm: ## Ask for confirmation before proceeding
	@read -p "Are you sure? [y/N] " ans && [ "$$ans" = "y" ] && echo "Confirmed!" || echo "Cancelled."
