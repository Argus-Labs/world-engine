rift:
	cd rift/proto && buf generate
.PHONY: rift

rollup:
	cd chain && go mod vendor
	./chain/scripts/start.sh --build

game:
	@docker compose up game nakama

forge-build: |
	@forge build --extra-output-files bin --extra-output-files abi  --root chain/precompile/contracts

rollup-build:
	cd chain && docker compose build
