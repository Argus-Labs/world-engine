# Systems

A v0.16 system was a function plus a state struct whose fields were injected at
registration. A v0.17 system is any type with `Run(w *cardinal.World)`, and everything the
state struct held is a method on `w`, called at the point of use.

## Recipe for one system

1. Rename the function `XSystem(state *XSystemState)` to a method
   `func (s *XSystem) Run(w *cardinal.World)` on `type XSystem struct{}`.
2. For each field of `XSystemState`, record the type it names (for bootstrap.md), then
   delete the field and rewrite its uses with the table below.
3. Fields that are not Cardinal types (config, caches, clients, other-world addresses)
   become fields of `XSystem`, set where the system is constructed in `register`.
   Package globals that existed only to reach a system can move there too.
4. Replace `state.` with `w.` for the helpers in the table.
5. Delete `XSystemState`.

Before:

```go
type AttackPlayerSystemState struct {
	cardinal.BaseSystemState
	AttackPlayerCommands    cardinal.WithCommand[AttackPlayerCommand]
	PlayerDeathSystemEvents cardinal.WithSystemEventEmitter[systemevent.PlayerDeath]
	PlayerDeathEvents       cardinal.WithEvent[event.PlayerDeath]
	Players                 PlayerSearch
}

func AttackPlayerSystem(state *AttackPlayerSystemState) {
	for cmd := range state.AttackPlayerCommands.Iter() {
		for entity, player := range state.Players.Iter() {
			tag := player.Tag.Get()
			// ...
			state.Players.Destroy(entity)
			state.PlayerDeathEvents.SendTo(cmd.Persona, event.PlayerDeath{Nickname: tag.Nickname})
			state.PlayerDeathSystemEvents.Emit(systemevent.PlayerDeath{Nickname: tag.Nickname})
		}
	}
}
```

After:

```go
type AttackPlayerSystem struct{}

func (s *AttackPlayerSystem) Run(w *cardinal.World) {
	players := w.Exact[Player]()
	for cmd := range w.Commands[AttackPlayerCommand]() {
		attacker, ok := cmd.Sender.Player()
		if !ok {
			continue // Sent by another shard: no player to reply to.
		}
		for player := range players.Iter() {
			tag := player.Get[component.PlayerTag]()
			// ...
			player.Destroy()
			w.SendTo(attacker, event.PlayerDeath{Nickname: tag.Nickname})
			w.EmitSystemEvent(systemevent.PlayerDeath{Nickname: tag.Nickname})
		}
	}
}
```

## Mapping

| v0.16.7                                             | v0.17                                                         |
| --------------------------------------------------- | ------------------------------------------------------------- |
| embed `cardinal.BaseSystemState`                    | nothing; take `w *cardinal.World` in `Run`                    |
| `state.Logger()`                                    | `w.Logger()`                                                  |
| `state.Tick()`                                      | `w.TickHeight()`. Never `w.Tick(ts)`: that advances the world |
| `state.Timestamp()`                                 | `w.Timestamp()`                                               |
| `state.SendToShard(to, cmd)`                        | `w.SendToShard(to, cmd)`                                      |
| `F cardinal.WithCommand[T]` + `state.F.Iter()`      | `w.Commands[T]()` (`CommandContext{Payload, Sender}`)         |
| `F cardinal.WithEvent[T]` + `.Broadcast(e)`         | `w.Broadcast(e)`                                              |
| `F cardinal.WithEvent[T]` + `.SendTo(p, e)`         | `w.SendTo(p, e)`                                              |
| `F cardinal.WithSystemEventEmitter[T]` + `.Emit(e)` | `w.EmitSystemEvent(e)`                                        |
| `F cardinal.WithSystemEventReceiver[T]` + `.Iter()` | `w.SystemEvents[T]()`                                         |
| `F cardinal.Contains[...]` / `Exact[...]` field     | `w.Contains[A]()` / `w.Exact[A]()` (entities.md)              |
| `RegisterSystemV2` type with `Run()`                | add the `w *cardinal.World` parameter, drop the embed         |

## Helpers that took state

Helpers that took `*cardinal.BaseSystemState`, `cardinal.BaseSystemState` by value, or a
whole `*XState` now take `w *cardinal.World` and whatever else they read. Helpers that took
a search pointer take `cardinal.Search`, which is a small value; pass it by value. The old
rule that searches must be passed by pointer (a copied search had nil Refs) no longer
applies.

## Gotchas

- System names in traces and introspection are now `%T` of the instance (`*system.X`),
  not `func(*system.XState)`. Update dashboards or tests that match on names.
- A value-receiver `Run` is accepted but loses any state it mutates between ticks. Use
  pointer receivers.
- State kept on the system struct lives outside the ECS and outside snapshots, exactly like
  the package globals it replaces. Keep game state in components.
- There is still no world rand. Keep whatever the game did before.

## Errors

| You see                                                                                  | Fix                                                                     |
| ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `undefined: cardinal.BaseSystemState` / `WithCommand` / `WithEvent` / `WithSystemEvent*` | Apply this recipe                                                       |
| `state.Tick undefined`                                                                   | `w.TickHeight()`                                                        |
| `have Run() want Run(*cardinal.World)`                                                   | Add the parameter                                                       |
| `not enough arguments in call to w.Tick`                                                 | You meant `w.TickHeight()`                                              |
| `system.X (value of type func(...)) is not a type` in `register`                         | The system is still a func. Convert it before registering `&X{}`        |
| `state.Logger undefined` (or `Timestamp`, `SendToShard`)                                 | The embed is gone but the body is not converted: use `w.`               |
| panic `command X is not registered; call RegisterCommand before StartGame`               | bootstrap.md                                                            |
| panic `event *X is not registered`                                                       | You passed `&e`; pass the value, or register the type you actually send |
| panic `system event X is not registered`                                                 | `RegisterSystemEvent[X]`                                                |
