package ecs

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// -----------------------------------------------------------------------------
// Register Functions
// -----------------------------------------------------------------------------

// RegisterComponent registers a component type with the world and returns its ID.
// It should be called once per component type during initialization. It will panic
// if the component type is already registered.
//
// Performance: O(1)
//
// Example:
//
//	type Position struct{ X, Y float32 }
//	func (Position) Name() string { return "Position" }
//
//	world := ecs.NewWorld()
//	ecs.RegisterComponent[Position](world)
func RegisterComponent[T Component](w *World) {
	var zero T
	name := zero.Name()

	if err := w.components.register(name, newColumnConstructor[T]()); err != nil {
		panic(err)
	}
}

// RegisterSystem registers a system and its state with the world. It analyzes the system's
// state struct fields to determine component dependencies and adds the system to the scheduler for
// execution. By default, systems are registered to the Update hook. This can be overridden with the
// optional WithHook option.
//
// Performance: O(k) where k is the number of fields in the system state struct.
// The cost comes primarily from reflection operations to examine state fields.
//
// Example:
//
//	type RegenSystemState struct {
//		Players ecs.Exact[struct {
//			PlayerTag ecs.Ref[PlayerTag]
//			Health    ecs.Ref[Health]
//		}]
//	}
//
//	world := ecs.NewWorld()
//	ecs.RegisterSystem(world, func(state *RegenSystemState) error {
//	    // System logic here
//	    return nil
//	})
func RegisterSystem[T any](w *World, system System[T], opts ...SystemOption) {
	// Apply options to the default config.
	cfg := newSystemConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Initialize the fields in the system state.
	state := new(T)
	componentDeps, err := initializeSystemState(w, state, cfg.modifiers)
	if err != nil {
		panic(eris.Wrapf(err, "failed to register system %T", system))
	}

	name := fmt.Sprintf("%T", system)
	systemFn := func() error { return system(state) }

	switch cfg.hook {
	case Init:
		w.initSystems = append(w.initSystems, initSystem{name: name, fn: systemFn})
	case PreUpdate, Update, PostUpdate:
		w.scheduler[cfg.hook].register(name, componentDeps, systemFn)
	default:
		panic("invalid system hook")
	}
}

// RegisterCommand registers a command type with the world and returns its ID.
// It should be called once per command type during initialization. It will panic
// if the command type is already registered.
//
// Performance: O(1)
//
// Example:
//
//	type AttackPlayerCommand struct{ Target string; Damage int }
//	func (AttackPlayerCommand) Name() string { return "attack_player" }
//
//	world := ecs.NewWorld()
//	ecs.RegisterCommand[AttackPlayerCommand](world)
func RegisterCommand[T Command](w *World) CommandID {
	var zero T

	id, err := w.commands.register(zero.Name())
	if err != nil {
		panic(err)
	}

	return id
}

// -----------------------------------------------------------------------------
// Entity Operations
// -----------------------------------------------------------------------------

// Create creates an entity and adds it to the world with the given components.
// It returns the ID of the newly created entity.
//
// Performance: O(1) for component registration, O(log n) for archetype lookup
// where n is the number of unique component combinations.
//
// Example:
//
//	entity := ecs.Create(ws, Position{X: 0, Y: 0}, Velocity{X: 1, Y: 1})
func Create(ws *WorldState, components ...Component) EntityID {
	assert.That(len(components) > 0, "no components provided when creating entity")

	entity, err := ws.opNewEntity(components)
	if err != nil {
		panic(err)
	}

	return entity
}

// Destroy deletes an entity and all its components from the world.
// If the entity does not exist, it will be a no-op.
//
// Performance: O(1) for entity removal, O(k) for component cleanup
// where k is the number of components on the entity.
//
// Example:
//
//	entity := ecs.Create(ws, Position{X: 0, Y: 0})
//	ecs.Destroy(ws, entity)
func Destroy(ws *WorldState, entity EntityID) {
	// If the entity doesn't exist, no-op
	if !ws.entities.isAlive(entity) {
		return
	}

	err := ws.opRemoveEntity(entity)
	if err != nil && !eris.Is(err, ErrEntityNotFound) {
		panic(eris.Wrap(err, "failed to destroy entity"))
	}
}

// Set sets a component on an entity. If the entity doesn't have the component,
// it will be added. If it already has the component, it will be updated.
//
// Performance: O(1) for updates, O(k) for additions where k is the number
// of existing components on the entity.
//
// Example:
//
//	entity := ecs.Create(ws, Position{X: 0, Y: 0})
//	ecs.Set(ws, entity, Position{X: 10, Y: 20})
func Set[T Component](ws *WorldState, entity EntityID, component T) {
	currentArch, err := ws.entities.getArchetype(entity)
	if err != nil {
		panic(eris.Wrap(err, "failed to get entity archetype"))
	}

	setTypeID, _ := ws.world.components.getComponentID(component)

	// Fast path: entity already has the component
	if currentArch.hasComponent(setTypeID) {
		if err = opSetComponent(entity, currentArch, component); err != nil {
			panic(eris.Wrap(err, "failed to set component"))
		}
		return
	}

	// Create new component type bitmap
	newCompTypes := currentArch.componentBitmap()
	newCompTypes.Set(setTypeID)

	// Collect existing components
	newComps := currentArch.collectComponents(entity)
	newComps = append(newComps, component)

	if err = ws.opMoveEntity(entity, newComps); err != nil {
		panic(eris.Wrap(err, "failed to move entity to new archetype"))
	}
}

// Get retrieves a component from an entity.
// Returns the component and an error if the entity doesn't exist or doesn't have the component.
//
// Performance: O(1) for component lookup
//
// Example:
//
//	position, err := ecs.Get[Position](ws, entity)
//	if err != nil {
//		// Handle error
//	}
//	// Use position component
func Get[T Component](ws *WorldState, entity EntityID) (T, error) {
	var comp T

	arch, err := ws.entities.getArchetype(entity)
	if err != nil {
		if eris.Is(err, ErrEntityNotFound) {
			return comp, eris.Wrapf(err, "entity %d not found", entity)
		}
		return comp, eris.Wrap(err, "failed to get entity archetype")
	}

	col, err := getColumnFromArch[T](arch)
	if err != nil {
		panic(eris.Wrap(err, "failed to get column from archetype"))
	}

	comp, ok := col.get(entity)
	if !ok {
		panic(fmt.Errorf("component not found for entity %d", entity))
	}

	return comp, nil
}

// Has checks if an entity has a specific component type.
// Returns false if either the entity doesn't exist or doesn't have the component.
//
// Performance: O(1)
//
// Example:
//
//	if ecs.Has[Position](ws, entity) {
//	    pos, _ := ecs.Get[Position](ws, entity)
//	    // Use position component
//	}
func Has[T Component](ws *WorldState, entity EntityID) bool {
	arch, err := ws.entities.getArchetype(entity)
	if err != nil {
		if eris.Is(err, ErrEntityNotFound) {
			return false
		}
		panic(eris.Wrap(err, "failed to get entity archetype"))
	}

	var zero T
	id, _ := ws.world.components.getComponentID(zero)

	return arch.hasComponent(id)
}

// Remove removes a specific component from an entity.
// If the entity doesn't exist or doesn't have the component, it will be a no-op.
//
// Performance: O(1) for removal, O(k) for archetype transition
// where k is the number of remaining components.
//
// Example:
//
//	ecs.Remove[Velocity](ws, entity)
func Remove[T Component](ws *WorldState, entity EntityID) {
	var zero T

	currentArch, err := ws.entities.getArchetype(entity)
	if err != nil {
		// If the entity doesn't exist, no-op
		if eris.Is(err, ErrEntityNotFound) {
			return
		}
		panic(eris.Wrap(err, "failed to get entity archetype"))
	}

	removeTypeID, _ := ws.world.components.getComponentID(zero)

	// If the entity doesn't have the component, no-op
	if !currentArch.hasComponent(removeTypeID) {
		return
	}

	newCompTypes := currentArch.componentBitmap()
	newCompTypes.Remove(removeTypeID)

	// If no components left, destroy the entity
	if newCompTypes.Count() == 0 {
		if err = ws.opRemoveEntity(entity); err != nil {
			if eris.Is(err, ErrEntityNotFound) {
				return // If the entity doesn't exist, no-op
			}
			panic(eris.Wrap(err, "failed to destroy entity"))
		}
		return
	}

	// Collect remaining components (exclude the removed component)
	newComps := currentArch.collectComponents(entity, zero.Name())
	if err = ws.opMoveEntity(entity, newComps); err != nil {
		panic(eris.Wrap(err, "failed to move entity to new archetype"))
	}
}

// Alive checks if an entity exists in the world.
//
// Performance: O(1)
//
// Example:
//
//	if ecs.Alive(ws, entity) {
//		// Entity exists, safe to use
//	}
func Alive(ws *WorldState, entity EntityID) bool {
	return ws.entities.isAlive(entity)
}
