dc:
	docker compose up --build
.PHONY: dc


reset:
	rm -rf vendor/
	go mod tidy
	go mod vendor
	docker compose up --build
.PHONY: reset
