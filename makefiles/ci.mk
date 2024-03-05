#################
# golangci-lint #
#################

golangci_version=v1.56.2

golangci-install:
	@echo "--> Installing golangci-lint $(golangci_version)"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(golangci_version)

lint:
	@$(MAKE) golangci-install
	@echo "--> Running linter"
	@go list -f '{{.Dir}}/...' -m | xargs golangci-lint run  --timeout=10m --concurrency 8 -v

golangci-fix:
	@$(MAKE) golangci-install
	@echo "--> Running linter"
	@go list -f '{{.Dir}}/...' -m | xargs golangci-lint run  --timeout=10m --fix --concurrency 8 -v


.PHONY: tidy

tidy:
	cd $(filter-out $@,$(MAKECMDGOALS)) && go mod tidy


GO_DIRS := $(shell find . -name "go.mod" -exec dirname {} \;)

.PHONY: tidy-all

tidy-all:
	@for dir in $(GO_DIRS); do \
		echo "Running go mod tidy in $$dir"; \
		(cd $$dir && go mod tidy); \
	done