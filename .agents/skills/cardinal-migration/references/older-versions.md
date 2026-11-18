# Starting points other than v0.16.7

`scripts/audit.go` prints the pinned world-engine version from every go.mod it finds.

## From v0.16.4

Everything below is in addition to the main workflow.

- Generated `MarshalWire` returned `([]byte, error)` at v0.16.4. World CLI replaces
  the generated code. Hand-written callers change: `b, err := v.MarshalWire()` becomes
  `b := v.AppendWire(nil)`. `T{}.UnmarshalWire(b)` returns `(any, error)`.
- Wire shapes: see the next section. This changes the proto schema, so client SDKs must be
  regenerated and shipped with the shard.
- Generated proto packages moved from `world_engine.*` to
  `github_com.argus_labs.world_engine.*`. Fully-qualified message names in introspection
  and client code change.
- Lobby: `LobbyComponent` was reshaped (fixed arrays with counts, `PassthroughData` is a
  string, `Team.PlayerIDs` and `ConfigComponent`/`IndexComponent` removed). Lobby client
  payloads change shape.
- physics2d moved to a native Go Box2D port (no CGO, float64). Package functions
  `Raycast`, `OverlapAABB`, `CircleSweep`, `ResetRuntime`, `WorldID`,
  `FlushBufferedContacts` and `SetStepContactEmitter` are gone. Use `*Plugin` methods
  (`p.Raycast`, `p.OverlapAABB`, `p.CircleSweep`, `p.Reset`, `p.Engine`, `p.BodyID`,
  `p.ShapeIDs`). Give querying systems a `Physics *physics2d.Plugin` field. Recorded
  physics results differ.
- `RegisterSystemV2`, `cardinal.Slice` and fixed-array introspection are additions.

### Wire shapes

World CLI refuses `[]T`, `map`, pointer and interface fields in every component, command,
event and system event, server-only components included. Size the work with `--no-emit`
before editing (SKILL.md, Before editing). Convert each field, then fix its uses:

| Field                  | Becomes                                                               |
| ---------------------- | --------------------------------------------------------------------- |
| `[]T`                  | `immutable.Slice[T]`, or `[N]T` plus a count when N is small and hard |
| `map[K]V`              | `immutable.Map[K, V]` with integer or string keys; see below          |
| optional `*T`          | `T` plus `HasT bool`                                                  |
| `*T` shared on purpose | an ID (`EntityID`, index or key) looked up where it is used           |
| `any` or an interface  | a kind enum plus one field per variant, or an encoded string          |

`immutable.Slice` hides its storage and is edited like a Go slice, but never writes to
storage it shares, so an edit reaches the world only through `Set` and `d := c.Items` is a
free, real copy. Build with
`immutable.SliceOf(xs...)`. Read with `Len`, `At`, `All`, `Values`, `IndexFunc`. Edit with
`Set`, `Append`, `Insert`, `Delete`, `DeleteFunc`, `Reset`, `SortStableFunc`, `Reverse`,
then `e.Set(c)`. Old derivations map as: `c.Items = c.Items.With(i, x)` becomes
`c.Items.Set(i, x)`, `Filter(keep)` becomes `DeleteFunc` with the test inverted, `Reversed`
becomes `Reverse`, `SortedFunc` becomes `SortStableFunc`, and `Sub(i, j)` becomes
`Delete(j, Len())` then `Delete(0, i)`. The `immutable.Map` function, `Concat`, `Chunk`, `Repeat`, `Replace`,
`Compact` and `Min`/`Max` are gone; use a loop or `slices` on `slices.Collect(s.Values())`.
`immutable.Map` now names the Map type, so a leftover `immutable.Map(s, f)` call fails with
"cannot use generic type immutable.Map without instantiation" rather than "undefined".
A `for i := range c.Items` over the old slice becomes `for i, x := range c.Items.All()`.

`immutable.Map[K, V]` is the same for a lookup table, edited like a Go map and safe to
copy the same way: build with `immutable.MapFrom(maps.All(goMap))`, read with `Get`,
`Has`, `All`, edit with `Set`, `Delete`, `DeleteFunc`, `Clear`, then `e.Set(c)`. A Go map
maps as: `v, ok := m[k]` becomes `m.Get(k)`, `m[k] = v` becomes `m.Set(k, v)`, `delete(m, k)`
becomes `m.Delete(k)`, `clear(m)` becomes `m.Clear()`, and `for k, v := range m` becomes
`for k, v := range m.All()`, in key order. Setting and deleting keys inside that loop is safe; the
loop reads the Map as it was when it started. Keys are integers, strings or named types of them, kept sorted. `EntityID`s are reused as soon as the entity is destroyed: key
long-lived data by an ID the game owns, or remove the key in the system that destroys the
entity.

Which of these needs a design decision is the user's call: a shared pointer or an
interface field usually does. List them and ask before converting.

## From v0.16.8 or v0.16.9 (interim API)

These releases already had some World methods. Map them too:

| Interim                                                              | v0.17                                           |
| -------------------------------------------------------------------- | ----------------------------------------------- |
| `w.RegisterSystem(fn)` with a function                               | `w.RegisterSystem(&S{})` with a `Run(w)` type   |
| `w.RegisterSystemV2(&S{})`                                           | `w.RegisterSystem(&S{})`                        |
| `w.RegisterArchetype[A]()` (v0.16.9)                                 | one `w.RegisterComponent[C]()` per field of `A` |
| `cardinal.WithComponent[C]` field (v0.16.9)                          | `RegisterComponent[C]` + `e.Get[C]()`           |
| `state.Create()` / `state.Entity(id)` on `BaseSystemState` (v0.16.9) | `w.Create[A]()` / `w.Entity(id)`                |

v0.16.9 search aliases already yield `cardinal.Entity`, so loop bodies
(`e.Get/Set/ID/Destroy`) need no change. Only the declarations move:

| v0.16.9                                                          | v0.17                                   |
| ---------------------------------------------------------------- | --------------------------------------- |
| `type S = cardinal.Exact[struct{ F cardinal.WithComponent[C] }]` | `type A struct{ F C }` + `w.Exact[A]()` |
| `state.S.Create()` / `state.S.Iter()`                            | `w.Exact[A]().Create()` / `.Iter()`     |

Snapshots written by v0.16.9 load in v0.17, and v0.17 snapshots load in v0.16.9. No wipe
is needed from v0.16.9. Snapshots written by v0.16.8 or earlier do not load.
