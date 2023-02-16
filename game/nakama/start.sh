#!/bin/sh

set -o errexit -o nounset

rm -rf vendor/
go mod tidy
go mod vendor
docker compose up --build
