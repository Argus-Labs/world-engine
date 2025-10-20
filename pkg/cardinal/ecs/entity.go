package ecs

import (
	"math"
	"sync"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// EntityID is a unique identifier for an entity.
type EntityID uint32

// MaxEntityID is the maximum entity ID that can be created.
const MaxEntityID = math.MaxUint32 - 1

// entityManager manages entity IDs and references to their associated archetypes. This struct acts
// as an index/mapping from entity ID to its archetype to avoid iterating through all archetypes.
// All methods that accept a pointer to an archetype expects a non-nil pointer. archetype is an
// internal type, and if we were to find a nil pointer it means we have made a mistake somewhere.
type entityManager struct {
	nextID     EntityID                // The next ID to allocate if no free IDs are available
	free       []EntityID              // A queue of free IDs
	entityArch map[EntityID]*archetype // Maps entity IDs to archetypes
	mu         sync.Mutex              // Mutex for thread-safe operations
}

// newEntityManager creates a new entity manager with initial capacity.
func newEntityManager() entityManager {
	return entityManager{
		nextID:     0,
		free:       make([]EntityID, 0),
		entityArch: make(map[EntityID]*archetype),
		mu:         sync.Mutex{},
	}
}

// new returns a new entity ID.
func (em *entityManager) new(arch *archetype, comps []Component) (EntityID, error) {
	assert.That(arch != nil, "archetype must not be nil")

	em.mu.Lock()
	defer em.mu.Unlock()

	var id EntityID
	if len(em.free) > 0 {
		// Pop from the front of the free list (FIFO).
		id = em.free[0]
		// Remove the element at index 0 efficiently:
		//   - Overwrite index 0 with the last element, or
		//   - Copy from [1:] to [0:], etc.
		// For a true FIFO in a slice, we can do:
		em.free = em.free[1:]
	} else {
		// No free IDs, use the next sequential ID.
		id = em.nextID
		if id > MaxEntityID {
			return 0, eris.New("max number of entities exceeded")
		}
		em.nextID++
	}

	// Create the entity in the archetype.
	err := arch.newEntity(id, comps)
	if err != nil {
		return 0, eris.Wrap(err, "failed to create entity")
	}

	// Map the entity ID to the archetype.
	em.entityArch[id] = arch

	return id, nil
}

// remove marks an entity ID as available for reuse.
func (em *entityManager) remove(id EntityID) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	if !em.isAlive(id) {
		return ErrEntityNotFound
	}

	// Remove entity from its archetype.
	arch := em.entityArch[id]
	arch.removeEntity(id)

	// Add to free list.
	em.free = append(em.free, id)

	// Remove archetype mapping.
	delete(em.entityArch, id)

	return nil
}

// move moves an entity from one archetype to another.
func (em *entityManager) move(entity EntityID, newArch *archetype, newComps []Component) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	if !em.isAlive(entity) {
		return ErrEntityNotFound
	}

	// Set the new archetype.
	if err := newArch.newEntity(entity, newComps); err != nil {
		return eris.Wrap(err, "failed to set entity in new archetype")
	}

	// Remove from old archetype.
	currentArch, err := em.getArchetype(entity)
	if err != nil {
		return err
	}
	currentArch.removeEntity(entity)

	// Set the new archetype.
	em.entityArch[entity] = newArch

	return nil
}

// isAlive checks if an entity ID is currently active.
func (em *entityManager) isAlive(id EntityID) bool {
	_, exists := em.entityArch[id]
	return exists
}

// getArchetype returns the archetype associated with the given entity.
// Returns ErrEntityNotFound if the entity does not exist.
//
// Performance: O(1) for archetype lookup.
func (em *entityManager) getArchetype(entity EntityID) (*archetype, error) {
	arch, exists := em.entityArch[entity]
	if !exists {
		return nil, ErrEntityNotFound
	}
	return arch, nil
}
