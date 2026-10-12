# Entities, components and searches

## Archetypes

A search alias becomes a plain struct of component values. Keep the field names; they are
now only documentation. Name the struct after the entity (`PlayerSearch` becomes `Player`).

```go
// v0.16.7
type PlayerSearch = cardinal.Exact[struct {
	Tag    cardinal.Ref[component.PlayerTag]
	Health cardinal.Ref[component.Health]
}]

// v0.17
type Player struct {
	Tag    component.PlayerTag
	Health component.Health
}
// use: w.Exact[Player]()  or  w.Contains[Player]()
```

Anonymous and embedded forms work: `w.Contains[struct{ component.Health }]()`.
The type argument must be a wrapper struct. Never pass a component itself:
`w.Contains[component.Health]()` panics, and for an empty tag component
`w.Contains[PlayerTag]()` silently matches every entity and `w.Create[PlayerTag]()`
creates an entity without the tag.

## Mapping

| v0.16.7                                     | v0.17                                                                           |
| ------------------------------------------- | ------------------------------------------------------------------------------- |
| `for id, row := range s.Iter()`             | `for e := range q.Iter()`; `id := e.ID()`                                       |
| `for _, row := range s.Iter()`              | `for e := range q.Iter()`                                                       |
| `for id := range s.Iter()`                  | `for e := range q.Iter()`; the variable is now an `Entity`, so fix `%v` logging |
| `row.F.Get()`                               | `e.Get[C]()`                                                                    |
| `row.F.Set(v)`                              | `e.Set(v)`                                                                      |
| `row.F.Remove()`                            | `e.Remove[C]()`                                                                 |
| (none)                                      | `e.Has[C]()`, `e.Alive()`                                                       |
| `id, row := s.Create()` then `row.F.Set(v)` | `e := w.Create[A]()` then `e.Set(v)` (or `q.Create()`)                          |
| `s.Destroy(id)`                             | `e.Destroy()` or `w.Entity(id).Destroy()`                                       |
| `row, err := s.GetByID(id)`                 | `e, err := w.Exact[A]().GetByID(id)` (checks archetype)                         |
| stored `EntityID` -> row                    | `w.Entity(id)` (no checks) or `GetByID` (checked)                               |
| `id, row, err := it.Single()`               | `e, err := it.Single()`; same `ErrSingleNoResult` / `ErrSingleMultipleResult`   |
| `Filter(func(id EntityID, row T) bool)`     | `Filter(func(e Entity) bool)`                                                   |
| `Limit(n)`                                  | unchanged, except `Limit(0)` now yields nothing (it yielded one)                |
| helper param `cardinal.Ref[C]`              | `cardinal.Entity`                                                               |
| helper param `*PlayerSearch`                | `cardinal.Search` by value                                                      |
| search embedded in the state struct         | `q := w.Contains[A]()` at the top of `Run`; `state.Iter()` becomes `q.Iter()`   |
| search that may be unset (`s == nil`)       | `*cardinal.Search`, set with `new(w.Contains[A]())`. `Search` is not comparable |

## Behavior that moved

- `Set` and `Remove` reach any registered component on the entity, not only the ones the
  search declared. Adding a component to an entity no longer needs a second search.
- `Get` panics if the component is absent. Old rows always had their declared components.
  For optional components, check `e.Has[C]()` first.
- Get/Set/Remove on a destroyed entity panic in every build. v0.16 guarded them with
  `assert.That`, which is a no-op under `-tags release`, so release builds silently returned
  zero values. Code that relied on that now crashes: check `e.Alive()` where an entity may
  have been destroyed earlier in the same tick.
- Freed IDs are reused smallest-first (v0.16 reused oldest-first). A stale ID kept in a
  component or a package map can now point at a new entity sooner, and
  `w.Entity(staleID).Alive()` returns true for the new occupant. Clear IDs on destroy, or
  verify identity with a component value, not with `Alive`.

## Storing references

Store `e.ID()` (a `cardinal.EntityID`) in components, commands, maps and system fields.
Rebind with `w.Entity(id)` or `GetByID` when you need it. Never keep an `Entity` across
destruction or a world reset.

## Errors

| You see                                                                                  | Fix                                              |
| ---------------------------------------------------------------------------------------- | ------------------------------------------------ |
| `undefined: cardinal.Ref` / `cardinal.Contains` / `cardinal.Exact`                       | Archetype struct + `w.Contains`/`w.Exact`        |
| `range over ... permits only one iteration variable`                                     | `for e := range q.Iter()`                        |
| `assignment mismatch: 3 variables but ... Single returns 2 values`                       | `e, err := ...Single()`                          |
| panic `cannot resolve component field X of archetype A: ... component is not registered` | `RegisterComponent`                              |
| panic `field X of archetype A must be a component struct`                                | You passed a component as the archetype; wrap it |
| panic `unexpected column type` on `Set`                                                  | You passed a pointer; pass the value             |
| `X does not satisfy ecs.Component (missing method AppendWire)`                           | Run `world sdk generate` (runtime.md)            |
