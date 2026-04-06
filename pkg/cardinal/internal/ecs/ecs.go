package ecs

import (
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
// deleted, false otherwise. Also cleans up any disk component files for the entity.
func Destroy(world *World, eid EntityID) bool {
	// The archetype's removeEntity handles both in-memory and disk columns.
	// Disk column entries are removed via the normal swap-remove path.
	return world.state.removeEntity(eid)
}

// Alive checks if an entity exists in the world.
func Alive(world *World, eid EntityID) bool {
	_, exists := world.state.entityArch.get(eid)
	return exists
}

// Set sets a component on an entity. Routes through the archetype column,
// which is either column[T] (in-memory) or diskColumn[T] (disk-backed).
func Set[T Component](world *World, eid EntityID, component T) error {
	return setComponent(world.state, eid, component)
}

// Get gets a component from an entity. Routes through the archetype column,
// which is either column[T] (in-memory) or diskColumn[T] (disk-backed).
func Get[T Component](world *World, eid EntityID) (T, error) {
	return getComponent[T](world.state, eid)
}

// Remove removes a component from an entity.
func Remove[T Component](world *World, eid EntityID) error {
	return removeComponent[T](world.state, eid)
}

// Has checks if an entity has a specific component type.
// Uses the archetype bitmap for O(1) lookup. No disk read for disk components.
// Returns false if either the entity doesn't exist or doesn't have the component.
func Has[T Component](world *World, eid EntityID) bool {
	var zero T
	cid, err := world.state.components.getID(zero.Name())
	if err != nil {
		return false
	}
	aid, exists := world.state.entityArch.get(eid)
	if !exists {
		return false
	}
	return world.state.archetypes[aid].components.Contains(cid)
}

// IterEntities iterates all entities that match the given component bitmap and match mode.
//
// We intentionally keep this as a callback-based iterator instead of returning iter.Seq because
// the additional closure/layer on hot query paths adds measurable allocations in cardinal
// benchmarks. This still resolves matching archetypes dynamically on every call.
func IterEntities( //nolint:gocognit // it's fine
	world *World,
	components bitmap.Bitmap,
	match SearchMatch,
	yield func(EntityID) bool,
) error {
	switch match {
	case MatchExact:
		aid, exists := world.state.archExact(components)
		if !exists {
			return nil
		}

		arch := world.state.archetypes[aid]
		for _, eid := range arch.entities {
			if !yield(eid) {
				return nil
			}
		}
	case MatchContains:
		for _, arch := range world.state.archetypes {
			if !arch.contains(components) {
				continue
			}

			for _, eid := range arch.entities {
				if !yield(eid) {
					return nil
				}
			}
		}
	case MatchAll:
		for _, arch := range world.state.archetypes {
			for _, eid := range arch.entities {
				if !yield(eid) {
					return nil
				}
			}
		}
	default:
		return eris.Wrapf(ErrInvalidMatch, "%v", match)
	}
	return nil
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
	return enqueueSystemEvent(&world.systemEvents, systemEvent)
}
