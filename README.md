<div align="center"> <!-- markdownlint-disable-line first-line-heading -->
  <img alt="World Engine" src="https://i.imgur.com/P6YpZCT.png" width=250 />
  <br/>
  The world’s first Gamechain SDK that utilizes Argus Labs’ novel sharded rollup architecture.
  <br/>
  <br/>
  <a href="https://codecov.io/gh/Argus-Labs/world-engine" >
    <img alt="Code Coverage" src="https://codecov.io/gh/Argus-Labs/world-engine/branch/main/graph/badge.svg?token=XMH4P082HZ"/>
  </a>
  <a href="https://goreportcard.com/report/pkg.world.dev/world-engine/cardinal">
    <img src="https://goreportcard.com/badge/pkg.world.dev/world-engine/cardinal" alt="Go Report Card">
  </a>
  <a href="https://t.me/worldengine_dev" target="_blank">
    <img alt="Telegram Chat" src="https://img.shields.io/endpoint?color=neon&logo=telegram&label=chat&url=https%3A%2F%2Ftg.sumanjay.workers.dev%2Fworldengine_dev">
  </a>
  <a href="https://pkg.go.dev/pkg.world.dev/world-engine/cardinal" target="_blank">
    <img src="https://pkg.go.dev/badge/pkg.world.dev/world-engine/cardinal.svg" alt="Go Reference">
  </a>
  <a href="https://x.com/WorldEngineGG" target="_blank">
    <img alt="Twitter Follow" src="https://img.shields.io/twitter/follow/WorldEngineGG">
  </a>
</div>

## ⚠️ Work in Progress

Hey there!

We're in the process of rewriting the World Engine from the ground up.

Stay tuned!

## Development

Install [proto](https://moonrepo.dev/docs/proto/install), then run `proto install`
from the repository root to install the Go and moon versions in `.prototools`.
Keep proto's shims on your `PATH` so `go` and `moon` use those versions.

```sh
moon tasks                          # List project tasks
moon run world-engine:build          # Build Go packages
moon run world-engine:lint           # Lint (installs the pinned linter)
moon run world-engine:lint-fix       # Apply lint fixes
moon run world-engine:test           # Run all Go tests
moon run world-engine:test-unit      # Run without integration-tagged tests
moon run world-engine:test-ci        # Write coverage.out and junit.xml
TEST_SEED=123 moon run world-engine:test  # Replay a test seed
```

Go tools install into the ignored `bin/tools` directory. Tests print a seed for
reproduction. Moon task caching is disabled so randomized tests run each time.
Go's own compilation and test caches remain enabled.

Generated clients have separate projects:

- `moon run proto-csharp:build` or `proto-csharp:pack` builds or packages the C#
  client. Requires the .NET SDK. Debug variants use `build-debug` and `pack-debug`.
- `moon run proto-ts:build` type-checks the TypeScript client. Requires Node/npm
  and a prior `npm install` in `proto/gen/ts`.

Moon replaces both Taskfiles. Task names use hyphens, for example `test:ci`
becomes `world-engine:test-ci`. C# `rebuild` cleans before building, and `update`
reports outdated dependencies without changing them. Formatting and analysis
failures propagate to the caller.

CI runs the same Go tasks as local development. The Box2D workflow retains
its separate architecture matrix and explicit `determinism-*` targets. Workflows select
explicit targets instead of `moon ci`, keeping the expensive determinism checks
in their dedicated matrix. Generated clients can be selected explicitly
on runners with their SDKs installed. Cleanup and formatting never run automatically.
