package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// WorldState holds the state of the world.
type WorldState struct {
	world      *World        // Reference to the world
	entities   entityManager // Manages entity IDs and archetype mappings
	archetypes []archetype   // Contain all archetypes that exist where the index is the archetype ID
}

// newWorldState creates a new world state.
func newWorldState(world *World) WorldState {
	return WorldState{
		world:      world,
		entities:   newEntityManager(),
		archetypes: make([]archetype, 0),
	}
}

// findOrCreateArchetype finds an existing archetype that matches the component types or creates a
// new archetype if none match.
func (ws *WorldState) findOrCreateArchetype(components bitmap.Bitmap) *archetype {
	// First try to find existing archetype, if found return it.
	if arch := ws.archExact(components); arch != nil {
		return arch
	}

	// Create new archetype if none found.
	archID := archetypeID(len(ws.archetypes)) // archID = index in archetypes array
	arch := ws.world.components.createArchetype(archID, components)
	ws.archetypes = append(ws.archetypes, arch)

	return &ws.archetypes[len(ws.archetypes)-1]
}

// archContains returns all archetypes that have the given component types.
func (ws *WorldState) archContains(components bitmap.Bitmap) []*archetype {
	var archs []*archetype
	for i := range ws.archetypes {
		if ws.archetypes[i].hasComponents(components) {
			archs = append(archs, &ws.archetypes[i])
		}
	}
	return archs
}

// archExact returns the archetype that exactly matches the given component types.
func (ws *WorldState) archExact(components bitmap.Bitmap) *archetype {
	for i := range ws.archetypes {
		if ws.archetypes[i].matches(components) {
			return &ws.archetypes[i]
		}
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Entity Operations
// -------------------------------------------------------------------------------------------------

// opNewEntity creates a new entity in the write buffer.
func (ws *WorldState) opNewEntity(components []Component) (EntityID, error) {
	// Create the components bitmap from the provided components.
	compBitmap, err := ws.world.components.toComponentBitmap(components)
	if err != nil {
		return 0, eris.Wrap(err, "failed to create component bitmap")
	}

	// Get the archetype or create a new one if it doesn't exist.
	arch := ws.findOrCreateArchetype(compBitmap)
	// Create the entity.
	entity, err := ws.entities.new(arch, components)
	if err != nil {
		return 0, err
	}

	return entity, nil
}

// opRemoveEntity removes an entity from the world.
func (ws *WorldState) opRemoveEntity(entity EntityID) error {
	return ws.entities.remove(entity)
}

// opMoveEntity moves an entity to a new archetype.
func (ws *WorldState) opMoveEntity(entity EntityID, newComps []Component) error {
	compBitmap, err := ws.world.components.toComponentBitmap(newComps)
	if err != nil {
		return err
	}

	// Get the current archetype of the entity
	currentArch, err := ws.entities.getArchetype(entity)
	if err != nil {
		return err
	}
	// Find or create the new archetype
	newArch := ws.findOrCreateArchetype(compBitmap)
	err = ws.entities.move(entity, newArch, newComps)
	if err != nil {
		return err
	}
	// If the entity is already in the new archetype, something went wrong here.
	assert.That(currentArch.id != newArch.id, "entity moved into its existing archetype")
	return nil
}

// opSetComponent updates a component value for an entity.
func opSetComponent[T Component](entity EntityID, arch *archetype, component T) error {
	if !arch.hasEntity(entity) {
		return eris.Errorf("entity %d not in archetype", entity)
	}

	col, err := getColumnFromArch[T](arch)
	assert.That(err == nil, "failed to get column from archetype")

	return col.set(entity, component)
}

// -------------------------------------------------------------------------------------------------
// Serialization methods
// -------------------------------------------------------------------------------------------------

// serialize converts the WorldState to a protobuf message for serialization.
func (ws *WorldState) serialize() (*cardinalv1.CardinalSnapshot, error) {
	pbArchetypes := make([]*cardinalv1.Archetype, len(ws.archetypes))
	for i, arch := range ws.archetypes {
		pbArch, err := arch.serialize()
		if err != nil {
			return nil, eris.Wrapf(err, "failed to serialize archetype %d", i)
		}
		pbArchetypes[i] = pbArch
	}

	// Convert entity manager fields.
	ws.entities.mu.Lock()
	defer ws.entities.mu.Unlock()

	// Convert entityArch map[EntityID]*archetype to map[EntityID]archetypeID.
	entityArchetypes := make(map[uint32]uint64, len(ws.entities.entityArch))
	for entityID, arch := range ws.entities.entityArch {
		entityArchetypes[uint32(entityID)] = arch.id
	}

	// Convert free list to uint32 slice.
	freeIDs := make([]uint32, len(ws.entities.free))
	for i, entityID := range ws.entities.free {
		freeIDs[i] = uint32(entityID)
	}

	return &cardinalv1.CardinalSnapshot{
		Archetypes:       pbArchetypes,
		NextId:           uint32(ws.entities.nextID),
		FreeIds:          freeIDs,
		EntityArchetypes: entityArchetypes,
	}, nil
}

// deserialize populates the WorldState from a protobuf message.
func (ws *WorldState) deserialize(pb *cardinalv1.CardinalSnapshot) error {
	// Deserialize archetypes first.
	ws.archetypes = make([]archetype, len(pb.GetArchetypes()))
	for i, pbArch := range pb.GetArchetypes() {
		if err := ws.archetypes[i].deserialize(pbArch, &ws.world.components); err != nil {
			return eris.Wrapf(err, "failed to deserialize archetype %d", i)
		}
	}

	// Restore entity manager state.
	ws.entities.mu.Lock()
	defer ws.entities.mu.Unlock()

	ws.entities.nextID = EntityID(pb.GetNextId())

	// Convert free IDs from uint32 slice.
	ws.entities.free = make([]EntityID, len(pb.GetFreeIds()))
	for i, freeID := range pb.GetFreeIds() {
		ws.entities.free[i] = EntityID(freeID)
	}

	// Convert entity_archetypes map[EntityID]archetypeID back to map[EntityID]*archetype.
	ws.entities.entityArch = make(map[EntityID]*archetype, len(pb.GetEntityArchetypes()))
	for entityID, archID := range pb.GetEntityArchetypes() {
		if archID >= uint64(len(ws.archetypes)) {
			return eris.Errorf("invalid archetype ID %d for entity %d", archID, entityID)
		}
		ws.entities.entityArch[EntityID(entityID)] = &ws.archetypes[archID]
	}

	return nil
}
