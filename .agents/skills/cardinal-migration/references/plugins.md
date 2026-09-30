# Plugins

All three: `cardinal.RegisterPlugin(world, p)` becomes `w.RegisterPlugin(p)`. Each plugin
registers its own components, commands and system events, so do not register them again
(harmless) and do not reuse their names (panic). One plugin instance per world: registering
the same instance twice panics.

Systems that need a plugin get it as a struct field, set in `register`. There are no
process-global plugin handles anymore.

## data

```go
// v0.16.7
data.Register[component.Abilities](dataPlugin)
cardinal.RegisterPlugin(world, dataPlugin)
// in any system:
abilities := data.Get[component.Abilities]()

// v0.17
data.Register[component.Abilities](dataPlugin) // still before RegisterPlugin
w.RegisterPlugin(dataPlugin)
w.RegisterSystem(&AbilitySystem{Data: dataPlugin})

type AbilitySystem struct{ Data *data.Plugin }

func (s *AbilitySystem) Run(w *cardinal.World) {
	abilities := data.Get[component.Abilities](s.Data)
}
```

- Find every `data.Get[` call (the audit lists them) and give its system a
  `Data *data.Plugin` field. Helpers that call `data.Get` take the plugin as a parameter.
- Threading reaches every function between a system's `Run` and the call, including
  interface methods and package-level function variables. Build the call graph first. In
  a large game this is a codemod: one game threaded 179 functions for data and 91 for
  physics2d.
- Panics: `data: register kinds before calling w.RegisterPlugin(dataPlugin)`,
  `data: Get called with a nil *Plugin; inject the world's data plugin into the system`,
  `data: Plugin instance is already registered; create a separate plugin for each world`.
- Pointer kinds (`data.Register[*T]`) now run `Resolve` and `Validate`. They were skipped
  before. A `Validate` that never ran can now fail at boot: boot once locally.
- `ConfigManifest.Files` is an `immutable.Slice`, not a map. Only matters if you read the
  manifest component directly. Use `.Hash(path)`.

## lobby

- Registration is `w.RegisterPlugin(lobby.NewPlugin(cfg))`. `lobby.Config` is unchanged.
- `lobby/system` no longer exports `SetConfig`, `SetProvider`, the function systems or their
  state types. Do not construct `&system.LobbySystem{}` yourself: it panics on the first
  `Run` with "has no runtime; register lobby.NewPlugin instead".
- A game shard that receives session starts must call
  `w.RegisterCommand[lobby.NotifySessionStartCommand]()`. Without it `w.Commands` panics
  and the lobby's inter-shard notify is rejected as `unregistered command`. Sending
  `NotifySessionEndCommand` back with `w.SendToShard` needs no registration. The audit
  skips plugin types when any world registers that plugin, so check this one by hand.
- Game code that read `lobby.Component` through a `Ref` uses
  `w.Contains[struct{ Lobby lobby.Component }]()` and `e.Get[lobby.Component]()`.

## physics2d

- Registration is `w.RegisterPlugin(physics2d.NewPlugin(cfg))`.
- Queries are `*Plugin` methods: `p.Raycast`, `p.OverlapAABB`, `p.CircleSweep`, `p.Reset`,
  `p.Engine`, `p.BodyID`, `p.ShapeIDs`. Keep the plugin in a variable and give each
  querying system a `Physics *physics2d.Plugin` field.
- Contact events: `WithSystemEventReceiver[physics2d.ContactBeginEvent]` becomes
  `w.SystemEvents[physics2d.ContactBeginEvent]()`.
- Order is unchanged: an Init spawner that the first physics rebuild must see is registered
  before `RegisterPlugin`.
- Code that creates physics entities outside systems before `RegisterPlugin` (test harnesses)
  calls `physics2d.RegisterComponents(w)` first.
- Component shapes (from v0.16.8): `PhysicsBody2D.Shapes`, `ColliderShape.ChainPoints` and
  `ActiveContacts.Pairs` are `immutable.Slice[T]` (build with `immutable.SliceOf(...)`).
  `ColliderShape.Vertices` is `[8]Vec2` plus `VertexCount`. Build polygons with
  `.WithVertices(...)`. More than 8 vertices fails `Validate`.
- Starting from v0.16.4: the package functions `physics2d.Raycast` and friends are gone.
  See older-versions.md.
