rift:
	cd rift/proto && buf generate
.PHONY: rift

start-da:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh


start-evm:
	@docker compose up chain --build --abort-on-container-exit

rollup:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh && \
	docker compose up chain --build --abort-on-container-exit --exit-code-from celestia-devnet

game:
	cd internal/e2e/tester/game && go mod vendor
	@docker compose up game nakama --build --abort-on-container-exit cockroachdb redis

forge-build: |
	@forge build --extra-output-files bin --extra-output-files abi  --root evm/precompile/contracts

rollup-build:
	@docker build evm


rollup-install:
	cd evm && $(MAKE) install

rollup-proto-gen:
	cd evm && $(MAKE) proto-gen
