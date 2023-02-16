#!/bin/sh

set -o errexit -o nounset

original_PWD=$PWD

cd "$(dirname "$0")"

go mod tidy

go mod vendor


cd "$original_PWD"
