#!/usr/bin/env bash
set -euo pipefail

artifact_dir=$(cd "$(dirname "$0")" && pwd)
repo_dir=$(cd "$artifact_dir/../../.." && pwd)
base_rev=${BASE_REV:-d663f5b0}
result_dir=${RESULT_DIR:-$(mktemp -d /tmp/cardinal-go127.XXXXXX)}
samples=${SAMPLES:-10}
benchtime=${BENCHTIME:-200ms}
mkdir -p "$result_dir"
mkdir "$result_dir/source"
result_dir=$(cd "$result_dir" && pwd)
git -C "$repo_dir" archive "$base_rev" | tar -x -C "$result_dir/source"
cp "$artifact_dir/baseline_test.go.txt" "$result_dir/source/pkg/cardinal/go127_eval_test.go"
cp "$artifact_dir/methods_test.go.txt" "$result_dir/source/pkg/cardinal/go127_methods_eval_test.go"
export GOCACHE=${GOCACHE:-$result_dir/cache}
export TEST_SEED=20260907
cd "$result_dir/source"

for version in 1.26.5 1.27.1; do
  python3 - "$version" <<'PY'
import re
import sys
from pathlib import Path
p = Path('go.mod')
p.write_text(re.sub(r'^go \S+$', 'go ' + sys.argv[1], p.read_text(), flags=re.M))
PY
  GOTOOLCHAIN=go"$version" go version > "$result_dir/toolchain-$version.txt"
  GOTOOLCHAIN=go"$version" go test -c -o "$result_dir/go$version.test" ./pkg/cardinal
  : > "$result_dir/go$version.txt"
done

if [[ ${RUN_TESTS:-1} == 1 ]]; then
  GOTOOLCHAIN=go1.27.1 go test ./pkg/... > "$result_dir/tests.txt" 2>&1
fi
"$result_dir/go1.27.1.test" -test.run='^$' -test.bench='^BenchmarkGo127Semantics$' -test.benchtime=1x -test.v > "$result_dir/semantics.txt"

for ((sample=1; sample<=samples; sample++)); do
  if (( sample % 2 )); then versions='1.26.5 1.27.1'; else versions='1.27.1 1.26.5'; fi
  for version in $versions; do
    GOMAXPROCS=1 "$result_dir/go$version.test" -test.run='^$' \
      -test.bench='^BenchmarkGo127(Projection|ProjectionMethod|Component|ComponentMethod)$' \
      -test.benchtime="$benchtime" -test.count=1 -test.cpu=1 -test.benchmem >> "$result_dir/go$version.txt"
  done
  echo "Completed paired sample $sample/$samples"
done

python3 - "$result_dir" <<'PY'
import sys
from pathlib import Path
root = Path(sys.argv[1])
for version in ['1.26.5', '1.27.1']:
    raw = (root / f'go{version}.txt').read_text()
    normalized = raw.replace('BenchmarkGo127ProjectionMethod/', 'BenchmarkGo127Projection/').replace('BenchmarkGo127ComponentMethod/', 'BenchmarkGo127Component/')
    (root / f'normalized-{version}.txt').write_text(normalized)
PY

benchstat -filter '.name:Go127Projection' -col '/api@(function method inline)' "$result_dir/normalized-1.27.1.txt" > "$result_dir/api-comparison.txt"
benchstat -filter '.name:Go127Projection' -col '/api@(inline method)' "$result_dir/normalized-1.27.1.txt" > "$result_dir/inline-comparison.txt"
benchstat -filter '.name:Go127Component' -col '/api@(ref entity)' "$result_dir/normalized-1.27.1.txt" > "$result_dir/component-comparison.txt"
benchstat -filter '.name:(Go127Projection OR Go127Component)' "$result_dir/go1.26.5.txt" "$result_dir/go1.27.1.txt" > "$result_dir/toolchain-comparison.txt"
printf 'Results: %s\n' "$result_dir"
