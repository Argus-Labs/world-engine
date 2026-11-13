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
	aid, exists := w.state.entityArch.get(eid)
	if !exists {
		return false
	}
	var zero T
	cid, registered := w.state.components.catalog[zero.Name()]
	if !registered {
		return false
	}
	arch := w.state.archetypes[aid]
	if !arch.components.Contains(cid) {
		return false
	}
	// Check the concrete type without copying the component or constructing an absence error.
	_, matches := arch.columns[arch.components.CountTo(cid)].(*column[T])
	return matches
}

// IterEntities iterates all entities that match the given component bitmap and match mode.
//
// It visits the entities that match when it is called, each at most once, so yield may add or
// remove components and create or destroy entities. It skips an entity that an earlier yield
// destroyed or changed so it no longer matches. Entities that start matching during the iteration
// are not visited, unless one reuses the ID of a matching entity destroyed before it was reached.
//
// We intentionally keep this as a callback-based iterator instead of returning [iter.Seq] because
// the additional closure/layer on hot query paths adds measurable allocations in cardinal
// benchmarks. This still resolves matching archetypes dynamically on every call.
func (w *World) IterEntities(
	components bitmap.Bitmap,
	match SearchMatch,
	yield func(EntityID) bool,
) error {
	// Copy the matching IDs before yielding. Moving or destroying an entity swap-removes it from its
	// archetype's entity slice and appends it to another, so walking the live slices would skip
	// entities or visit them twice.
	exactID := -1
	switch match {
	case MatchExact:
		aid, exists := w.state.archExact(components)
		if !exists {
			return nil
		}
		exactID = aid
	case MatchContains, MatchAll:
	default:
		return eris.Wrapf(ErrInvalidMatch, "%v", match)
	}

	snapshot, ok := w.searchSnapshots.Get().(*searchSnapshot)
	if !ok {
		snapshot = new(searchSnapshot)
	}
	ids, segments := snapshot.ids[:0], snapshot.segments[:0]
	for aid, arch := range w.state.archetypes {
		if aid == exactID || match == MatchAll || (match == MatchContains && arch.contains(components)) {
			ids = append(ids, arch.entities...)
			segments = append(segments, searchSegment{arch: arch, version: arch.version, end: len(ids)})
		}
	}

	start := 0
search:
	for _, segment := range segments {
		for _, eid := range ids[start:segment.end] {
			// Until an entity joins or leaves the archetype, its copied IDs are still its members.
			if segment.arch.version != segment.version && w.MatchArchetype(eid, components, match) != nil {
				continue
			}
			if !yield(eid) {
				break search
			}
		}
		start = segment.end
	}

	clear(segments) // Don't let the pool keep archetypes alive.
	snapshot.ids, snapshot.segments = ids, segments
	w.searchSnapshots.Put(snapshot)
	return nil
}

// searchSnapshot is the IterEntities copy of the matching entity IDs, grouped by archetype.
type searchSnapshot struct {
	ids      []EntityID
	segments []searchSegment
}

// searchSegment marks the end of one archetype's IDs in a searchSnapshot and its version when copied.
type searchSegment struct {
	arch    *archetype
	version uint64
	end     int
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
	var zero T
	return w.systemEvents.get[T](zero.Name())
}

func (w *World) EmitSystemEvent[T SystemEvent](systemEvent T) error {
	return w.systemEvents.enqueue(systemEvent)
}
