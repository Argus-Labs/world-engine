#!/usr/bin/env bash

set -eo pipefail

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd "$SCRIPT_DIR"
cd ..
buf generate
cd ..

cp -r github.com/argus-labs/cardinal/* ../
rm -rf github.com
