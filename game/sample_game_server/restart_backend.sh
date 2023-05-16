#!/bin/sh

set -o errexit -o nounset

docker compose stop server
docker compose up server --build -d
