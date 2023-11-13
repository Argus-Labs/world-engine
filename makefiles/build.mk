rift:
	cd rift/proto && buf generate
.PHONY: rift

rollup:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh && \
	docker compose up chain --build --exit-code-from celestia-devnet

game:
	cd internal/e2e/tester/cardinal && go mod vendor
	@docker compose up game nakama --abort-on-container-exit postgres redis

forge-build: |
	@forge build --extra-output-files bin --extra-output-files abi  --root evm/precompile/contracts

rollup-build:
	@docker build evm


rollup-install:
	cd evm && $(MAKE) install

rollup-proto-gen:
	cd evm && $(MAKE) proto-gen
