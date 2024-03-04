DIRS_E2E = e2e/tests/nakama e2e/testgames/game relay/nakama
DIRS_E2E_BENCHMARK = e2e/tests/bench e2e/testgames/gamebenchmark relay/nakama
ROOT_DIR := $(shell pwd)

export ENABLE_ADAPTER=false

e2e-nakama:
	$(foreach dir, $(DIRS_E2E), \
		cd $(dir) && \
		go mod tidy && \
		go mod vendor && \
		cd $(ROOT_DIR); \
	)

	@docker compose up --build game nakama test_nakama --abort-on-container-exit --exit-code-from test_nakama --attach test_nakama

e2e-benchmark:
	$(foreach dir, $(DIRS_E2E_BENCHMARK), \
		cd $(dir) && \
		go mod tidy && \
		go mod vendor && \
		cd $(ROOT_DIR); \
	)

	@docker compose -f docker-compose.benchmark.yml up --build --exit-code-from game_benchmark --abort-on-container-exit --attach game_benchmark


#################
#   unit tests	#
#################

.PHONY: unit-test

unit-test:
	cd $(filter-out $@,$(MAKECMDGOALS)) && go test ./... -coverprofile=coverage-$(shell basename $(PWD)).out -covermode=count -v

unit-test-all:
	$(MAKE) unit-test cardinal
	$(MAKE) unit-test evm
	$(MAKE) unit-test sign
	$(MAKE) unit-test relay/nakama

#################
#   swagger	    #
#################
swagger:
	swag init -g cardinal/server/server.go -o cardinal/server/docs/

