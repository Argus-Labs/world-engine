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
| `NewWorld`, `StartGame`, `WithHook`, `WorldOptions` | unchanged                         | v0.18.0 drops `WorldOptions.Pprof`.                    |

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

v0.18.0 adds `cardinal.NewTestWorld`, a single-system harness. v0.17.x and v1.0.1 lack it,
and there `w.Tick` before `StartGame` panics with `Tick called before initialization`.
To check a pinned version, run inside the game module:

```sh
go doc github.com/argus-labs/world-engine/pkg/cardinal NewTestWorld
```

It exits 1 with `no symbol NewTestWorld` when the release lacks it. Ignore any
`k8s.io/... invalid version` lines before that.

Port each system test to it. Pass the shard's `register` as setup, or the package's
`registerTestTypes` below the shard:

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

The old harness still compiles and passes on v0.18.0, but it diverges from production:
reflection into `cardinal.World` runs the ECS `Init` directly, so registration never
closes and Init sees a zero `time.Time{}`. The audit lists every such test.

| Old harness                                                     | With `NewTestWorld`                                                                                             |
| --------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `cardinal.NewWorld` with `StorageTypeNop` and `t.Setenv`        | `cardinal.NewTestWorld(t, setup)`. It reads no env and connects to nothing.                                     |
| `w.RegisterSystem(...)` after `NewWorld`, before the first tick | Move it into setup. Register* after `NewTestWorld` returns panics: the world has started.                       |
| Reflection on the unexported `world` field to call `Init`       | Delete it. `NewTestWorld` runs the Init systems setup registers. Calling it again panics.                       |
| Reflection on the unexported `commands` field to call `Enqueue` | `w.Command(player, cmd)`.                                                                                       |
| Init-hook seed system that creates entities                     | `w.Create[A]()` and `e.Set(c)` in the test body.                                                                |
| Driver system that emits system events from a package variable  | `w.EmitSystemEvent(ev)` before the step. A driver that runs after the system under test stays a system (Rules). |
| Observer system writing to a package-level variable             | Read state after the step: `e.Get[C]()`, `w.Exact[A]().Iter()`.                                                 |
| Register the system under test, then `w.Tick(ts)`               | `w.RunSystem(&S{})` runs one system. `w.Tick()` runs the full schedule.                                         |
| Observer system collecting events or system events              | `w.Events[E]()`, `w.Emitted[SE]()`, `w.ShardCommands[C]()`                                                      |
| `LOG_LEVEL=disabled`                                            | Nothing. Logs go to `t.Log` at every level and show only on failure or under `-v`.                              |

Rules:

- Setup must register every type the system touches. `RunSystem` registers nothing, but
  the system itself need not be registered. Only `s` runs: no other system, no Init.
- `NewTestWorld` runs every Init system setup registered, plugin Init systems included,
  after setup returns. A `register` with spawners starts the test with their entities.
  Use a narrower setup when the test expects an empty world.
- `w.Command` fails the test for an unregistered command. It looks the command up by
  `Name()`. The command reaches the next step only, and any step drains it, even a
  `RunSystem` whose system does not read it.
- `Events` and `ShardCommands` return what the last step dispatched, in send order, as
  decoded copies. That includes sends from Init systems and from the test body between
  steps. Each item wraps the payload: `cardinal.Sent[E]{Recipient, Payload}`, with
  `Recipient` empty for a broadcast, and `cardinal.ShardCommand[C]{To, Payload}`.
- `Emitted` returns the system events the last step's systems emitted, as the values they
  passed: no encoding round trip, so a slice field still shares its array. System events
  the test or an Init system emitted are inputs: the next step's systems read them, but
  `Emitted` never includes them.
- A step clears system events when it ends. Code that runs inside a system in production
  must run inside a step in the test. Called between steps, it emits into the next step,
  and a receiver that ran earlier in the same tick in production now sees the event. That
  masks the bug a same-tick test pins. Wrap the system under test and the helper in a
  test-local system, and pass it to `RunSystem`. Its `Run` calls `(&S{}).Run(w)`, then the
  helper.
- The clock is the tick height: a step runs at `time.Unix(height, 0)`, one second per
  step. Init and the first step both run at `Unix(0)`. The old harness usually ticked at
  a constant `time.Unix(0, 0)`. Systems that read `w.Timestamp()` (cooldowns, timeouts,
  `IsZero` checks) now see time pass. Count steps in seconds when the test reasons about
  time.
- `w.World.Tick(ts)` runs the full schedule at `ts`, but it is not a step. `Events` and
  `ShardCommands` add its outputs to the last step's, and `Emitted` ignores it. It still
  increments the height, so a later `w.Tick()` can run at an earlier time than `ts`. Do
  not mix the two in one test.
- `w.Tick(ts)` does not compile on a `*TestWorld`: `too many arguments in call to w.Tick`.
  Helpers typed `*cardinal.World` take `w.World`.
- `w.StartGame()` fails the test.
- Plugins work: physics2d steps under `w.Tick()`, and the data plugin serves its catalog.
  A plugin instance registers on one world only. Construct it inside setup, and keep
  handles such as `*data.Plugin` in a variable that setup assigns.
- A `TestWorld` reads no env, so tests that only use it can call `t.Parallel()`, unless
  they share package-level game state. `RunDST` calls `t.Setenv` and cannot.

A system test that relied on package-level state, `time.Now` or map order can also run
its script through `cardinal.RequireDeterministic(t, setup, script)`. It runs setup and
the script on two fresh worlds, and fails at the first step where they differ in state,
events, shard commands or emitted system events, when the step counts differ, or when
they differ after the script. If it fails on a ported test, report it. Do not change the
system while migrating. Both runs share one process, so a pass can still hide map
iteration order over a few keys and coarse clock reads. Package-level game state carried
from the first run fails it at step 1. Read ported systems for `range` over a map and
`time.Now()` and report them. In code you write, range `slices.Sorted(maps.Keys(m))` and
use `w.Timestamp()` instead of `time.Now()`.

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
