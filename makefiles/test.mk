DIRS_E2E = internal/nakama internal/e2e/tester/game relay/nakama
DIRS_E2E_BENCHMARK = internal/nakama internal/e2ebenchmark/tester/cardinal relay/nakama
ROOT_DIR := $(shell pwd)

export ENABLE_ADAPTER=false

e2e-nakama:
	$(foreach dir, $(DIRS_E2E), \
		cd $(dir) && \
		go mod tidy && \
		go mod vendor && \
		cd $(ROOT_DIR); \
	)

	@docker compose up --build #--abort-on-container-exit --exit-code-from test_nakama --attach test_nakama

e2e-benchmark:
	$(foreach dir, $(DIRS_E2E_BENCHMARK), \
		cd $(dir) && \
		go mod tidy && \
		go mod vendor && \
		cd $(ROOT_DIR); \
	)

	@docker compose -f docker-compose.benchmark.yml up --build #--attach test_nakama_benchmark


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
