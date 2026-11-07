# Plugins

Both: `cardinal.RegisterPlugin(world, p)` becomes `w.RegisterPlugin(p)`. Each plugin
registers its own components, commands and system events, so do not register them again
(harmless) and do not reuse their names (panic). One plugin instance per world: registering
the same instance twice panics.

Systems that need a plugin get it as a struct field, set in `register`. There are no
process-global plugin handles anymore.

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
