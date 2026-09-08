# Get, Set, Has, and Remove in gameplay systems

Prioritize checked generic methods on `BaseSystemState` for access by entity ID. Keep the existing typed references inside query loops. Generic methods are most useful when they remove a query-row lookup that the caller does not otherwise need.

The code in [gameplay_test.go.txt](gameplay_test.go.txt) is a compiled prototype, not a production API migration. It runs against the real Cardinal ECS from `d663f5b0` using Go 1.27.1.

## Proposed public methods

Every system already embeds `BaseSystemState`, which receives its World during system initialization. The methods therefore appear directly on the system state.

| Operation | Proposed call | Result and behavior |
| --- | --- | --- |
| Read | `state.Get[Health](id)` | `(Health, error)` |
| Update or add | `state.Set(id, Health{HP: 100})` | `error`; the value determines the component type |
| Check presence | `state.Has[Health](id)` | `bool`; false for absent entity or component |
| Remove | `state.Remove[Shield](id)` | `error`; absent registered component is a no-op |

All Get, Set, and Remove calls report errors for missing entities or unregistered component types. Set adds a registered component if the entity lacks it. It does not register new types during a tick. Has tests existence only and does not diagnose which part is missing.

The signature of Get is a generic method on the existing concrete owner:

```go
func (s *BaseSystemState) Get[T Component](id EntityID) (T, error)
func (s *BaseSystemState) Set[T Component](id EntityID, value T) error
func (s *BaseSystemState) Has[T Component](id EntityID) bool
func (s *BaseSystemState) Remove[T Component](id EntityID) error
```

`Component` in this public signature sketch means an exported alias of the existing ECS component constraint. The prototype uses the internal constraint directly because it is compiled inside package cardinal. The shared checkout has an unrelated unfinished helper migration that also expects this alias.

## What a call site gains

For a known target ID, today's public API first retrieves a typed query row:

```go
target, err := state.Players.GetByID(targetID)
if err != nil {
	return err
}
hp := target.Health.Get()
target.Health.Set(Health{HP: hp.HP - damage})
return nil
```

A helper that only needs Health can instead use:

```go
hp, err := state.Get[Health](targetID)
if err != nil {
	return err
}
return state.Set(targetID, Health{HP: hp.HP - damage})
```

Both snippets are the body of an error-returning gameplay helper. They are API sketches, not changes to a system's `Run()` signature.

The lobby's ready-status update uses this pattern with a real ID from its player index: `state.Players.GetByID(id)`, then `playerEntity.Player.Get()`, then `playerEntity.Player.Set(playerComp)`. See [lobby.go](../../../pkg/plugin/lobby/system/lobby.go). Its Players query declares only PlayerComponent, so a direct `state.Get[component.PlayerComponent](id)` checks the same required component without constructing a row.

This removes the dependency on a particular query-row shape and lets shared helpers receive the concrete system state. It does not establish that the target satisfies the old Players filter. If a target must also contain PlayerTag or match an Exact query, preserve that validation. The lookup benchmarks preserve the same query match for every variant so the faster variants do not win by accepting more entities.

## Where the current API remains better

Actual gameplay call sites are already concise. [Attack](../../../pkg/template/basic/shards/game/system/player_attack.go) uses `player.Health.Get()` and `player.Health.Set(...)`. [Regeneration](../../../pkg/template/basic/shards/game/system/regen.go) reduces this further to `health.Get()` and `health.Set(...)`. [Movement](../../../pkg/template/multi-shard/shards/game/system/player_move.go) works with several named component references.

```go
for _, player := range state.Players.Iter() {
	hp := player.Health.Get()
	player.Health.Set(Health{HP: hp.HP + 10})
}
```

Replacing those calls with `state.Get[component.Health](id)` repeats the entity ID and qualified type name. It also permits any component type, rather than exposing the row's named fields. Keep typed query rows for this common case unless a measured hot path justifies ID-only iteration.

The row's initial match establishes component membership. It is not a lifetime guarantee after component removal, entity destruction, or world reset. Existing Ref assertions also disappear under the `release` build tag. The proposed arbitrary-ID methods return errors in every build instead of borrowing that assertion contract.

## Entity handles are a larger alternative

```go
for entity := range state.Players.EntityIter() {
	hp, err := entity.Get[Health]()
	if err != nil {
		return err
	}
	if err := entity.Set(Health{HP: hp.HP + 10}); err != nil {
		return err
	}
}
```

A handle carries one World and EntityID. The probe checks destruction before another entity is created, so it does not establish safety after ID reuse or a world reset. It avoids repeating the ID and is convenient when passing the same entity to several helpers. It does not prove component membership, so Get and Set still return errors.

The prototype retains setup-time query declarations, but EntityIter yields handles directly and skips attaching each row field. The alternative ID iterator does the same for system methods. These iterator changes are independent of generic method syntax.

A complete replacement for Ref rows would also need an explicit setup-built filter API, such as `query.With[Health]().With[PlayerTag]()`. The filter would register components before ticks and own its cached matching bitmap. That filter API is only a design sketch here. Go does not infer or register the types used by runtime Get calls.

**Recommendation.** Start with direct system methods for known IDs. An Entity type is worth a separate query redesign if removing Ref declarations is the actual goal. Avoid introducing both as equivalent mandatory entry points. Keep world storage private, retain value reads and explicit writes, and do not cache component-column pointers across archetype moves.

## Performance comparison

The workload moves every entity by reading Position and Velocity, then writing Position. Each timed operation performs the work across the full entity set. Setup and final mutation-checksum validation are outside timing. The lookup workload applies the same Contains filter check in all three variants before accessing components.

Measured on the same Apple M5 Max with Go 1.27.1, `GOMAXPROCS=1`, and assertions enabled. The first run covered 100, 1,000, and 10,000 entities with ten 200 ms samples. Large timing variance prompted a second run of the 1,000-entity cases with ten one-second samples. API order reverses between samples. Other host activity and CPU placement were not controlled.

The longer run produced these medians for 1,000 entities per operation:

| Workload | Existing Ref rows | System methods | Entity methods |
| --- | ---: | ---: | ---: |
| Iterate and Get/Get/Set | 33.99 µs | 30.27 µs | 29.71 µs |
| Lookup and Get/Get/Set | 55.21 µs | 48.15 µs | 50.38 µs |

**Timing remains too variable for a reliable percentage speedup claim.** Benchstat reports baseline confidence ranges as large as ±214% for iteration and ±136% for lookup. Its rank tests flag method-versus-Ref differences, but the uncontrolled variation limits the estimate. Treat lower method medians as a reason for further controlled measurement, not a production performance guarantee.

Allocation counts match across all variants. Full query iteration allocates twice and uses 16 B per scan. The matching lookup workload allocates 1,000 times and uses 8,000 B per 1,000 lookups. The new method APIs do not eliminate the existing matching cost.

These comparisons include changed iteration and row construction, not just receiver syntax. ID and entity iteration avoid attaching each Ref field. The prior isolated component benchmark found no significant difference between a pre-bound Ref and an assertion-based generic entity Get/Set pair. Neither result justifies claiming that Go generic methods make ECS storage faster.

[Initial raw samples](results/gameplay/initial.txt), [initial comparison](results/gameplay/initial-comparison.txt), [longer-run samples](results/gameplay/confirmed.txt), and [longer-run comparison](results/gameplay/confirmed-comparison.txt) retain both runs without discarding outliers.

## Verification

The [passing semantic probe](results/gameplay/semantics.txt) exercises successful Get and Set, false Has for unregistered and removed components, registration requirements, repeated Remove, re-addition across archetype moves, query matching, destroyed entity errors, and iterator early break. It is run separately from timing.

The current internal `ecs.Has` returns true for an unregistered component error. The prototype intentionally computes `Get` success instead. Its absent and unregistered cases are exercised by the semantic probe. A production migration must correct or avoid the internal implementation, not forward its behavior.

The prototypes preserve ECS storage and component registration. They do not migrate production callers. Existing shared-checkout TestWorld failures and the previously reported lint findings remain outside this evaluation.

[run-gameplay.sh](run-gameplay.sh) creates an isolated source snapshot, compiles and checks the prototype, reverses API order on alternate samples, and produces the comparison. It rejects an existing result source directory. The default is ten 200 ms samples per case. `BENCHTIME=1s ENTITY_PATTERN=1000` repeats only the 1,000-entity case with longer samples.

Model the Domain keeps component ownership and registration in the existing World. Build the Lever makes the public API comparison rerunnable. Prove It Works requires mutation checksums and missing-component behavior, not just method compilation.
