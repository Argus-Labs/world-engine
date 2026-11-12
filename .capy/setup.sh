#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
export PATH="$HOME/.local/bin:$PATH"

stamp=".moon/cache/capy-setup.sha256"
inputs="$(sha256sum .agents/setup .prototools go.mod go.sum .capy/setup.sh)"
if [[ "${1:-}" == "initialize" || ! -f "$stamp" || "$(cat "$stamp")" != "$inputs" ]]; then
	./.agents/setup
	mkdir -p "$(dirname "$stamp")"
	printf '%s\n' "$inputs" > "$stamp"
fi

./.agents/resume
