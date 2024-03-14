SHELL := /bin/bash

rift:
	cd rift/proto && buf generate
.PHONY: rift

start-da:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh


start-evm:
	@docker compose up chain --build --abort-on-container-exit

rollup:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh && \
	trap 'docker compose down chain celestia-devnet' EXIT; \
	docker compose up chain --build --abort-on-container-exit --exit-code-from celestia-devnet

game:
	cd e2e/testgames/game && GOWORK=off go mod vendor
	@docker compose up game nakama --build --abort-on-container-exit cockroachdb redis


contracts:
	@forge build --extra-output-files bin --extra-output-files abi  --root evm/precompile/contracts
	cd evm/precompile/contracts && go generate

rollup-build:
	@docker build evm


rollup-install:
	cd evm && $(MAKE) install

rollup-proto-gen:
	cd evm && $(MAKE) proto-gen

world-docs:
	mintlify --version || npm i -g mintlify
	cd docs && mintlify dev

# Find all directories containing go.mod files
GO_MOD_DIRS := $(shell find . -name "go.mod" -exec dirname {} \;)


# Runs go generate ./... in all go.mod directories.
generate:
	@echo "Running go generate..."
	@for dir in $(GO_MOD_DIRS); do \
		(cd $$dir && go generate ./...); \
	done
	@echo "Go generate completed successfully."

.PHONY: generate
