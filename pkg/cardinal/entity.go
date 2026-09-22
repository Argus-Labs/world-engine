package cardinal

import (
	"reflect"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// Entity is a runtime handle to an entity in one world. Store ID() in components,
// commands, and snapshots, then bind that ID with BaseSystemState.Entity when needed.
// Like EntityID, a handle must not be retained after destruction or a world reset:
// numeric IDs can be reused. Entity methods must run on the world's system goroutine.
type Entity struct {
	world *ecs.World
	id    EntityID
}

// ID returns the numeric identifier used by storage, commands, and physics.
func (e Entity) ID() EntityID { return e.id }

// Alive reports whether the entity exists in its world. A zero handle is not alive.
func (e Entity) Alive() bool { return e.world != nil && e.world.Alive(e.id) }

// Get returns a copy of a component. It panics if the entity or component is absent.
// Use Has when the component is optional, and Set to write a modified copy back.
func (e Entity) Get[T ecs.Component]() T {
	component, err := e.world.Get[T](e.id)
	if err != nil {
		panic(err)
	}
	return component
}

// Set adds or replaces a registered component. It panics if the entity is absent
// or the component type was not registered before the world started.
func (e Entity) Set[T ecs.Component](component T) {
	if err := e.world.Set(e.id, component); err != nil {
		panic(err)
	}
}

// Has reports whether this entity has the requested component.
func (e Entity) Has[T ecs.Component]() bool {
	return e.world != nil && e.world.Has[T](e.id)
}

// Remove removes a registered component, doing nothing if it is already absent.
// It panics if the entity is absent or the component type is unregistered.
func (e Entity) Remove[T ecs.Component]() {
	if err := e.world.Remove[T](e.id); err != nil {
		panic(err)
	}
}

// Destroy removes the entity and all its components. It returns false if absent.
func (e Entity) Destroy() bool { return e.world != nil && e.world.Destroy(e.id) }

// Entity binds a numeric ID to this system's world. It does not require the entity
// to exist. Use Alive or Has to check before accessing an optional entity.
func (b *BaseSystemState) Entity(id EntityID) Entity {
	return Entity{world: b.world.world, id: id}
}

// Create creates an entity with the zero-valued components declared by T, an archetype
// struct whose fields are component types. It panics if any of them was not registered
// with World.RegisterComponent.
func (b *BaseSystemState) Create[T any]() Entity {
	return b.Exact[T]().Create()
}

// Contains returns a query over entities that have every component declared by T, allowing
// extras. T is a struct whose fields are component types (named or embedded), for example
//
//	type Mob struct {
//	    Health   Health
//	    Position Position
//	}
//
// Build the query inside the system function. It panics if a component in T was not
// registered with World.RegisterComponent before the world started.
func (b *BaseSystemState) Contains[T any]() Search {
	return b.query[T](ecs.MatchContains)
}

// Exact returns a query over entities that have exactly the components declared by T.
// See Contains for the shape of T.
func (b *BaseSystemState) Exact[T any]() Search {
	return b.query[T](ecs.MatchExact)
}

func (b *BaseSystemState) query[T any](match ecs.SearchMatch) Search {
	components, err := b.world.archetype[T]()
	if err != nil {
		panic(err)
	}
	return Search{world: b.world.world, components: components, match: match}
}

// RegisterComponent registers a component type before world startup. Every component
// used by a system, archetype, or snapshot must be registered here; nothing registers
// components implicitly.
func (w *World) RegisterComponent[T ecs.Component]() {
	if _, err := w.world.RegisterComponent[T](); err != nil {
		panic(eris.Wrapf(err, "failed to register component %T", *new(T)))
	}
}

// archetype resolves the registered component IDs declared by T's fields and caches the
// result per type. The first call for a type walks the struct with reflection; later calls
// are one map lookup.
func (w *World) archetype[T any]() (bitmap.Bitmap, error) {
	typ := reflect.TypeFor[T]()
	if components, ok := w.archetypes[typ]; ok {
		return components, nil
	}
	if typ.Kind() != reflect.Struct {
		return nil, eris.Errorf("entity archetype must be a struct, got %v", typ)
	}
	var components bitmap.Bitmap
	for i := range typ.NumField() {
		field := typ.Field(i)
		component, ok := reflect.Zero(field.Type).Interface().(ecs.Component)
		if !ok || field.Type.Kind() != reflect.Struct {
			return nil, eris.Errorf("field %s of archetype %v must be a component struct, got %v",
				field.Name, typ, field.Type)
		}
		id, err := w.world.ComponentIDOf(component)
		if err != nil {
			return nil, eris.Wrapf(err, "component field %s of archetype %v is not registered", field.Name, typ)
		}
		components.Set(id)
	}
	if w.archetypes == nil {
		w.archetypes = make(map[reflect.Type]bitmap.Bitmap)
	}
	w.archetypes[typ] = components
	return components, nil
}
