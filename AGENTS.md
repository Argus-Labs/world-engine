# World Engine

## Go skills

- Before designing, editing, or reviewing Go code, read and follow [go-development](.agents/skills/go-development/SKILL.md).
- For Go command-line or terminal UI work, also read and follow [go-cli](.agents/skills/go-cli/SKILL.md).
- Existing repository conventions take precedence over the skills' defaults. Keep changes scoped to the request.

## Development and verification

Use the toolchain versions pinned in `.prototools` and `go.mod`. See [README.md](README.md#development) for setup. Moon task definitions live in [moon.yml](moon.yml).

Run commands from the repository root:

```sh
moon run world-engine:build       # Build Go packages under pkg/
moon run world-engine:lint        # Run the pinned linter on pkg/
moon run world-engine:test        # Run Go tests under pkg/
moon run world-engine:test-ci     # Run pkg/ tests with coverage and JUnit reports
moon run cli:build                # Build the World CLI under cli/
moon run cli:lint                 # Lint cli/ with cli/.golangci.yaml
moon run cli:test                 # Run World CLI unit tests
```

For focused checks, use `go tool gotestsum -- ./pkg/<package>/...`. Reproduce a failing randomized test with its logged seed, for example `TEST_SEED=123 moon run world-engine:test`.

For Box2D changes, run the relevant `determinism-*` tasks defined in `moon.yml`. Keep formatting and lint rules aligned with `.golangci.yaml`. Choose checks appropriate to the change and report their results, including any failures or checks that could not run.