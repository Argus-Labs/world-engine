#!/usr/bin/env bash
# Verify the public entity contract and the plugins/templates migrated from Ref.
set -euo pipefail
cd "$(dirname "$0")/.."
export TEST_SEED="${TEST_SEED:-1}"
bin/tools/gotestsum -- ./pkg/cardinal/... ./pkg/plugin/... ./pkg/template/... "$@"
