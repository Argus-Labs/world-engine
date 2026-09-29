package cardinal

import "github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"

// Entity is a runtime handle to an entity in one world. Store ID() in components,
// commands, and snapshots, then bind that ID with World.Entity when needed.
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
