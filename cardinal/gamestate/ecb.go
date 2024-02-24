package gamestate

import (
	"context"
	"encoding/json"
	"errors"

	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/iterators"
	ecslog "pkg.world.dev/world-engine/cardinal/log"
	"pkg.world.dev/world-engine/cardinal/types"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var _ Manager = &EntityCommandBuffer{}

type EntityCommandBuffer struct {
	dbStorage PrimitiveStorage[string]

	compValues         PrimitiveStorage[compKey]
	compValuesToDelete PrimitiveStorage[compKey]
	typeToComponent    map[types.ComponentID]types.ComponentMetadata

	activeEntities map[types.ArchetypeID]activeEntities

	// Fields that track the next valid entity EntityID that can be assigned
	nextEntityIDSaved uint64
	pendingEntityIDs  uint64
	isEntityIDLoaded  bool

	// Archetype EntityID management.
	entityIDToArchID       map[types.EntityID]types.ArchetypeID
	entityIDToOriginArchID map[types.EntityID]types.ArchetypeID

	archIDToComps  map[types.ArchetypeID][]types.ComponentMetadata
	pendingArchIDs []types.ArchetypeID

	logger *zerolog.Logger
}

var (
	ErrArchetypeNotFound    = errors.New("archetype for components not found")
	doesNotExistArchetypeID = types.ArchetypeID(-1)
)

// NewEntityCommandBuffer creates a new command buffer manager that is able to queue up a series of states changes and
// atomically commit them to the underlying redis dbStorage layer.
func NewEntityCommandBuffer(storage PrimitiveStorage[string]) (*EntityCommandBuffer, error) {
	m := &EntityCommandBuffer{
		dbStorage:          storage,
		compValues:         NewMapStorage[compKey, any](),
		compValuesToDelete: NewMapStorage[compKey, bool](),

		activeEntities: map[types.ArchetypeID]activeEntities{},
		archIDToComps:  map[types.ArchetypeID][]types.ComponentMetadata{},

		entityIDToArchID:       map[types.EntityID]types.ArchetypeID{},
		entityIDToOriginArchID: map[types.EntityID]types.ArchetypeID{},

		// This field cannot be set until RegisterComponents is called
		typeToComponent: nil,

		logger: &log.Logger,
	}

	return m, nil
}

func (m *EntityCommandBuffer) RegisterComponents(comps []types.ComponentMetadata) error {
	m.typeToComponent = map[types.ComponentID]types.ComponentMetadata{}
	for _, comp := range comps {
		m.typeToComponent[comp.ID()] = comp
	}

	return m.loadArchIDs()
}

// DiscardPending discards any pending state changes.
func (m *EntityCommandBuffer) DiscardPending() error {
	ctx := context.Background()
	err := m.compValues.Clear(ctx)
	if err != nil {
		return err
	}

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
	return nil
}

// RemoveEntity removes the given entity from the ECS data model.
func (m *EntityCommandBuffer) RemoveEntity(idToRemove types.EntityID) error {
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
	ctx := context.Background()
	for _, comp := range comps {
		key := compKey{comp.ID(), idToRemove}
		err = m.compValues.Delete(ctx, key)
		if err != nil {
			return err
		}
		err = m.compValuesToDelete.Set(ctx, key, true)
		if err != nil {
			return err
		}
	}

	return nil
}

// CreateEntity creates a single entity with the given set of components.
func (m *EntityCommandBuffer) CreateEntity(comps ...types.ComponentMetadata) (types.EntityID, error) {
	ids, err := m.CreateManyEntities(1, comps...)
	if err != nil {
		return 0, err
	}
	return ids[0], nil
}

// CreateManyEntities creates many entities with the given set of components.
func (m *EntityCommandBuffer) CreateManyEntities(num int, comps ...types.ComponentMetadata) ([]types.EntityID, error) {
	archID, err := m.getOrMakeArchIDForComponents(comps)
	if err != nil {
		return nil, err
	}

	ids := make([]types.EntityID, num)
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
func (m *EntityCommandBuffer) SetComponentForEntity(
	cType types.ComponentMetadata,
	id types.EntityID, value any,
) error {
	comps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return err
	}
	if !filter.MatchComponentMetadata(comps, cType) {
		return eris.Wrap(iterators.ErrComponentNotOnEntity, "")
	}

	key := compKey{cType.ID(), id}
	ctx := context.Background()
	return m.compValues.Set(ctx, key, value)
}

// GetComponentForEntity returns the saved component data for the given entity.
func (m *EntityCommandBuffer) GetComponentForEntity(cType types.ComponentMetadata, id types.EntityID) (any, error) {
	ctx := context.Background()
	key := compKey{cType.ID(), id}
	value, err := m.compValues.Get(ctx, key)
	if err == nil {
		return value, nil
	}
	// Make sure this entity has this component
	comps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return nil, err
	}
	if !filter.MatchComponentMetadata(comps, cType) {
		return nil, eris.Wrap(iterators.ErrComponentNotOnEntity, "")
	}

	// Fetch the value from storage
	redisKey := storageComponentKey(cType.ID(), id)

	bz, err := m.dbStorage.GetBytes(ctx, redisKey)
	if err != nil {
		// todo: this is redis specific, should be changed to a general error on storage
		// todo: RedisStorage needs to be modified to return this general error when a redis.Nil is detected.
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
	return value, m.compValues.Set(ctx, key, value)
}

// GetComponentForEntityInRawJSON returns the saved component data as JSON encoded bytes for the given entity.
func (m *EntityCommandBuffer) GetComponentForEntityInRawJSON(cType types.ComponentMetadata, id types.EntityID) (
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
func (m *EntityCommandBuffer) AddComponentToEntity(cType types.ComponentMetadata, id types.EntityID) error {
	fromComps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return err
	}
	if filter.MatchComponentMetadata(fromComps, cType) {
		return eris.Wrap(iterators.ErrComponentAlreadyOnEntity, "")
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
func (m *EntityCommandBuffer) RemoveComponentFromEntity(cType types.ComponentMetadata, id types.EntityID) error {
	comps, err := m.GetComponentTypesForEntity(id)
	if err != nil {
		return err
	}
	newCompSet := make([]types.ComponentMetadata, 0, len(comps)-1)
	found := false
	for _, comp := range comps {
		if comp.ID() == cType.ID() {
			found = true
			continue
		}
		newCompSet = append(newCompSet, comp)
	}
	if !found {
		return eris.Wrap(iterators.ErrComponentNotOnEntity, "")
	}
	if len(newCompSet) == 0 {
		return eris.Wrap(iterators.ErrEntityMustHaveAtLeastOneComponent, "")
	}
	key := compKey{cType.ID(), id}
	ctx := context.Background()
	err = m.compValues.Delete(ctx, key)
	if err != nil {
		return err
	}
	err = m.compValuesToDelete.Set(ctx, key, true)
	if err != nil {
		return err
	}
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
func (m *EntityCommandBuffer) GetComponentTypesForEntity(id types.EntityID) ([]types.ComponentMetadata, error) {
	archID, err := m.getArchetypeForEntity(id)
	if err != nil {
		return nil, err
	}

	return m.GetComponentTypesForArchID(archID), nil
}

// GetComponentTypesForArchID returns the set of components that are associated with the given archetype id.
func (m *EntityCommandBuffer) GetComponentTypesForArchID(archID types.ArchetypeID) []types.ComponentMetadata {
	return m.archIDToComps[archID]
}

// GetArchIDForComponents returns the archetype EntityID that has been assigned to this set of components.
// If this set of components does not have an archetype EntityID assigned to it, an error is returned.
func (m *EntityCommandBuffer) GetArchIDForComponents(components []types.ComponentMetadata) (types.ArchetypeID, error) {
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

// GetEntitiesForArchID returns all the entities that currently belong to the given archetype EntityID.
func (m *EntityCommandBuffer) GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error) {
	active, err := m.getActiveEntities(archID)
	if err != nil {
		return nil, err
	}
	return active.ids, nil
}

// SearchFrom returns an ArchetypeIterator based on a component filter. The iterator will iterate over all archetypes
// that match the given filter.
func (m *EntityCommandBuffer) SearchFrom(filter filter.ComponentFilter, start int) *iterators.ArchetypeIterator {
	itr := &iterators.ArchetypeIterator{}
	for i := start; i < len(m.archIDToComps); i++ {
		archID := types.ArchetypeID(i)
		if !filter.MatchesComponents(types.ConvertComponentMetadatasToComponents(m.archIDToComps[archID])) {
			continue
		}
		itr.Values = append(itr.Values, archID)
	}
	return itr
}

// ArchetypeCount returns the number of archetypes that have been generated.
func (m *EntityCommandBuffer) ArchetypeCount() int {
	return len(m.archIDToComps)
}

// InjectLogger sets the logger for the manager.
func (m *EntityCommandBuffer) InjectLogger(logger *zerolog.Logger) {
	m.logger = logger
}

// Close closes the manager.
func (m *EntityCommandBuffer) Close() error {
	ctx := context.Background()
	err := eris.Wrap(m.dbStorage.Close(ctx), "")
	// todo: make error general to storage and not redis specific
	// todo: adjust redis client to be return a general storage error when redis.ErrClosed is detected
	if eris.Is(eris.Cause(err), redis.ErrClosed) {
		// if redis is already closed that means another shutdown pathway got to it first.
		// There are multiple modules that will try to shutdown redis, if it is already shutdown it is not an error.
		return nil
	}
	return err
}

// getArchetypeForEntity returns the archetype EntityID for the given entity EntityID.
func (m *EntityCommandBuffer) getArchetypeForEntity(id types.EntityID) (types.ArchetypeID, error) {
	archID, ok := m.entityIDToArchID[id]
	if ok {
		return archID, nil
	}
	key := storageArchetypeIDForEntityID(id)
	num, err := m.dbStorage.GetInt(context.Background(), key)
	if err != nil {
		// todo: Make redis.Nil a general error on storage
		if errors.Is(err, redis.Nil) {
			return 0, eris.Wrap(redis.Nil, iterators.ErrEntityDoesNotExist.Error())
		}
		return 0, eris.Wrap(err, "")
	}
	archID = types.ArchetypeID(num)
	m.entityIDToArchID[id] = archID
	return archID, nil
}

// nextEntityID returns the next available entity EntityID.
func (m *EntityCommandBuffer) nextEntityID() (types.EntityID, error) {
	if !m.isEntityIDLoaded {
		// The next valid entity EntityID needs to be loaded from dbStorage.
		ctx := context.Background()
		nextID, err := m.dbStorage.GetUInt64(ctx, storageNextEntityIDKey())
		err = eris.Wrap(err, "")
		if err != nil {
			// todo: make redis.Nil a general error on storage.
			if !eris.Is(eris.Cause(err), redis.Nil) {
				return 0, err
			}
			// redis.Nil means there's no value at this key. Start with an EntityID of 0
			nextID = 0
		}
		m.nextEntityIDSaved = nextID
		m.pendingEntityIDs = 0
		m.isEntityIDLoaded = true
	}

	id := m.nextEntityIDSaved + m.pendingEntityIDs
	m.pendingEntityIDs++
	return types.EntityID(id), nil
}

// getOrMakeArchIDForComponents converts the given set of components into an archetype EntityID.
// If the set of components has already been assigned an archetype EntityID, that EntityID is returned.
// If this is a new set of components, an archetype EntityID is generated.
func (m *EntityCommandBuffer) getOrMakeArchIDForComponents(
	comps []types.ComponentMetadata,
) (types.ArchetypeID, error) {
	archID, err := m.GetArchIDForComponents(comps)
	if err == nil {
		return archID, nil
	}
	if !eris.Is(eris.Cause(err), ErrArchetypeNotFound) {
		return 0, err
	}
	// An archetype EntityID was not found. Create a pending arch EntityID
	id := types.ArchetypeID(len(m.archIDToComps))
	m.pendingArchIDs = append(m.pendingArchIDs, id)
	m.archIDToComps[id] = comps
	m.logger.Debug().Int("archetype_id", int(id)).Msg("created")
	return id, nil
}

// getActiveEntities returns the entities that are currently assigned to the given archetype EntityID.
func (m *EntityCommandBuffer) getActiveEntities(archID types.ArchetypeID) (activeEntities, error) {
	active, ok := m.activeEntities[archID]
	// The active entities for this archetype EntityID has not yet been loaded from dbStorage
	if ok {
		return m.activeEntities[archID], nil
	}
	ctx := context.Background()
	key := storageActiveEntityIDKey(archID)
	bz, err := m.dbStorage.GetBytes(ctx, key)
	err = eris.Wrap(err, "")
	var ids []types.EntityID
	if err != nil {
		// todo: this is redis specific, should be changed to a general error on storage
		// todo: RedisStorage needs to be modified to return this general error when a redis.Nil is detected.
		if !eris.Is(eris.Cause(err), redis.Nil) {
			return active, err
		}
	} else {
		ids, err = codec.Decode[[]types.EntityID](bz)
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

// setActiveEntities sets the entities that are associated with the given archetype EntityID and marks
// the information as modified so it can later be pushed to the dbStorage layer.
func (m *EntityCommandBuffer) setActiveEntities(archID types.ArchetypeID, active activeEntities) {
	active.modified = true
	m.activeEntities[archID] = active
}

// moveEntityByArchetype moves an entity EntityID from one archetype to another archetype.
func (m *EntityCommandBuffer) moveEntityByArchetype(fromArchID, toArchID types.ArchetypeID, id types.EntityID) error {
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
