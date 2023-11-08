DIRS = internal/nakama internal/e2e/tester/cardinal relay/nakama
ROOT_DIR := $(shell pwd)

export ENABLE_ADAPTER=false

e2e-nakama:
	$(foreach dir, $(DIRS), \
		cd $(dir) && \
		go mod tidy && \
		go mod vendor && \
		cd $(ROOT_DIR); \
	)

	@docker compose up --build --abort-on-container-exit --exit-code-from test_nakama --attach test_nakama

#################
#   unit tests	#
#################

.PHONY: unit-test

unit-test:
	cd $(filter-out $@,$(MAKECMDGOALS)) && go test ./... -coverprofile=coverage-$(filter-out $@,$(MAKECMDGOALS)).out -covermode=count -v

unit-test-all:
	$(MAKE) unit-test cardinal
	$(MAKE) unit-test chain
	$(MAKE) unit-test sign
