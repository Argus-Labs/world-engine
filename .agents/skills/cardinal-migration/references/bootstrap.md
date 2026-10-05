# Bootstrap and registration

v0.16 learned what a game used by reflecting over each system's state struct and
registering every `WithCommand`, `WithEvent` and `Ref` field it found. v0.17 does not.
`main` declares every type up front, and `RegisterSystem` only stores `s.Run(w)`.

## Mapping

| v0.16.7                                             | v0.17                             | Notes                                                  |
| --------------------------------------------------- | --------------------------------- | ------------------------------------------------------ |
| `cardinal.RegisterSystem(world, fn, opts...)`       | `w.RegisterSystem(&S{}, opts...)` | Takes a constructed `System`. No func adapter.         |
| `cardinal.RegisterSystemV2(world, &S{})`            | `w.RegisterSystem(&S{})`          | V2 is gone.                                            |
| `cardinal.RegisterPlugin(world, p)`                 | `w.RegisterPlugin(p)`             | Plugins register their own types.                      |
| implicit via `Ref[C]` / `WithComponent[C]`          | `w.RegisterComponent[C]()`        | Every component in an archetype, `Set`, or a snapshot. |
| implicit via `WithCommand[T]`                       | `w.RegisterCommand[T]()`          | Also opens the client and inter-shard handler.         |
| implicit via `WithEvent[T]`                         | `w.RegisterEvent[T]()`            | Keyed by Go type, not `Name()`.                        |
| implicit via `WithSystemEvent*[T]`                  | `w.RegisterSystemEvent[T]()`      |                                                        |
| `NewWorld`, `StartGame`, `WithHook`, `WorldOptions` | unchanged                         | A release with `NewTestWorld` drops `Pprof`.           |

## Shape

Put all registration for a shard in one function and call it from `main`, tests and DST:

```go
func main() {
	w, err := cardinal.NewWorld(cardinal.WorldOptions{ /* unchanged */ })
	if err != nil {
		panic(err.Error())
	}
	register(w)
	w.StartGame()
}

func register(w *cardinal.World) {
	// Types first. Order among Register* calls does not matter.
	w.RegisterComponent[component.PlayerTag]()
	w.RegisterCommand[system.AttackPlayerCommand]()
	w.RegisterEvent[event.PlayerDeath]()
	w.RegisterSystemEvent[systemevent.PlayerDeath]()

	// Plugins and systems: keep the exact v0.16 order within each hook.
	w.RegisterSystem(&system.PlayerSpawnerSystem{}, cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(&system.AttackPlayerSystem{})
}
```

## Tests

Shard-level tests (DST, E2E) call the same function. A test in `package main` calls
`register` directly. A test in `package main_test` cannot, so export it (`Register`) or
move it to a small package the shard and its tests both import. The v0.17 templates keep
a copy of the list in `main_test.go`; a copy drifts, so share one function instead. The
multi-shard template's root `dst_game_test.go` registers every shard, and Go cannot
import a `main` package. There, put `Register` in a non-main package such as
`shards/<id>/system`.

Package tests below the shard cannot import its `register` (import cycle). Give each such
package a `registerTestTypes(w)` helper that registers what its systems touch, built from
the old state structs.

## Testing one system

Check whether the target release has the single-system harness:

```sh
go doc github.com/argus-labs/world-engine/pkg/cardinal NewTestWorld
```

It may first print `k8s.io/... invalid version` lines. Ignore them. The last line contains
`no symbol NewTestWorld` when the release lacks it.

v0.17.0 and v0.17.1 do not have it. There, `w.Tick` before `StartGame` panics with
`Tick called before initialization`. Cover system behavior with `RunDST` plus
`preTestCommands`, or `RunE2E`. A test that ran a system on a zero state struct and never
touched it can call `(&S{}).Run(nil)` only if `Run` never uses `w`.

A release with `NewTestWorld` also removes `WorldOptions.Pprof`, `CARDINAL_PPROF` and the
debug `StreamPerf` RPC. Delete `Pprof:` from `WorldOptions` literals and the variable from
deploy config. Per-system timing comes from the OTel `cardinal.system` spans instead.

When `go doc` finds it, port each v0.16 system test to `cardinal.NewTestWorld`. Pass the
shard's `register` as setup, or the package's `registerTestTypes` below the shard:

```go
func TestAttackKillsPlayer(t *testing.T) {
	w := cardinal.NewTestWorld(t, registerTestTypes)
	bob := w.Create[system.Player]()
	bob.Set(component.PlayerTag{Nickname: "bob"})
	bob.Set(component.Health{HP: 10})

	w.Command("alice", system.AttackPlayerCommand{Target: "bob", Damage: 10})
	w.RunSystem(&system.AttackPlayerSystem{})

	assert.False(t, bob.Alive())
	assert.Equal(t, []systemevent.PlayerDeath{{Nickname: "bob"}},
		w.Emitted[systemevent.PlayerDeath]())
}
```

| v0.16 test harness                                        | With `NewTestWorld`                                                         |
| --------------------------------------------------------- | --------------------------------------------------------------------------- |
| `cardinal.NewWorld` with `StorageTypeNop` and `t.Setenv`  | `cardinal.NewTestWorld(t, setup)`. It reads no env and connects to nothing. |
| Reflection on the unexported `world` field to call `Init` | Delete it. `NewTestWorld` runs the Init systems that setup registers.       |
| Init-hook seed system that creates entities               | `w.Create[A]()` and `e.Set(c)` in the test body.                            |
| Observer system writing to a package-level variable       | Read state after the step: `e.Get[C]()`, `w.Exact[A]().Iter()`.             |
| Register the system under test, then `w.Tick(ts)`         | `w.RunSystem(&S{})` runs one system. `w.Tick()` runs the full schedule.     |
| Observer system collecting events or system events        | `w.Events[E]()`, `w.Emitted[SE]()`, `w.ShardCommands[C]()`                  |

Rules:

- Setup must register every type the system touches. `RunSystem` registers nothing, but
  the system itself need not be registered.
- Setup also runs every Init system it registers. A `register` with spawners starts the
  test with their entities. Use a narrower setup when the test expects an empty world.
- `w.Command` fails the test for an unregistered command. The command reaches the next
  step only.
- `Events`, `Emitted` and `ShardCommands` return the last step's outputs. Events sent by
  Init systems appear after the first step.
- Step n runs at `time.Unix(height, 0)`: one second per step. A test that needs other
  timestamps (1 ms ticks, a fixed date) calls `w.World.Tick(ts)`, which runs the full
  schedule at `ts`. Assert only on state after it: `Events`, `Emitted` and
  `ShardCommands` do not describe that tick. `RequireDeterministic` checks such ticks
  only through its final digest after the script, not step by step.
- `w.StartGame()` fails the test.

A system test that relied on package-level state, `time.Now` or map order can also run
its script through `cardinal.RequireDeterministic(t, setup, script)`. It fails at the
first step where two fresh runs differ. If it fails on a ported test, report it. Do not
change the system while migrating. Both runs share one process, so a pass can still hide
map iteration order over a few keys and coarse clock reads. Read ported systems for
`range` over a map and `time.Now()` and report them. In code you write, range
`slices.Sorted(maps.Keys(m))` and use `w.Timestamp()` instead of `time.Now()`.

## What to register

Build the list from the old state structs before you delete them:

- every `WithCommand[T]` field type -> `RegisterCommand[T]`
- every `WithEvent[T]` -> `RegisterEvent[T]`
- every `WithSystemEventEmitter[T]` / `WithSystemEventReceiver[T]` -> `RegisterSystemEvent[T]`
- every `Ref[C]` / `WithComponent[C]` inside a search -> `RegisterComponent[C]`
- commands the shard only receives from other shards (for example
  `lobby.NotifySessionStartCommand`) -> `RegisterCommand`. Commands it only sends with
  `w.SendToShard` need no registration.

Re-registering the same type is a no-op, so duplicates are harmless.

## Rules

- Every Register* panics after `StartGame`, including from inside an Init system:
  `cannot register after the world has started; register before StartGame and outside systems`.
- Two Go types with the same component or system-event `Name()` now panic at registration
  (`component X already registered with a different type`). v0.16 silently shared one
  column. Plugin names are reserved too: lobby `player`, `lobby`; physics2d `transform_2d`,
  `velocity_2d`, `physics_body_2d`, `physics_singleton_tag`, `active_contacts`; data
  `data_config_manifest`. A struct that embeds a component inherits its `Name()` and collides.
- Commands with the same `Name()` are not checked: the first type wins silently. Keep command
  names unique.
- Hooks are unchanged: Init once (and on debug reset), then PreUpdate, Update, PostUpdate
  each tick.

## Errors

| You see                                                                      | Fix                                   |
| ---------------------------------------------------------------------------- | ------------------------------------- |
| `undefined: cardinal.RegisterSystem` / `RegisterSystemV2` / `RegisterPlugin` | Method on `w`                         |
| `func(...) does not implement cardinal.System (missing method Run)`          | Convert the system (systems.md)       |
| `X does not implement cardinal.System (method Run has pointer receiver)`     | Register `&X{}`                       |
| `system *X is nil; register a constructed instance`                          | Pass `&X{}`, not a nil pointer        |
| `X does not satisfy ... (missing method AppendWire)`                         | Run `world sdk generate` (runtime.md) |
| `invalid world options: region cannot be empty` in a test                    | See below                             |

`NewWorld` validates the merged options and env, and stops at the first missing one. A test
must set `Region`, `Organization`, `Project`, `ShardID`, `SnapshotRate` (or
`CARDINAL_REGION`, `CARDINAL_ORG`, `CARDINAL_PROJECT`, `CARDINAL_SHARD_ID`,
`CARDINAL_SNAPSHOT_RATE`) and `TickRate > 0`, which has no env var.
