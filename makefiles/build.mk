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
	cd e2e/testgames/game && go mod vendor
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
