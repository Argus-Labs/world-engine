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
	node ./scripts/pre-speakeasy.js "$(GEN_DIR)/openapi.json"
	mv "$(GEN_DIR)/openapi.json" $(SWAGGER_DIR)
	@echo "--> Cleanup"
	rm -rf $(TMP_DIR)

speakeasy:
	$(MAKE) swagger-codegen
	speakeasy run
	./scripts/post-speakeasy.sh
