#!/usr/bin/make -f

###############################################################################
###                              E2E Tests                                  ###
###############################################################################

test-sidecar:
	@docker-compose down -v --remove-orphans
	@docker-compose build --build-arg TEST_PACKAGE=./...tests/e2e/sidecar --build-arg GO_VERSION=1.19  tester newrollup celestia
	@docker-compose up --abort-on-container-exit --exit-code-from  tester newrollup celestia tester
.PHONY: test-sidecar
