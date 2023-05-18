#!/bin/sh

set -o errexit -o nounset

cd nakama
rm -rf vendor/
go mod tidy
go mod vendor
cd ..

cd server
rm -rf vendor/
go mod tidy
go mod vendor
cd ..

docker compose up --build
