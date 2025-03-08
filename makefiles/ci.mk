#################
# golangci-lint #
#################

lint_version=latest

lint-install:
	@echo "--> Checking if golangci-lint $(lint_version) is installed"
	@if ! command -v golangci-lint >/dev/null 2>&1 || [ "$$(golangci-lint --version 2>/dev/null | awk '{print $$4}')" != "$(lint_version)" ]; then \
		echo "--> Installing golangci-lint $(lint_version)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(lint_version); \
	else \
		echo "--> golangci-lint $(lint_version) is already installed"; \
	fi
	
lint:
	@$(MAKE) lint-install
	@echo "--> Running linter"
	@for dir in $(GO_DIRS); do \
		echo "Running linter in \"$$dir\""; \
		(cd "$$dir" && golangci-lint run --timeout=10m ./...); \
	done

lint-cardinal:
	@$(MAKE) lint-install
	@echo "--> Running linter only on ./cardinal"
	@(cd cardinal && golangci-lint run ./... --timeout=10m)

lint-fix:
	@$(MAKE) lint-install
	@echo "--> Running linter"
	@for dir in $(GO_DIRS); do \
		echo "Running linter with fix in \"$$dir\""; \
		(cd "$$dir" && golangci-lint run --timeout=10m --fix ./...); \
	done

push-check:
	@$(MAKE) lint
	@$(MAKE) unit-test-all
	@$(MAKE) e2e-nakama

.PHONY: tidy

tidy:
	cd "$(filter-out $@,$(MAKECMDGOALS))" && go mod tidy


GO_DIRS := $(shell find . -type f -name "go.mod" -exec dirname {} \;)

.PHONY: tidy-all

tidy-all:
	@for dir in $(GO_DIRS); do \
		echo "Running go mod tidy in \"$$dir\""; \
		(cd "$$dir" && go mod tidy); \
	done
