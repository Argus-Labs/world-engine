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
| `map[K]V`              | `immutable.Slice[P]` of `struct{ K K; V V }`, found with `IndexFunc`  |
| optional `*T`          | `T` plus `HasT bool`                                                  |
| `*T` shared on purpose | an ID (`EntityID`, index or key) looked up where it is used           |
| `any` or an interface  | a kind enum plus one field per variant, or an encoded string          |

`immutable.Slice` hides its array but does not copy on write. Build with
`immutable.SliceOf(xs...)`. Read with `Len`, `At`, `All`, `Values`, `IndexFunc`. Every
derivation returns a Slice, but only `Append`, `Repeat`, `immutable.Map` and
`immutable.Concat` allocate. `With`, `Without`, `Filter`, `Delete`, `Reversed`,
`SortedFunc`, `CompactFunc`, `immutable.Sorted` and `immutable.Compact` edit the
receiver's array in place. `Insert` and `Replace` do when it has spare capacity, and
`Sub` and `Chunk` return windows onto it. `e.Get[C]()` returns a copy that shares the
column's array, so a derivation rewrites stored state before any `Set`. A discarded
`c.Items.Filter(keep)` leaves the stored slice at its old length over a shifted,
zero-filled tail. Always assign and set: `c.Items = c.Items.Filter(keep)`, then
`e.Set(c)`. For a read-only result, copy first:
`immutable.Collect(c.Items.Values()).SortedFunc(cmp)`. A `for i := range c.Items` over
the old slice becomes `for i, x := range c.Items.All()`.

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
