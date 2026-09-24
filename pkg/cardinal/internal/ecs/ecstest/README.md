# ecstest

Test fixtures for the ECS package. Nothing here is engine code, and nothing outside `internal/ecs`
can import it.

## earlyaccess and fullrelease

Snapshot migration is about one thing: a save written by yesterday's build being loaded by today's.
Testing that needs both builds alive at once, and Go types are fixed at compile time — a struct
named `Renamed` holding `Before` and a struct named `Renamed` holding `After` are two types, and two
types cannot share a name in one package.

So there are two packages:

- **`earlyaccess`** — the component shapes a stored snapshot holds. Think of them as the game as it
  shipped in early access. Once a shape is here it does not change, because changing it would change
  what "the old save" means.
- **`fullrelease`** — the shapes the current code declares, the retired shapes it knows older builds
  wrote, and the migrations that convert between them. This is the side under test.

Both declare the same component *names*. A snapshot records a component by name and nothing else, so
bytes written from early access are indistinguishable from bytes any other build wrote for
`renamed`. That is what makes the fixture honest rather than a simulation.

The two packages share no Go types. `fullrelease` declares its own copy of each retired shape rather
than importing the early-access struct, which matters: sharing one type would make the two shape
hashes equal by construction, and the fixture could never catch a hand-written retired struct that
got a field wrong — the likeliest mistake there is.

## Layout

Each build is laid out the way a shard is, so the shape of it is familiar:

```
wire/                          shared encoding helpers
earlyaccess/component/         one file per component, named after its case
fullrelease/component/         same names, current shapes plus the retired ones
fullrelease/migration/         one file per migration
plugin/earlyaccess/component/  provided.go
plugin/fullrelease/component/  provided.go, current + retired shapes
```

Every case in the tree from ADR-065 is here, one component per case: `added`, `removed`,
`renamed`, `reordered`, `retyped` for the one-to-one changes; `split`, `merge_left`/`merge_right`,
`absorber`/`absorbed`, `moved_from`/`moved_to`, `dropped` for the ones that change an entity's
component set; and `ranked`, `spawner`/`minion`, `pointer`/`slotted` for the ones that cross
entities. `chained` covers ordering rather than a case, and `unchanged` is the control.

One file per component, named after it; retired shapes sit in the file of the component they are a
past version of. Migrations get their own package with one file each, named after what the migration
does.

There is no `gen/` directory because nothing here is generated: a shard's `gen/` holds protobuf
message types, and these components have none.

Neither build is a shard, though. There are no systems, no commands, no ticking, and no storage. A
test builds a world from one build's components, encodes it, and decodes it into a world built from
the other's.

## Reading it

Each component carries a comment naming the migration case it exercises, keyed to the case tree in
ADR-065 — `1c rename a field`, `2b merge many into a new component`, and so on. The components are
named after their case rather than after anything in a game, so a test asserting on `renamed` says
what it is testing without a theme to decode first.

The full table of which test covers which case is at the top of `migration_test.go`, one directory
up, so the claim and the tests sit in the same file.

Three of the migrations here are whole-world rather than per-entity: they declare `All`, `Put` or
`Create` instead of `In` and `Out`, run once after every entity is restored, and exist for the
changes no single entity can make. `rank_all.go`, `spawn_minions.go` and `resolve_pointers.go`.

## plugin

`plugin/` is one component owned by a separately released package, and it is deliberately small. It
is not where migration cases live — those all sit in the two packages above. Its whole job is to
show that a plugin declares a migration through the same `cardinal.RegisterMigration` a shard uses,
and that the result lands in the same registry.

Chaining is not a plugin feature. A shard can put its own component through as many versions, and
`chained` in the shard fixture does exactly that. A plugin is simply where it happens in practice,
because a plugin author has no say in how old their users' saves are.

The test for it lives in `pkg/cardinal`, since that is where `RegisterPlugin` is.

## Hand-written wire methods

`SizeWire`, `AppendWire` and `UnmarshalWire` are what `world sdk generate` writes into a component
package as `wire.gen.go`. The fixture writes them by hand, for two reasons: the generator only
emits for components wired to a system, and a fixture has no systems; and it emits nothing at all
for retired shapes, which is exactly what a migration has to decode.

The three encoding helpers they share live in `wire/` rather than being copied into each component
package, since what the generator emits per package is the methods, not the helpers.
