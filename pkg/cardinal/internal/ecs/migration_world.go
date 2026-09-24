package ecs

import (
	"iter"
	"slices"

	"github.com/rotisserie/eris"
)

// Some changes cannot be made from inside one entity. Ranking every player needs every player's
// score; repointing a reference needs to know which entity the target became. A migration
// declaring All, Put or Create is a whole-world migration: it runs once, after every entity has
// been restored and every per-entity migration has run, and it sees the finished world.
//
// The ADR describes interleaving the two kinds in passes, so a whole-world migration could feed a
// per-entity one. This build does not do that: whole-world migrations run last, as a single pass,
// in the order Kahn's algorithm gives. Every case in the tree is reachable that way, and a
// per-entity migration consuming a whole-world output has no case behind it yet.

// All declares a whole-world input: every entity holding T, ordered by entity ID.
//
// The order is by ID rather than by storage layout so two shards restoring the same save see the
// same sequence. Anything derived from the order, such as a rank, is then the same on both.
type All[T Component] struct {
	entries []allEntry[T]
}

type allEntry[T Component] struct {
	id    EntityID
	value T
}

// Iter yields each entity holding T and its value.
func (a *All[T]) Iter() iter.Seq2[EntityID, T] {
	return func(yield func(EntityID, T) bool) {
		for _, entry := range a.entries {
			if !yield(entry.id, entry.value) {
				return
			}
		}
	}
}

// Len is how many entities hold T.
func (a *All[T]) Len() int { return len(a.entries) }

func (a *All[T]) worldInputName() string {
	var zero T
	return zero.Name()
}

func (a *All[T]) worldInputShape() uint64 {
	var zero T
	return shapeHash(zero)
}

// worldInputFill reads every entity holding T out of the restored world.
func (a *All[T]) worldInputFill(ws *worldState) error {
	a.entries = a.entries[:0]

	var zero T
	cid, err := ws.components.getID(zero.Name())
	if err != nil {
		return eris.Wrapf(err, "migration reads unregistered component %q", zero.Name())
	}

	for _, arch := range ws.archetypes {
		if arch == nil || !arch.components.Contains(cid) {
			continue
		}
		col, ok := arch.columns[arch.components.CountTo(cid)].(*column[T])
		if !ok {
			return eris.Errorf("component %q is not stored as %T", zero.Name(), zero)
		}
		for row, eid := range arch.entities {
			a.entries = append(a.entries, allEntry[T]{id: eid, value: col.get(row)})
		}
	}

	slices.SortFunc(a.entries, func(x, y allEntry[T]) int { return int(x.id) - int(y.id) })
	return nil
}

// Put declares a whole-world output: T written to any entity, not only the one being migrated.
//
// The entity keeps whatever else it holds, and gains T if it did not have one.
type Put[T Component] struct {
	ws  *worldState
	err error
}

// Set writes value to eid. The first failure is kept and reported once the migration returns, so a
// migration body does not have to check an error after every write.
func (p *Put[T]) Set(eid EntityID, value T) {
	if p.err != nil {
		return
	}
	p.err = p.ws.setComponent(eid, value)
}

func (p *Put[T]) worldOutputName() string {
	var zero T
	return zero.Name()
}

func (p *Put[T]) worldOutputShape() uint64 {
	var zero T
	return shapeHash(zero)
}

func (p *Put[T]) worldOutputBind(ws *worldState) { p.ws, p.err = ws, nil }

func (p *Put[T]) worldOutputErr() error { return p.err }

// Create declares a whole-world output that makes new entities holding T.
//
// New returns the entity's ID straight away, so a migration can record it on another component in
// the same pass. That is what case 3c needs: a reference has to name the entity that was created.
type Create[T Component] struct {
	ws  *worldState
	err error
}

// New creates an entity holding value and returns its ID. On failure it returns the invalid ID and
// keeps the error, which the engine reports once the migration returns.
func (c *Create[T]) New(value T) EntityID {
	if c.err != nil {
		return invalidEntityID
	}

	eid := c.ws.newEntity()
	if err := c.ws.setComponent(eid, value); err != nil {
		c.err = err
		return invalidEntityID
	}
	return eid
}

func (c *Create[T]) worldOutputName() string {
	var zero T
	return zero.Name()
}

func (c *Create[T]) worldOutputShape() uint64 {
	var zero T
	return shapeHash(zero)
}

func (c *Create[T]) worldOutputBind(ws *worldState) { c.ws, c.err = ws, nil }

func (c *Create[T]) worldOutputErr() error { return c.err }

// The engine reaches whole-world declarations through these. As with In and Out, the methods are
// unexported so a migration author can declare the fields but cannot supply their own.
type (
	worldInput interface {
		worldInputName() string
		worldInputShape() uint64
		worldInputFill(ws *worldState) error
	}
	worldOutput interface {
		worldOutputName() string
		worldOutputShape() uint64
		worldOutputBind(ws *worldState)
		worldOutputErr() error
	}
)

// runWorldMigrations runs every whole-world migration once, in dependency order, against the
// finished world.
func (ws *worldState) runWorldMigrations() error {
	order, err := ws.migrationOrder()
	if err != nil {
		return err
	}

	for _, m := range order {
		if !m.wholeWorld || !ws.worldMigrationApplies(m) {
			continue
		}
		if err := ws.runWorldMigration(m); err != nil {
			return err
		}
	}
	return nil
}

// worldMigrationApplies reports whether a whole-world migration has anything to do.
//
// Per-entity migrations are triggered by a stored shape, so an ordinary restart runs none of them.
// A whole-world migration has no stored shape of its own — it reads components that are already
// current — so without this it would run on every boot. For a ranking that is merely wasteful; for
// one that creates entities it would add them again every time the shard starts.
//
// The rule is that it runs only when something earlier in this restore produced one of the shapes
// it reads. That makes it part of the same conversion rather than a system that runs at boot.
func (ws *worldState) worldMigrationApplies(m *boundMigration) bool {
	for _, shape := range m.consumes() {
		if ws.restoreProduced[shape] {
			return true
		}
	}
	return false
}

func (ws *worldState) runWorldMigration(m *boundMigration) error {
	for _, input := range m.worldInputs {
		if err := input.worldInputFill(ws); err != nil {
			return eris.Wrapf(err, "migration %s failed to read %q", m.name, input.worldInputName())
		}
	}
	for _, output := range m.worldOutputs {
		output.worldOutputBind(ws)
	}

	m.run.Migrate()

	for _, output := range m.worldOutputs {
		if err := output.worldOutputErr(); err != nil {
			return eris.Wrapf(err, "migration %s failed to write %q", m.name, output.worldOutputName())
		}
		ws.markProduced(storedShape{name: output.worldOutputName(), shape: output.worldOutputShape()})
	}
	return nil
}
