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

// Create creates an entity with the zero-valued components declared by T.
// It panics if any component in T was not registered with World.RegisterComponent.
func (b *BaseSystemState) Create[T any]() Entity {
	components, err := b.world.archetype[T]()
	if err != nil {
		panic(err)
	}
	return b.Entity(b.world.world.CreateWithArchetype(components))
}

// RegisterComponent registers a component type before world startup. Every component
// used by a system, archetype, or snapshot must be registered here; nothing registers
// components implicitly.
func (w *World) RegisterComponent[T ecs.Component]() {
	if _, err := w.world.RegisterComponent[T](); err != nil {
		panic(eris.Wrapf(err, "failed to register component %T", *new(T)))
	}
}

// WithComponent declares a component dependency on a type registered with
// World.RegisterComponent. Use it as a system field to check an optional component
// at startup, or as a field of a Contains/Exact archetype.
// Values are accessed through Entity, not through this declaration.
type WithComponent[T ecs.Component] struct{}

func (WithComponent[T]) lookup(w *ecs.World) (ecs.ComponentID, error) {
	return w.ComponentID[T]()
}

func (c *WithComponent[T]) init(meta *systemInitMetadata) error {
	_, err := c.lookup(meta.world.world)
	return err
}

type componentDeclaration interface {
	lookup(*ecs.World) (ecs.ComponentID, error)
}

// archetype resolves the registered component IDs declared by T's WithComponent fields
// and caches the result per type.
func (w *World) archetype[T any]() (bitmap.Bitmap, error) {
	typ := reflect.TypeFor[T]()
	if components, ok := w.entityArchetypes[typ]; ok {
		return components, nil
	}
	if typ.Kind() != reflect.Struct {
		return nil, eris.Errorf("entity archetype must be a struct, got %v", typ)
	}
	var components bitmap.Bitmap
	declarations := reflect.Zero(typ)
	for i := range typ.NumField() {
		// Read only field types on success. Type.Field also decodes names/tags and
		// constructs index metadata, which we need only for lookup errors.
		fieldType := declarations.Field(i).Type()
		declaration, ok := reflect.Zero(fieldType).Interface().(componentDeclaration)
		if !ok || fieldType.Kind() != reflect.Struct {
			return nil, eris.Errorf("field %s must be WithComponent[T], got %v", typ.Field(i).Name, fieldType)
		}
		id, err := declaration.lookup(w.world)
		if err != nil {
			return nil, eris.Wrapf(err, "component field %s of archetype %v is not registered", typ.Field(i).Name, typ)
		}
		components.Set(id)
	}
	if w.entityArchetypes == nil {
		w.entityArchetypes = make(map[reflect.Type]bitmap.Bitmap)
	}
	w.entityArchetypes[typ] = components
	return components, nil
}
