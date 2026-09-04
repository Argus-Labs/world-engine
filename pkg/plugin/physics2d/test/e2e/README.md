# physics2d end-to-end suite

Headless Cardinal worlds whose only job is to exercise `pkg/plugin/physics2d`
and fail on anything that does not behave the way Box2D documents. No NATS, no
Redis, no Docker; `task test` runs it with the rest of `./pkg/...`.

```sh
go test ./pkg/plugin/physics2d/test/e2e/                            # everything
go test ./pkg/plugin/physics2d/test/e2e/ -run TestScenarios/flags -v # one scenario, passing checks included
go run  ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -h         # the same suite as a CLI
```

## What runs

| Test | Pins |
|---|---|
| `TestScenarios` | Thirteen scripted scenarios, about 400 checks. Each runs in its own world as a parallel subtest, so `-run TestScenarios/flags` works and a failure names the scenario and the line |
| `TestExhaustiveBodyMatrix` | Every body type × every shape type × every combination of the five body flags (896 bodies, one world). Each flag is read back from the engine, not the component; the wire round-trip is lossless; a `Plugin.Reset` rebuild recreates the same engine state |
| `TestRandomScenes` | Seeded random scenes of valid bodies over a static floor. Invariants: lossless wire round-trip, no solid dynamic body ends up inside the floor, and the same seed simulates identically twice and at `Workers: 4` |
| `TestRestore` | Snapshot a world, round-trip every component through the wire format, rebuild it in a fresh world, simulate both on and compare. Once with the documented `Plugin.Reset`, once without |
| `TestDigestIsWorkerInvariant` | All scenarios together in one world, twice in the same configuration, then at `Workers: 4`. All three state hashes must match |
| `TestHostile` | Each crash-prone shape, alone in a child process |

Every check reports through the subtest's `testing.T`, attributed to the scenario's
own line; the harness's printed report is what the CLI shows instead.

`TestDigestIsWorkerInvariant` covers two different promises. Same configuration
twice is the only check that catches a Go map iterated for its side effects; no
worker-count or golden-file comparison can. Different worker counts pins the
engine's promise that `Config.Workers` is a throughput knob and nothing else.

### Generated coverage

`TestExhaustiveBodyMatrix` is the enumerated form of the question that started this
suite: does a Go zero value leak anywhere Box2D defaults to true? `testutils.Gen`
walks every combination, so nothing is left to a hand-picked sample. It found one
divergence, pinned in `knownDivergence`: the plugin accepts chains and edges on
dynamic bodies although the component docs say static or kinematic only. Enforcing
the rule flips that check, which is the signal to remove the entry.

`TestRandomScenes` is the physics analogue of `pkg/box2d`'s op-sequence fuzz. A
scene is a pure function of the seed in its subtest name; reproduce one with

```sh
PHYSICS2D_E2E_SCENE_SEED=<hex from the subtest name> go test ./pkg/plugin/physics2d/test/e2e/ -run TestRandomScenes -v
```

Its generator respects the documented static/kinematic-only rule for edges and
chains because the consequence of breaking it is not subtle: a dynamic body carrying
an edge falls straight through a 10 m static floor. Kinematic bodies collide with
nothing there, because one moving downward can legitimately squeeze a dynamic body
through the floor, and that would make the invariant unprovable.

### Scenarios

| Scenario | Pins |
|---|---|
| `defaults` | Constructor and wire defaults, the zero-value trap, `Validate` |
| `shapes` | All 7 `ShapeType`s reach Box2D and collide by their geometry |
| `bodytypes` | Static / dynamic / kinematic / manual, and the writeback rules |
| `flags` | Active, Awake, SleepingAllowed, Bullet, FixedRotation, gravity scale, damping; teleport with an explicit same-tick sleep |
| `material` | Friction mixes as `sqrt(a*b)`, restitution as `max(a,b)`, density becomes mass |
| `filtering` | Category/mask, one-sided masks, group index, full 64-bit filter width |
| `sensors` | Triggers vs contacts, compound slots, sleeping bodies, static visitors, disabled bodies |
| `contacts` | Event entities, shape slots, world-space normal and point, filters, Begin/End lifecycle |
| `compound` | Child offsets and rotations, slot identity, combined centre of mass |
| `queries` | Raycast, OverlapAABB (narrow-phase), CircleSweep, plus every documented edge case |
| `lifecycle` | Create, destroy, teleport, retype, resize (capsules included), add/remove shapes, refilter, retune |
| `stability` | 10-box stack, deep overlap recovery, 2 cm to 100 m shapes, 5 km from origin |
| `reset` | `Plugin.Reset` rebuild: poses, velocities, no replayed events, queries |

Two watchdogs run every tick regardless of scenario: every body is checked for
NaN/Inf and named if it goes bad, and the engine world is checked for
disappearing without a scenario asking for it.

Behaviour worth knowing, all pinned by passing checks:

- The engine clamps linear speed to **400 m/s**; the plugin exposes no way to raise it.
- The tick that rebuilds after `Plugin.Reset` reconciles but does not step, so one
  tick of simulated time is lost.
- `Transform2D.Rotation` is wrapped to `[-π, π]` on writeback.
- A spinning body whose centre of mass is off its origin gets linear velocity even
  with `Velocity2D.Linear` zero. That is Box2D, not a bug.
- `NewPhysicsBody2D` lives in `physics2d/component`; the plugin root does not
  re-export it.

### Crash-prone cases

`TestHostile` runs each case from `scenario.HostileNames()` in its own process,
because a shape that panics inside a tick would otherwise take every later test
with it. The child is `TestHostileChild`, driven by `PHYSICS2D_E2E_HOSTILE_CASE`.
The test binary's exit code is the verdict: 0 passed, 1 a check failed, 2 the
process panicked (reported as FATAL).

`knownFailures` in `e2e_test.go` lists the cases that fail today and why. A case
that starts passing fails the test until it is removed from that map, so an
engine change that fixes one is noticed rather than absorbed. Today that is only
`zero-extent-box`: `ColliderShape.Validate` accepts zero half-extents, the engine
builds a `(NaN, NaN)` body from them where C's assert would have fired, and the
reconciler then rejects the entity every tick for as long as it lives.

The rest reject cleanly or simulate: `destroy-during-contact`, `short-chain`,
`short-chain-loop`, `zero-radius-circle`, `negative-radius-circle`,
`polygon-no-vertices`, `polygon-two-vertices`, `polygon-too-many-vertices`,
`degenerate-capsule`, `chain-on-dynamic-body`. Note that a rejected shape retries
forever: the entity stays in ECS with no body and `ReconcileFromECS` logs the same
failure every tick.

## How it works

- Each scenario owns a **lane**, 300 m of world to itself. Scenario code is written
  in lane-local coordinates and the harness offsets them, so nothing in one
  scenario can collide with, or be found by a query from, another.
- A scenario has three hooks. `Setup` runs on `cardinal.Init`, before the plugin,
  so the first `FullRebuildFromECS` sees the bodies. `EachTick` runs on
  `PreUpdate`, before the physics pipeline; drive gameplay-owned bodies there.
  `Steps` run on `Update`, after the pipeline, where post-step positions and this
  tick's contact events are both visible.
- `harness.InitECS` reaches the unexported `ecs.World.Init` by reflection, and the
  restore driver reaches `ToProto`/`FromProto` the same way. `cardinal.World` only
  calls these inside `StartGame`, which needs NATS. It is the same shim the
  plugin's own integration tests use.

## Adding a check

```go
func MyThing() harness.Scenario {
	var s struct{ ball cardinal.EntityID }
	return harness.Scenario{
		Name: "mything",
		Setup: func(c *harness.Ctx) {
			s.ball = c.Spawn("ball", 0, 10, body(physics.BodyTypeDynamic, circle(0.5)))
		},
		Steps: []harness.Step{
			{Tick: 120, Do: func(c *harness.Ctx) {
				c.Near("the ball lands", c.Pos(s.ball).Y, 0.5, 0.15)
			}},
		},
	}
}
```

Add it to `scenario.All()`; it gets the next lane automatically.

Assert an **observable consequence**, never the component you just wrote: the
component is exactly where a flag that never reached Box2D still looks correct.
Use `c.Note(...)` for a measurement worth reading but not worth a hard threshold;
notes never fail the run.

## The crash-restore check

This is a different path from the `reset` scenario. `reset` calls `Plugin.Reset`
on a live world and rebuilds from ECS still in memory. A restore round-trips every
component through `MarshalWire`/`UnmarshalWire` first, which for `PhysicsBody2D`
means its custom decoder, the code that decides whether an absent field means
`false` or Box2D's default of `true`.

It reproduces Cardinal's real ordering: the game's Init systems spawn a whole
scene and the plugin builds a Box2D world from it, and only then does the restore
call `FromProto` and throw that ECS state away. The physics world is left
describing a scene that no longer exists.

The scene is built so every body carries at least one value that differs from the
default it would fall back to: no body has all three sleep flags true, none is
left at `GravityScale` 1 alone, filters use bits above 32 and both signs of group
index, and all seven shape types are present. A dropped or defaulted field shows
up as that field reverting, and the comparison names it.

## The CLI

```sh
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e             # the suite
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -v          # ...with passing checks
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -digest     # ...plus a state hash to diff between runs
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -workers 8  # engine worker count; the digest must not change
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -restore [-restore-no-reset]
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -hostile list
go run ./pkg/plugin/physics2d/test/e2e/cmd/physics2d-e2e -serve      # run as a real Cardinal shard (needs NATS)
```

## Layout

```
e2e_test.go                 scripted scenarios, restore, digest, hostile cases
exhaustive_matrix_test.go   every body/shape/flag combination
random_scenes_test.go       seeded random scenes and their invariants
cmd/physics2d-e2e/          the CLI
internal/harness/      lanes, the scenario API, the report, snapshot helpers, the runner
internal/scenario/     the scenarios, shape builders, the hostile cases
internal/restore/      the two-world crash-restore driver
internal/probe/        the Probe component that names every spawned body
```

The tree has a scoped exclusion in `.golangci.yaml` for `forbidigo`, `funlen` and
`goprintffuncname`; the reasons are in the comment there. Every correctness
linter still applies.
