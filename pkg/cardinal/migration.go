package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/rotisserie/eris"
)

// Snapshot migration converts stored component values whose shape no longer matches the code.
//
// A migration is a struct whose fields declare what it reads and writes, plus a Migrate method:
//
//	type RenamedV1ToCurrent struct {
//	    Old cardinal.In[component.RenamedV1]
//	    New cardinal.Out[component.Renamed]
//	}
//
//	func (m *RenamedV1ToCurrent) Migrate() { m.New.Set(component.Renamed{After: m.Old.Get().Before}) }
//
// What makes one run is the stored shape of a value, so a save written by the build reading it
// migrates nothing. Where the stored shape is two or more versions behind, the engine runs one
// migration per step: an output nothing else consumes lands on the entity, and an output another
// migration declares as its input is handed straight to it.
//
// A plugin declares migrations the same way a shard does, from its own Register method, and they
// go into the same registry. Which route runs is decided by the shapes a migration declares, never
// by who registered it or in what order, so a plugin and the shard using it can ship separately.
// Nothing here is plugin-only — a shard can chain its own component through several versions too,
// though in practice a plugin is far likelier to need it.

// Migration is a declared conversion. The engine calls Migrate once per entity whose stored values
// match the declared inputs.
type Migration = ecs.Migration

// In declares a required input: a value the entity must hold, in the shape T describes.
//
// T is usually a retired struct — the shape as an older build wrote it — kept beside the current
// one. It exists to decode old bytes and is never registered as a component.
type In[T ecs.Component] = ecs.In[T]

// Out declares an output: a value the migration produces.
//
// When another migration declares the same shape as its input, the value goes there instead of
// onto the entity, which is what lets a save several versions behind reach the current shape.
type Out[T ecs.Component] = ecs.Out[T]

// All declares a whole-world input: every entity holding T, ordered by entity ID.
//
// A migration declaring All, Put or Create sees the whole world instead of one entity, and runs
// once after every entity has been restored. It is for the changes no single entity can make:
// a rank is a position among all the others, and a reference names an entity that something else
// has to have created first.
type All[T ecs.Component] = ecs.All[T]

// Put declares a whole-world output: T written to any entity, not only the one being migrated.
type Put[T ecs.Component] = ecs.Put[T]

// Create declares a whole-world output that makes new entities holding T. It returns each new
// entity's ID straight away, so a migration can record it on another component in the same pass.
type Create[T ecs.Component] = ecs.Create[T]

// RegisterMigration registers a migration with the world. Must be called before StartGame().
// Panics if the migration cannot be registered, consistent with other registration functions.
func (w *World) RegisterMigration(migration Migration) {
	if err := w.world.RegisterMigration(migration); err != nil {
		panic(eris.Wrap(err, "failed to register migration"))
	}
}
