---
name: cardinal-migration
description: Migrate a Cardinal game to world-engine v0.18 from v0.16.x (Run(w) systems, explicit Register* calls, Entity handles) or from v0.17.x and the retracted v1.0.x (NewTestWorld system tests, pprof removed). Use when upgrading world-engine in a game repo, or when a build fails with errors like `undefined: cardinal.BaseSystemState`, `undefined: cardinal.Ref`, `want Run(*cardinal.World)` or `unknown field Pprof in struct literal of type cardinal.WorldOptions`.
---

# Cardinal migration

Port a game to world-engine v0.18.0. v0.17 removed state-struct systems: systems are
types with `Run(w *cardinal.World)`, every component, command, event and system event is
registered on the world before `StartGame`, and searches yield `cardinal.Entity` handles.
From v0.17.1, World CLI ships in the world-engine module, and the project's go.mod pins
it. v0.18.0 keeps that API, snapshot format and generated code. It adds
`cardinal.NewTestWorld` for system tests and removes the pprof server and the debug
`StreamPerf` RPC.

v1.0.0 and v1.0.1 are retracted. v1.0.1 is v0.17.1 plus the retraction, so migrate a
v1.0.1 pin as v0.17.1. Semver sorts it above v0.18.0, so `go get` must name the version.

Game behavior must not change. Do not merge, split, reorder or redesign systems while
migrating. Leave game logic as it is, even where the new API invites a cleanup.

## From v0.17.x or v1.0.x

Skip the workflow below. The audit's old-API list is empty except for test reflection.

1. `go get github.com/argus-labs/world-engine@v0.18.0 && go mod tidy`.
2. Delete `Pprof:` from `WorldOptions` literals, and `CARDINAL_PPROF` from deploy config
   (runtime.md, Telemetry).
3. Run the repo's `world sdk generate`. The output must not change.
4. Port system tests to `cardinal.NewTestWorld` (bootstrap.md, Testing one system). The
   audit lists every test that reflects into `cardinal.World` to run Init systems or
   enqueue commands. Those are the first to port.
5. Verify (Done when). Snapshots load both ways, so nothing is wiped.

## Before editing

1. Run `go run <skill-dir>/scripts/audit.go <game-root>` and keep the output. It reports
   the pinned versions, a count per old pattern with example locations, stale generated
   code, and packages that look like Cardinal but do not import it.
2. Install `world` (runtime.md, Versions). Run
   `world sdk generate <backend-dir> --no-emit` on the untouched code, while go.mod still
   pins the old release. Its `undeclared` list names every type an old field used
   (`component.Health  (cardinal.Ref)`): that is your registration checklist. Any other
   violation is a wire shape (`[]T`, `map`, pointer or interface field). Each must be
   converted before generation succeeds, and a large count is the biggest job of the
   migration: see older-versions.md.
3. Only rewrite packages that import `github.com/argus-labs/world-engine/pkg/cardinal`.
   The audit lists look-alike packages (their own `BaseSystemState`, `WithCommand`, ...).
   Leave them alone. If a look-alike imports the game's generated wire code or `gen/`
   proto packages, stop and ask the user: World CLI does not generate code for its
   types, so the first regenerate deletes what it imports.
4. Write down the system and plugin registration order per hook from each shard's
   `main.go`. You will reproduce it exactly.
5. Tell the user whether snapshots must be wiped (Hard rules) before anything ships.
6. With more than a few dozen systems, write codemods (go/ast or go/types) for the
   mechanical steps instead of editing by hand: system conversion, plugin threading, and
   per-package test registration. A 224-system game needed six.

## Workflow

| Step | Do                                                                                                                    | Read                         |
| ---- | --------------------------------------------------------------------------------------------------------------------- | ---------------------------- |
| 1    | Starting anywhere but v0.16.7 (older, or the v0.16.8/9 interim API): read that section first                          | references/older-versions.md |
| 2    | `go get github.com/argus-labs/world-engine@v0.18.0`, add the World CLI tool, `go mod tidy`, set `go 1.27.1`           | references/runtime.md        |
| 3    | Add one `register(w *cardinal.World)` per shard: every Register* call, then plugins and systems in the recorded order | references/bootstrap.md      |
| 4    | Convert each system                                                                                                   | references/systems.md        |
| 5    | Convert entity, component and search code                                                                             | references/entities.md       |
| 6    | Rewire plugins                                                                                                        | references/plugins.md        |
| 7    | Once `go build` fails only in wire code, delete stale `wire.gen.go` files and run the repo's `world sdk generate`     | references/runtime.md        |
| 8    | `go build ./... && go vet ./...` until clean, then `go vet -tags` per audit build constraint. Fix by the error tables | each reference               |
| 9    | Port system tests to `cardinal.NewTestWorld`                                                                          | references/bootstrap.md      |
| 10   | Re-run the audit and `world sdk generate --no-emit`, then check every item the compiler cannot catch                  | references/hazards.md        |
| 11   | Verify (Done when)                                                                                                    |                              |

Generation cannot run between steps 2 and 7. Once go.mod pins v0.18, discovery
type-checks the backend and stops with `DISCOVERY FAILED` while any system still uses
the v0.16 API.

Convert one package at a time. In large games, convert helpers that take a search
alias, `*cardinal.BaseSystemState` or `cardinal.Ref[...]` before their callers.

## Hard rules

- Snapshots: v0.17 and v0.18 cannot read snapshots written by v0.16.8 or earlier. The
  version number did not change, so every boot fails with `proto: cannot parse invalid
  wire-format data` and the shard restart-loops. A v0.16.8-or-earlier build reading a
  newer snapshot restores a wrong world without an error. From those versions, each
  shard's snapshot must be deleted at deploy, and again before any rollback. v0.16.9,
  v0.17.x and v1.0.1 snapshots load in v0.18 and back. Tell the user which case applies.
  Locally, `world purge` deletes them with the whole local cluster. Never delete remote
  snapshots yourself.
- Register everything a system touches. Registration is no longer inferred from fields.
  A missing registration compiles, then panics on the first tick that reaches it.
- Keep registration order. Systems in one hook still run in registration order, and
  same-tick system events depend on it.
- Store `e.ID()` (a `cardinal.EntityID`) in components, commands, maps and system fields,
  and rebind with `w.Entity(id)`. Never keep a `cardinal.Entity` across ticks.
- Pass component and event values, not pointers: `e.Set(c)`, `w.Broadcast(ev)`.
  Pointers compile and panic.

## Done when

- `go build ./...` and `go vet ./...` pass, and `go vet -tags <tags> ./...` with tags that
  satisfy each build constraint the audit lists. `go build` skips test files.
- `scripts/audit.go` exits 0: no old API, no hazards, no stale generated code, and no
  command, event, system event or component used without a Register* call.
- `world sdk generate --no-emit` reports nothing blocking, and every `ORPHANS` entry is
  either registered or was unused in v0.16 too. `--no-emit` exits 0 on orphans.
- Each shard passes `cardinal.RunDST(t, register, pre)` on several seeds. Each seed fuzzes
  a random subset of commands, and the seed is fixed per process, so `-count` repeats it.
  Run separate processes, for example
  `for i in 1 2 3 4 5; do go test -count=1 -run DST ./shards/<id>/ || break; done`.
  Put commands that unlock deeper paths (a kill, a match start) in `pre`.
  From v0.18.0, rerunning the same test with the logged `TEST_SEED` replays its op
  schedule and command payloads. The per-test seed mixes in the test name, so keep it.
  In v0.17.x, `TEST_SEED` does not reproduce a run that fuzzes any command; keep the
  failing log's `op_weights` and stack trace instead.
- Existing game tests pass, including build-tagged ones (`go test -tags <tags> ./...`).
  Tests that built state structs are rewritten as described in bootstrap.md (Tests).
  System tests use `cardinal.NewTestWorld` (bootstrap.md, Testing one system). Ported
  tests keep every scenario and assertion. Where the 1-second step clock changes what a
  test observes, say so in the summary.
- Your summary to the user states whether snapshots must be wiped, any hazard from
  hazards.md that applied, and anything you left unmigrated with the reason.
