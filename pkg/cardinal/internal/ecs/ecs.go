package ecs

import (
	"iter"

	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// -------------------------------------------------------------------------------------------------
// Entity/Component Functions
// -------------------------------------------------------------------------------------------------

// Create creates an entity without any components.
func Create(world *World) EntityID {
	return world.state.newEntity()
}

func CreateWithArchetype(world *World, components bitmap.Bitmap) EntityID {
	return world.state.newEntityWithArchetype(components)
}

// Destroy deletes an entity and all its components from the world. Returns true if the entity is
// deleted, false otherwise.
func Destroy(world *World, eid EntityID) bool {
	return world.state.removeEntity(eid)
}

// Alive checks if an entity exists in the world.
func Alive(world *World, eid EntityID) bool {
	_, exists := world.state.entityArch.get(eid)
	return exists
}

// Set sets a component on an entity. If the entity contains the component type, it will update the
// value. If it doesn't, it will add the component.
func Set[T Component](world *World, eid EntityID, component T) error {
	return setComponent(world.state, eid, component)
}

// Get gets a component from an entity.
// Returns an error if the entity doesn't exist or doesn't contain the component type.
func Get[T Component](world *World, eid EntityID) (T, error) {
	return getComponent[T](world.state, eid)
}

// Remove removes a component from an entity.
// Returns an error if the entity or the component to remove doesn't exist.
func Remove[T Component](world *World, eid EntityID) error {
	return removeComponent[T](world.state, eid)
}

// Has checks if an entity has a specific component type.
// Returns false if either the entity doesn't exist or doesn't have the component.
func Has[T Component](world *World, eid EntityID) bool {
	_, err := Get[T](world, eid)
	if err == nil {
		return true
	}
	return eris.Is(err, ErrComponentNotFound)
}

func IterEntities(world *World, components bitmap.Bitmap, match SearchMatch) (iter.Seq[EntityID], error) {
	var archetypeIDs []archetypeID
	switch match {
	case MatchExact:
		if aid, exists := world.state.archExact(components); exists {
			archetypeIDs = []archetypeID{aid}
		}
		// If it doesn't exist, just leave empty so the iterator returns immediately
	case MatchContains:
		archetypeIDs = world.state.archContains(components)
	case MatchAll:
		archetypeIDs = world.state.archAll()
	default:
		return nil, eris.Wrapf(ErrInvalidMatch, "%v", match)
	}

	return func(yield func(EntityID) bool) {
		for _, id := range archetypeIDs {
			arch := world.state.archetypes[id]
			for _, eid := range arch.entities {
				if !yield(eid) {
					return
				}
			}
		}
	}, nil
}

func MatchArchetype(world *World, eid EntityID, components bitmap.Bitmap, match SearchMatch) error {
	aid, exists := world.state.entityArch.get(eid)
	if !exists {
		return ErrEntityNotFound
	}

	arch := world.state.archetypes[aid]
	switch match {
	case MatchExact:
		if !arch.exact(components) {
			return ErrArchetypeMismatch
		}
	case MatchContains:
		if !arch.contains(components) {
			return ErrArchetypeMismatch
		}
	case MatchAll:
		return nil
	default:
		return eris.Wrapf(ErrInvalidMatch, "%v", match)
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// System Event Functions
// -------------------------------------------------------------------------------------------------

func GetSystemEvents[T SystemEvent](world *World) ([]T, error) {
	return getSystemEvent[T](&world.systemEvents)
}

func EmitSystemEvent[T SystemEvent](world *World, systemEvent T) error {
	return enqueueSystemEvent[T](&world.systemEvents, systemEvent)
}
