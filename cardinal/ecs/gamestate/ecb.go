package gamestate

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/types/archetype"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

var _ GameStateManager = &EntityComponentBuffer{}

type EntityComponentBuffer struct {
	client *redis.Client

	compValues         map[compKey]any
	compValuesToDelete map[compKey]bool
	typeToComponent    map[component.TypeID]component.ComponentMetadata

	activeEntities map[archetype.ID]activeEntities

	// Fields that track the next valid entity ID that can be assigned
	nextEntityIDSaved uint64
	pendingEntityIDs  uint64
	isEntityIDLoaded  bool

	// Archetype ID management.
	entityIDToArchID       map[entity.ID]archetype.ID
	entityIDToOriginArchID map[entity.ID]archetype.ID

	archIDToComps  map[archetype.ID][]component.ComponentMetadata
	pendingArchIDs []archetype.ID

	logger *zerolog.Logger
}

var (
	ErrArchetypeNotFound    = errors.New("archetype for components not found")
	doesNotExistArchetypeID = archetype.ID(-1)
)

// NewEntityCommandBuffer creates a new command buffer manager that is able to queue up a series of states changes and
// atomically commit them to the underlying redis storage layer.
func NewEntityCommandBuffer(client *redis.Client) (*EntityComponentBuffer, error) {
	m := &EntityComponentBuffer{
		client:             client,
		compValues:         map[compKey]any{},
		compValuesToDelete: map[compKey]bool{},

		activeEntities: map[archetype.ID]activeEntities{},
		archIDToComps:  map[archetype.ID][]component.ComponentMetadata{},

		entityIDToArchID:       map[entity.ID]archetype.ID{},
		entityIDToOriginArchID: map[entity.ID]archetype.ID{},

		// This field cannot be set until RegisterComponents is called
		typeToComponent: nil,

		logger: &log.Logger,
	}

	return m, nil
}

func (m *EntityComponentBuffer) RegisterComponents(comps []component.ComponentMetadata) error {
	m.typeToComponent = map[component.TypeID]component.ComponentMetadata{}
	for _, comp := range comps {
		m.typeToComponent[comp.ID()] = comp
	}

	return m.loadArchIDs()
}

// DiscardPending discards any pending state changes.
func (m *EntityComponentBuffer) DiscardPending() {
	clear(m.compValues)

	// Any entity archetypes movements need to be undone
	clear(m.activeEntities)
	for id := range m.entityIDToOriginArchID {
		delete(m.entityIDToArchID, id)
	}
	clear(m.entityIDToOriginArchID)

	m.isEntityIDLoaded = false
	m.pendingEntityIDs = 0

	for _, archID := range m.pendingArchIDs {
		delete(m.archIDToComps, archID)
	}
	m.pendingArchIDs = m.pendingArchIDs[:0]
}

// RemoveEntity removes the given entity from the ECS data model.
func (m *EntityComponentBuffer) RemoveEntity(idToRemove entity.ID) error {
	archID, err := m.getArchetypeForEntity(idToRemove)
	if err != nil {
		return err
	}
	active, err := m.getActiveEntities(archID)
	if err != nil {
		return err
	}

	if err = active.swapRemove(idToRemove); err != nil {
		return err
	}

	m.setActiveEntities(archID, active)
	if _, ok := m.entityIDToOriginArchID[idToRemove]; !ok {
		m.entityIDToOriginArchID[idToRemove] = archID
	}
	delete(m.entityIDToArchID, idToRemove)

	comps := m.GetComponentTypesForArchID(archID)
	for _, comp := range comps {
		key := compKey{comp.ID(), idToRemove}
		delete(m.compValues, key)
		m.compValuesToDelete[key] = true
	}

	return nil
}

// CreateEntity creates a single entity with the given set of components.
func (m *EntityComponentBuffer) CreateEntity(comps ...component.ComponentMetadata) (entity.ID, error) {
	ids, err := m.CreateManyEntities(1, comps...)
	if err != nil {
		return 0, err
	}
	return ids[0], nil
}

// CreateManyEntities creates many entities with the given set of components.
func (m *EntityComponentBuffer) CreateManyEntities(num int, comps ...component.ComponentMetadata) ([]entity.ID, error) {
	archID, err := m.getOrMakeArchIDForComponents(comps)
	if err != nil {
		return nil, err
	}

	ids := make([]entity.ID, num)
	active, err := m.getActiveEntities(archID)
	if err != nil {
		return nil, err
	}
	for i := range ids {
		currID, err := m.nextEntityID()
		if err != nil {
			return nil, err
		}
		ids[i] = currID
		m.entityIDToArchID[currID] = archID
		m.entityIDToOriginArchID[currID] = doesNotExistArchetypeID
		active.ids = append(active.ids, currID)
		active.modified = true
		ecslog.Entity(m.logger, zerolog.DebugLevel, currID, archID, comps)
	}
	m.setActiveEntities(archID, active)
	return ids, nil
}

// SetComponentForEntity sets the given entity's component data to the given value.
func (m *EntityComponentBuffer) SetComponentForEntity(
	cType component.ComponentMetadata,
	id entity.ID, value any) error {
	comps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return err
	}
	if !filter.MatchComponentMetadata(comps, cType) {
		return eris.Wrap(storage.ErrComponentNotOnEntity, "")
	}

	key := compKey{cType.ID(), id}
	m.compValues[key] = value
	return nil
}

// GetComponentForEntity returns the saved component data for the given entity.
func (m *EntityComponentBuffer) GetComponentForEntity(cType component.ComponentMetadata, id entity.ID) (any, error) {
	key := compKey{cType.ID(), id}
	value, ok := m.compValues[key]
	if ok {
		return value, nil
	}
	// Make sure this entity has this component
	comps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return nil, err
	}
	if !filter.MatchComponentMetadata(comps, cType) {
		return nil, eris.Wrap(storage.ErrComponentNotOnEntity, "")
	}

	// Fetch the value from redis
	redisKey := redisComponentKey(cType.ID(), id)
	ctx := context.Background()

	bz, err := m.client.Get(ctx, redisKey).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			return nil, err
		}
		// This value has never been set. Make a default value.
		bz, err = cType.New()
		if err != nil {
			return nil, err
		}
	}
	value, err = cType.Decode(bz)
	if err != nil {
		return nil, err
	}
	m.compValues[key] = value
	return value, nil
}

// GetComponentForEntityInRawJSON returns the saved component data as JSON encoded bytes for the given entity.
func (m *EntityComponentBuffer) GetComponentForEntityInRawJSON(cType component.ComponentMetadata, id entity.ID) (
	json.RawMessage, error,
) {
	value, err := m.GetComponentForEntity(cType, id)
	if err != nil {
		return nil, err
	}
	return cType.Encode(value)
}

// AddComponentToEntity adds the given component to the given entity. An error is returned if the entity
// already has this component.
func (m *EntityComponentBuffer) AddComponentToEntity(cType component.ComponentMetadata, id entity.ID) error {
	fromComps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return err
	}
	if filter.MatchComponentMetadata(fromComps, cType) {
		return eris.Wrap(storage.ErrComponentAlreadyOnEntity, "")
	}
	toComps := append(fromComps, cType) //nolint:gocritic // easier this way.
	if err = sortComponentSet(toComps); err != nil {
		return err
	}

	toArchID, err := m.getOrMakeArchIDForComponents(toComps)
	if err != nil {
		return err
	}
	fromArchID, err := m.getOrMakeArchIDForComponents(fromComps)
	if err != nil {
		return err
	}
	return m.moveEntityByArchetype(fromArchID, toArchID, id)
}

// RemoveComponentFromEntity removes the given component from the given entity. An error is returned if the entity
// does not have the component.
func (m *EntityComponentBuffer) RemoveComponentFromEntity(cType component.ComponentMetadata, id entity.ID) error {
	comps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return err
	}
	newCompSet := make([]component.ComponentMetadata, 0, len(comps)-1)
	found := false
	for _, comp := range comps {
		if comp.ID() == cType.ID() {
			found = true
			continue
		}
		newCompSet = append(newCompSet, comp)
	}
	if !found {
		return eris.Wrap(storage.ErrComponentNotOnEntity, "")
	}
	if len(newCompSet) == 0 {
		return eris.Wrap(storage.ErrEntityMustHaveAtLeastOneComponent, "")
	}
	key := compKey{cType.ID(), id}
	delete(m.compValues, key)
	m.compValuesToDelete[key] = true
	fromArchID, err := m.getOrMakeArchIDForComponents(comps)
	if err != nil {
		return err
	}
	toArchID, err := m.getOrMakeArchIDForComponents(newCompSet)
	if err != nil {
		return err
	}
	return m.moveEntityByArchetype(fromArchID, toArchID, id)
}

// GetComponentTypesForEntity returns all the component types that are currently on the given entity. Only types
// are returned. To get the actual component data, use GetComponentForEntity.
func (m *EntityComponentBuffer) GetComponentTypesForEntity(id entity.ID) ([]component.ComponentMetadata, error) {
	archID, err := m.getArchetypeForEntity(id)
	if err != nil {
		return nil, err
	}

	return m.GetComponentTypesForArchID(archID), nil
}

// GetComponentTypesForArchID returns the set of components that are associated with the given archetype id.
func (m *EntityComponentBuffer) GetComponentTypesForArchID(archID archetype.ID) []component.ComponentMetadata {
	return m.archIDToComps[archID]
}

// GetArchIDForComponents returns the archetype ID that has been assigned to this set of components.
// If this set of components does not have an archetype ID assigned to it, an error is returned.
func (m *EntityComponentBuffer) GetArchIDForComponents(components []component.ComponentMetadata) (archetype.ID, error) {
	if len(components) == 0 {
		return 0, eris.New("must provide at least 1 component")
	}
	if err := sortComponentSet(components); err != nil {
		return 0, err
	}
	for archID, comps := range m.archIDToComps {
		if isComponentSetMatch(comps, components) {
			return archID, nil
		}
	}
	return 0, eris.Wrap(ErrArchetypeNotFound, "")
}

// GetEntitiesForArchID returns all the entities that currently belong to the given archetype ID.
func (m *EntityComponentBuffer) GetEntitiesForArchID(archID archetype.ID) ([]entity.ID, error) {
	active, err := m.getActiveEntities(archID)
	if err != nil {
		return nil, err
	}
	return active.ids, nil
}

// SearchFrom returns an ArchetypeIterator based on a component filter. The iterator will iterate over all archetypes
// that match the given filter.
func (m *EntityComponentBuffer) SearchFrom(filter filter.ComponentFilter, start int) *storage.ArchetypeIterator {
	itr := &storage.ArchetypeIterator{}
	for i := start; i < len(m.archIDToComps); i++ {
		archID := archetype.ID(i)
		if !filter.MatchesComponents(component.ConvertComponentMetadatasToComponents(m.archIDToComps[archID])) {
			continue
		}
		itr.Values = append(itr.Values, archID)
	}
	return itr
}

// ArchetypeCount returns the number of archetypes that have been generated.
func (m *EntityComponentBuffer) ArchetypeCount() int {
	return len(m.archIDToComps)
}

// InjectLogger sets the logger for the manager.
func (m *EntityComponentBuffer) InjectLogger(logger *zerolog.Logger) {
	m.logger = logger
}

// Close closes the manager.
func (m *EntityComponentBuffer) Close() error {
	err := eris.Wrap(m.client.Close(), "")
	if eris.Is(eris.Cause(err), redis.ErrClosed) {
		// if redis is already closed that means another shutdown pathway got to it first.
		// There are multiple modules that will try to shutdown redis, if it is already shutdown it is not an error.
		return nil
	}
	return err
}

// getArchetypeForEntity returns the archetype ID for the given entity ID.
func (m *EntityComponentBuffer) getArchetypeForEntity(id entity.ID) (archetype.ID, error) {
	archID, ok := m.entityIDToArchID[id]
	if ok {
		return archID, nil
	}
	key := redisArchetypeIDForEntityID(id)
	num, err := m.client.Get(context.Background(), key).Int()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, eris.Wrap(redis.Nil, storage.ErrEntityDoesNotExist.Error())
		}
		return 0, eris.Wrap(err, "")
	}
	archID = archetype.ID(num)
	m.entityIDToArchID[id] = archID
	return archID, nil
}

// nextEntityID returns the next available entity ID.
func (m *EntityComponentBuffer) nextEntityID() (entity.ID, error) {
	if !m.isEntityIDLoaded {
		// The next valid entity ID needs to be loaded from storage.
		ctx := context.Background()
		nextID, err := m.client.Get(ctx, redisNextEntityIDKey()).Uint64()
		err = eris.Wrap(err, "")
		if err != nil {
			if !eris.Is(eris.Cause(err), redis.Nil) {
				return 0, err
			}
			// redis.Nil means there's no value at this key. Start with an ID of 0
			nextID = 0
		}
		m.nextEntityIDSaved = nextID
		m.pendingEntityIDs = 0
		m.isEntityIDLoaded = true
	}

	id := m.nextEntityIDSaved + m.pendingEntityIDs
	m.pendingEntityIDs++
	return entity.ID(id), nil
}

// getOrMakeArchIDForComponents converts the given set of components into an archetype ID.
// If the set of components has already been assigned an archetype ID, that ID is returned.
// If this is a new set of components, an archetype ID is generated.
func (m *EntityComponentBuffer) getOrMakeArchIDForComponents(
	comps []component.ComponentMetadata) (archetype.ID, error) {
	archID, err := m.GetArchIDForComponents(comps)
	if err == nil {
		return archID, nil
	}
	if !eris.Is(eris.Cause(err), ErrArchetypeNotFound) {
		return 0, err
	}
	// An archetype ID was not found. Create a pending arch ID
	id := archetype.ID(len(m.archIDToComps))
	m.pendingArchIDs = append(m.pendingArchIDs, id)
	m.archIDToComps[id] = comps
	m.logger.Debug().Int("archetype_id", int(id)).Msg("created")
	return id, nil
}

// getActiveEntities returns the entities that are currently assigned to the given archetype ID.
func (m *EntityComponentBuffer) getActiveEntities(archID archetype.ID) (activeEntities, error) {
	active, ok := m.activeEntities[archID]
	// The active entities for this archetype ID has not yet been loaded from storage
	if ok {
		return m.activeEntities[archID], nil
	}
	ctx := context.Background()
	key := redisActiveEntityIDKey(archID)
	bz, err := m.client.Get(ctx, key).Bytes()
	err = eris.Wrap(err, "")
	var ids []entity.ID
	if err != nil {
		if !eris.Is(eris.Cause(err), redis.Nil) {
			return active, err
		}
	} else {
		ids, err = codec.Decode[[]entity.ID](bz)
		if err != nil {
			return active, err
		}
	}

	m.activeEntities[archID] = activeEntities{
		ids:      ids,
		modified: false,
	}
	return m.activeEntities[archID], nil
}

// setActiveEntities sets the entities that are associated with the given archetype ID and marks
// the information as modified so it can later be pushed to the storage layer.
func (m *EntityComponentBuffer) setActiveEntities(archID archetype.ID, active activeEntities) {
	active.modified = true
	m.activeEntities[archID] = active
}

// moveEntityByArchetype moves an entity ID from one archetype to another archetype.
func (m *EntityComponentBuffer) moveEntityByArchetype(fromArchID, toArchID archetype.ID, id entity.ID) error {
	if _, ok := m.entityIDToOriginArchID[id]; !ok {
		m.entityIDToOriginArchID[id] = fromArchID
	}
	m.entityIDToArchID[id] = toArchID

	active, err := m.getActiveEntities(fromArchID)
	if err != nil {
		return err
	}
	if err = active.swapRemove(id); err != nil {
		return err
	}
	m.setActiveEntities(fromArchID, active)

	active, err = m.getActiveEntities(toArchID)
	if err != nil {
		return err
	}
	active.ids = append(active.ids, id)
	m.setActiveEntities(toArchID, active)

	return nil
}
