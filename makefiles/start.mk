
start-da:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh

start-evm:
	@docker compose up chain --build --abort-on-container-exit

rollup:
	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh && \
	trap 'docker compose down' EXIT; \
	docker compose up chain --build --abort-on-container-exit --exit-code-from celestia-devnet


game:
	cd e2e/testgames/game && GOWORK=off go mod vendor
	@docker compose up game nakama --build --abort-on-container-exit cockroachdb redis