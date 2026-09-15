package ecs

import (
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// -------------------------------------------------------------------------------------------------
// Entity/Component Functions
// -------------------------------------------------------------------------------------------------

// Create creates an entity without any components.
func (w *World) Create() EntityID {
	return w.state.newEntity()
}

func (w *World) CreateWithArchetype(components bitmap.Bitmap) EntityID {
	return w.state.newEntityWithArchetype(components)
}

// Destroy deletes an entity and all its components from the world. Returns true if the entity is
// deleted, false otherwise.
func (w *World) Destroy(eid EntityID) bool {
	return w.state.removeEntity(eid)
}

// Alive checks if an entity exists in the world.
func (w *World) Alive(eid EntityID) bool {
	_, exists := w.state.entityArch.get(eid)
	return exists
}

// Set sets a component on an entity. If the entity contains the component type, it will update the
// value. If it doesn't, it will add the component.
func (w *World) Set[T Component](eid EntityID, component T) error {
	return w.state.setComponent(eid, component)
}

// Get gets a component from an entity.
// Returns an error if the entity doesn't exist or doesn't contain the component type.
func (w *World) Get[T Component](eid EntityID) (T, error) {
	return w.state.getComponent[T](eid)
}

// Remove removes a component from an entity.
// Returns an error if the entity or the component to remove doesn't exist.
func (w *World) Remove[T Component](eid EntityID) error {
	return w.state.removeComponent[T](eid)
}

// Has checks if an entity has a specific component type.
// Returns false if either the entity doesn't exist or doesn't have the component.
func (w *World) Has[T Component](eid EntityID) bool {
	_, err := w.Get[T](eid)
	if err == nil {
		return true
	}
	return eris.Is(err, ErrComponentNotFound)
}

// IterEntities iterates all entities that match the given component bitmap and match mode.
//
// We intentionally keep this as a callback-based iterator instead of returning iter.Seq because
// the additional closure/layer on hot query paths adds measurable allocations in cardinal
// benchmarks. This still resolves matching archetypes dynamically on every call.
func (w *World) IterEntities( //nolint:gocognit // it's fine
	components bitmap.Bitmap,
	match SearchMatch,
	yield func(EntityID) bool,
) error {
	switch match {
	case MatchExact:
		aid, exists := w.state.archExact(components)
		if !exists {
			return nil
		}

		arch := w.state.archetypes[aid]
		for _, eid := range arch.entities {
			if !yield(eid) {
				return nil
			}
		}
	case MatchContains:
		for _, arch := range w.state.archetypes {
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
		for _, arch := range w.state.archetypes {
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

func (w *World) MatchArchetype(eid EntityID, components bitmap.Bitmap, match SearchMatch) error {
	aid, exists := w.state.entityArch.get(eid)
	if !exists {
		return ErrEntityNotFound
	}

	arch := w.state.archetypes[aid]
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

// SearchMatch is the type of archetype match to use when selecting entities.
type SearchMatch string

const (
	// MatchExact matches entities that have exactly the specified components.
	MatchExact SearchMatch = "exact"
	// MatchContains matches entities that contain the specified components and may have others.
	MatchContains SearchMatch = "contains"
	// MatchAll matches all entities regardless of components.
	MatchAll SearchMatch = "all"
)

// -------------------------------------------------------------------------------------------------
// System Event Functions
// -------------------------------------------------------------------------------------------------

func (w *World) GetSystemEvents[T SystemEvent]() ([]T, error) {
	return getSystemEvent[T](&w.systemEvents)
}

func (w *World) EmitSystemEvent[T SystemEvent](systemEvent T) error {
	return enqueueSystemEvent(&w.systemEvents, systemEvent)
}
