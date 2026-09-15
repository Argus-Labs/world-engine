#!/usr/bin/env bash
set -euo pipefail

mode="${1:?expected all, unit, or ci}"
shift
export TEST_SEED="${TEST_SEED:-$(date +%s)}"
runner_args=(bin/tools/gotestsum)
go_args=(./pkg/...)
case "$mode" in
  all) target=test ;;
  unit) target=test-unit; go_args+=('-tags=!integration') ;;
  ci)
    target=test-ci
    runner_args+=(--junitfile=junit.xml)
    go_args+=(-coverprofile=coverage.out -covermode=atomic)
    ;;
  *) echo "Unknown test mode: $mode" >&2; exit 2 ;;
esac
printf 'To reproduce: TEST_SEED=%s moon run world-engine:%s\n' "$TEST_SEED" "$target"
exec "${runner_args[@]}" -- "${go_args[@]}" "$@"
