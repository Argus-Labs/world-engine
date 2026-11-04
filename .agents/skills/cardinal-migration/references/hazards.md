# Hazards: changes the compiler cannot catch

Work through every item after the build is clean. For each, either fix the hits or record
why none apply. Items marked (audit) are also reported by `scripts/audit.go`.

## Registration

1. (audit) Missing registration panics mid-game, not at boot. `w.Contains/Exact/Create`,
   `e.Set/Remove`, `w.Commands`, `w.Broadcast/SendTo`, `w.SystemEvents` and
   `w.EmitSystemEvent` panic on the first call with an unregistered type. The audit check
   is repo-wide: a type one shard registers hides the same type missing from another. In
   a monorepo, run the audit on each shard directory, then `world sdk generate --no-emit`
   and `RunDST` per shard. A per-shard run still reads `Name()` methods and archetype
   structs from the whole enclosing module. The audit only sees a use where a composite
   literal or type argument names the type (`w.Broadcast(E{})`, `e.Get[C]()`), not a
   value passed through a variable (`w.SendTo(p, ev)`) or a helper. It credits a
   plugin only when the `RegisterPlugin` argument traces to its constructor, and reports
   any other argument for you to confirm by hand.
2. `e.Has[C]()` returns false forever for an unregistered `C`, with no error.
3. (audit) Events are keyed by Go type. `RegisterEvent[E]()` followed by
   `w.Broadcast(&E{})` infers `*E` and panics. Send values.
4. Two Go types with one component or system-event `Name()` panic at registration. This
   includes game types that reuse a plugin's names and structs that embed a component.
5. Two command types with one `Name()` are not detected. The first registered wins. A
   command and an event may share a `Name()`: kinds have separate namespaces.
6. Every Register* panics once the world has started, including calls from Init systems.

## Entities

7. (audit) Passing a component instead of an archetype: `w.Contains[component.X]()`
   panics; for an empty tag it silently matches every entity, and `w.Create[Tag]()`
   creates an untagged entity.
8. (audit) `e.Set(&c)` compiles and panics with `unexpected column type`. The audit flags
   it only where the receiver traces to a World or Entity (a typed parameter or var,
   `w.Create`, `w.Entity`, `Single`, or a search `range`). Grep for `Set(&`,
   `Broadcast(&`, `SendTo(` and `EmitSystemEvent(&` to cover the rest.
9. `e.Get[C]()` panics when `C` is absent. Old search rows always had their components.
   Guard optional components with `Has`.
10. Get/Set/Remove on a destroyed entity panic in release builds too. v0.16 returned zero
    values silently under `-tags release`. Check `Alive()` where an entity may already be
    gone this tick. `Destroy` on a destroyed entity returns false.
11. Freed IDs are reused smallest-first. A stale stored ID can now resolve to a new entity,
    and `Alive()` on it returns true. Clear stored IDs on destroy.
12. `Limit(0)` yields nothing (it yielded one).
13. `for x := range q.Iter()` still compiles when `x` is only logged, but `x` is now an
    `Entity`, not an `EntityID`. Use `x.ID()` in logs and maps.

## Order and names

14. System registration order within a hook is execution order, and same-tick system-event
    delivery depends on it. Diff the new `register` against the recorded v0.16 order.
15. System names in traces, introspection and debug tooling are now `*pkg.XSystem`, not
    `func(*pkg.XSystemState)`.

## Data and serialization

16. Data plugin pointer kinds now run `Resolve`/`Validate` at boot.
17. `immutable.Slice` encodes to JSON as an array (was `{}`). Affects logs and anything that
    `json.Marshal`s components.
18. `immutable.Slice` derivations other than `Append` and `Repeat` edit the stored
    component's array in place, before any `Set`. A discarded `c.Items.Filter(keep)` or a
    read-only `c.Items.SortedFunc(cmp).At(0)` still changes the world. Assign the result
    and `e.Set(c)`. For a read-only result, copy first with
    `immutable.Collect(c.Items.Values())`. See older-versions.md, Wire shapes.
19. Snapshots: see runtime.md. Snapshots from v0.16.8 or earlier must be deleted.

## Operations

20. Tracing samples every tick by default. Set `OTEL_TRACE_SAMPLE_RATE` in production.
21. From v0.18.0, `CARDINAL_PPROF` is silently ignored and nothing listens on :6060. Run
    `rg CARDINAL_PPROF` in the game repo and the deploy repo, delete it, and drop any
    container port or scrape on 6060.

## Tests

22. (audit) A test that reflects into `cardinal.World`'s unexported `world` or `commands`
    field still compiles and passes. It calls the ECS `Init` directly, so registration
    never closes: a late Register* passes in the test and panics in production. Any
    internal rename breaks it at run time. Port it to `cardinal.NewTestWorld`
    (bootstrap.md, Testing one system).
23. (audit) `go build` skips test files, and CI often runs without the repo's build tags.
    For every build constraint the audit lists, run `go vet -tags <tags> ./...` and
    `go test -tags <tags> ./...` with tags that satisfy it (`integration && release` needs
    `-tags integration,release`). In one game, every `integration` test package of a shard
    had stopped compiling at v0.17, unnoticed until the next upgrade.
