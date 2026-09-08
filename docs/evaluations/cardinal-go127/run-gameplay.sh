#!/usr/bin/env bash
set -euo pipefail
artifact_dir=$(cd "$(dirname "$0")" && pwd)
repo_dir=$(cd "$artifact_dir/../../.." && pwd)
result_dir=${RESULT_DIR:-$(mktemp -d /tmp/cardinal-gameplay.XXXXXX)}
mkdir -p "$result_dir"
mkdir "$result_dir/source"
result_dir=$(cd "$result_dir" && pwd)
git -C "$repo_dir" archive "${BASE_REV:-d663f5b0}" | tar -x -C "$result_dir/source"
cp "$artifact_dir/baseline_test.go.txt" "$result_dir/source/pkg/cardinal/go127_eval_test.go"
cp "$artifact_dir/gameplay_test.go.txt" "$result_dir/source/pkg/cardinal/go127_gameplay_eval_test.go"
export GOTOOLCHAIN=go1.27.1
export GOCACHE=${GOCACHE:-$result_dir/cache}
export TEST_SEED=20260907
cd "$result_dir/source"
python3 - <<'PY'
from pathlib import Path
import re
p=Path('go.mod')
p.write_text(re.sub(r'^go \S+$', 'go 1.27.1', p.read_text(), flags=re.M))
PY
go version > "$result_dir/toolchain.txt"
go test -c -o "$result_dir/gameplay.test" ./pkg/cardinal
"$result_dir/gameplay.test" -test.run='^$' -test.bench='^BenchmarkGo127GameplaySemantics$' -test.benchtime=1x -test.v > "$result_dir/semantics.txt"
: > "$result_dir/gameplay.txt"
for ((sample=1;sample<=${SAMPLES:-10};sample++)); do
  if (( sample % 2 )); then apis='ref system entity'; else apis='entity system ref'; fi
  for api in $apis; do
    GOMAXPROCS=1 "$result_dir/gameplay.test" -test.run='^$' \
      -test.bench="^BenchmarkGo127Gameplay$/entities=${ENTITY_PATTERN:-.*}$/work=.*/api=$api$" \
      -test.benchtime="${BENCHTIME:-200ms}" -test.count=1 -test.cpu=1 -test.benchmem >> "$result_dir/gameplay.txt"
  done
  echo "Completed sample $sample/${SAMPLES:-10}"
done
benchstat -col '/api@(ref system entity)' "$result_dir/gameplay.txt" > "$result_dir/comparison.txt"
printf 'Results: %s\n' "$result_dir"
