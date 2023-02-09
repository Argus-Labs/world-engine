#!/bin/sh

set -o errexit -o nounset

cd nakama

go mod tidy
go mod vendor

cd ..
