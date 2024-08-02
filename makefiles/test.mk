SHELL := /bin/bash
DIRS_E2E = e2e/tests/nakama e2e/testgames/game relay/nakama
DIRS_E2E_BENCHMARK = e2e/tests/bench e2e/testgames/gamebenchmark relay/nakama
DIRS_E2E_EVM = e2e/tests/evm e2e/testgames/game relay/nakama
ROOT_DIR := $(shell pwd)

e2e-nakama:
	@echo "--> Purging running Docker containers, if any"
	@docker compose rm --force --stop
	
	$(foreach dir, $(DIRS_E2E), \
		cd $(dir) && \
		go mod tidy && \
		cd $(ROOT_DIR); \
	)

	@(set -o pipefail; \
		(. ${CURDIR}/evm/scripts/start-celestia-devnet.sh && \
		export CARDINAL_ROLLUP_ENABLED=true && \
	    docker compose up --build chain -d && \
	    sleep 2 && \
		docker compose up --build chain game nakama test_nakama --abort-on-container-exit --exit-code-from test_nakama 2>&1) | grep --color=force "test_nakama  "; \
		docker compose rm --force --stop)

e2e-benchmark:
	@echo "--> Purging running Docker containers, if any"
	@docker compose rm --force --stop

	$(foreach dir, $(DIRS_E2E_BENCHMARK), \
		cd $(dir) && \
		go mod tidy && \
		go mod vendor && \
		cd $(ROOT_DIR); \
	)

	@docker compose -f docker-compose.benchmark.yml up --build --exit-code-from game_benchmark --abort-on-container-exit --attach game_benchmark
	@docker compose rm --force --stop


# check_url takes a URL (1), and an expected http status code (2), and will continuously ping the URL until it either
# gets the code, or the timeout is reached (180s).
# to call this function in make: `$(call check_url,localhost:1317,501)`
define check_url
	@echo "Checking $(1) with curl..."
	@timeout=60; \
	start=$$(date +%s); \
	while [ $$(( $$(date +%s) - start )) -lt $$timeout ]; do \
		if curl -s -o /dev/null -w "%{http_code}" $(1) -m 1 | grep -q "$(2)"; then \
			echo "Curl successful."; \
			exit 0; \
		else \
			echo "Waiting for response..."; \
			sleep 5; \
		fi; \
	done; \
	echo "Timeout reached. No response from $(1)."; \
	exit 1;
endef

e2e-evm:
	@echo "--> Purging running Docker containers, if any"
	@docker compose rm --force --stop
	
	$(foreach dir, $(DIRS_E2E), \
		cd $(dir) && \
		go mod tidy && \
		cd $(ROOT_DIR); \
	)

	@. ${CURDIR}/evm/scripts/start-celestia-devnet.sh && \
		docker compose up chain --build -d

	@CARDINAL_ROLLUP_ENABLED=true docker compose up game nakama -d

	@go test -v ./e2e/tests/evm/evm_test.go
	@docker compose rm --force --stop

.PHONY: e2e-evm


#################
#   unit tests	#
#################

.PHONY: unit-test

unit-test:
	cd $(filter-out $@,$(MAKECMDGOALS)) && GOWORK=off go test -coverpkg=./... ./... -coverprofile=coverage-$(shell basename $(PWD)).out -covermode=count -v

unit-test-all:
	$(MAKE) unit-test cardinal
	$(MAKE) unit-test evm
	$(MAKE) unit-test sign
	$(MAKE) unit-test relay/nakama

#################
#   swagger	    #
#################

.PHONY: swaggo-install

swaggo-install:
	echo "--> Installing swaggo/swag cli"
	go install github.com/swaggo/swag/cmd/swag@latest

swagger:
	$(MAKE) swaggo-install
	swag init -g cardinal/server/server.go -o cardinal/server/docs/ --parseDependency

swagger-check:
	$(MAKE) swaggo-install

	@echo "--> Generate latest Swagger specs"
	cd cardinal && \
		mkdir -p .tmp/swagger && \
		swag init -g server/server.go -o .tmp/swagger --parseInternal --parseDependency

	@echo "--> Compare existing and latest Swagger specs"
	cd cardinal && \
		docker run --rm -v ./:/local-repo ghcr.io/argus-labs/devops-infra-swagger-diff:2.0.0 \
		/local-repo/server/docs/swagger.json /local-repo/.tmp/swagger/swagger.json && \
		echo "swagger-diff: no changes detected"

	@echo "--> Cleanup"
	rm -rf .tmp/swagger

#####################
#  swagger codegen  #
#####################

.PHONY: swagger-codegen
SWAGGER_DIR = cardinal/server/docs
GEN_DIR = .tmp/swagger-codegen

swagger-codegen-install:
	@echo "--> Installing swagger-codegen"
	wget https://repo1.maven.org/maven2/io/swagger/codegen/v3/swagger-codegen-cli/3.0.57/swagger-codegen-cli-3.0.57.jar -O swagger-codegen-cli.jar
	echo '#!/usr/bin/java -jar' > swagger-codegen
	cat swagger-codegen-cli.jar >> swagger-codegen
	chmod +x swagger-codegen
	mv swagger-codegen ~/.local/bin/
	rm swagger-codegen-cli.jar

swagger-codegen:
	@echo "--> Generating OpenAPI v3.0 document from $(SWAGGER_DIR)"
	swagger-codegen generate -l openapi -i "$(SWAGGER_DIR)/swagger.json" -o $(GEN_DIR)
	node ./scripts/customize-openapi.js "$(GEN_DIR)/openapi.json"
	mv "$(GEN_DIR)/openapi.json" $(SWAGGER_DIR)
	@echo "--> Cleanup"
	rm -rf $(TMP_DIR)
