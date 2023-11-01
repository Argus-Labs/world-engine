rift:
	cd rift/proto && buf generate
.PHONY: rift

rollup:
	./chain/scripts/start.sh --build

game:
	@docker compose -f docker-compose-integration-test.yml up game nakama

forge-build: |
	@forge build --extra-output-files bin --extra-output-files abi  --root chain/contracts

rollup-build:
	cd chain && docker compose build
