# Use PWD instead of CURDIR for better cross-platform compatibility
ROOT_DIR := $(shell pwd)

start-da:
	@bash "$(ROOT_DIR)/evm/scripts/start-celestia-devnet.sh"

start-evm:
	@docker compose up chain --build --abort-on-container-exit

rollup:
	@docker compose down --volumes -v
	@bash "$(ROOT_DIR)/evm/scripts/start-celestia-devnet.sh" && \
	trap 'docker compose down' EXIT && \
	docker compose up chain --build --abort-on-container-exit --exit-code-from celestia-devnet

game:
	@docker compose up game nakama --build --abort-on-container-exit