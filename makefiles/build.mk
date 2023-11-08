rift:
	cd rift/proto && buf generate
.PHONY: rift

rollup:
	./chain/scripts/start.sh --build

game:
	@docker compose up game nakama --abort-on-container-exit postgres redis

forge-build: |
	@forge build --extra-output-files bin --extra-output-files abi  --root chain/precompile/contracts

rollup-build:
	@docker build chain


rollup-install:
	cd chain && $(MAKE) install

rollup-proto-gen:
	cd chain && $(MAKE) proto-gen
