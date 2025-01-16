SHELL := /bin/bash

# Use PWD instead of CURDIR for better cross-platform compatibility
ROOT_DIR := $(shell pwd)

rift:
	cd rift/proto && buf generate
.PHONY: rift

contracts:
	@forge build --extra-output-files bin --extra-output-files abi  --root evm/precompile/contracts
	cd evm/precompile/contracts && go generate

rollup-build:
	@docker build -f evm/Dockerfile .

rollup-install:
	cd evm && $(MAKE) install

rollup-proto-gen:
	cd evm && $(MAKE) proto-gen

world-docs:
	mintlify --version || npm i -g mintlify
	cd docs && mintlify dev

# Find all directories containing go.mod files
GO_MOD_DIRS := $(shell find . -type f -name "go.mod" -exec dirname {} \;)

# Runs go generate ./... in all go.mod directories
generate:
	@echo "Running go generate..."
	@for dir in $(GO_MOD_DIRS); do \
		(cd "$$dir" && go generate ./...); \
	done
	@echo "Go generate completed successfully."

.PHONY: generate
