package gamestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/redis/go-redis/v9"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
)

var (
	ErrNotInitialized       = errors.New("ecb is not ready to perform entity operations")
	ErrArchetypeNotFound    = errors.New("archetype for components not found")
	doesNotExistArchetypeID = types.ArchetypeID(-1)
)

type EntityCommandBuffer struct {
	locked bool

	dbStorage PrimitiveStorage[string]

	compValues          VolatileStorage[compKey, types.Component]
	compValuesToDelete  []compKey
	compNameToComponent map[types.ComponentName]types.ComponentMetadata

	activeEntities VolatileStorage[types.ArchetypeID, activeEntities]

	// Fields that track the next valid entity EntityID that can be assigned
	nextEntityIDSaved uint64
	pendingEntityIDs  uint64
	isEntityIDLoaded  bool

	// Archetype EntityID management.
	entityIDToArchID       map[types.EntityID]types.ArchetypeID
	entityIDToOriginArchID map[types.EntityID]types.ArchetypeID

	archIDToComps  map[types.ArchetypeID][]types.ComponentName
	pendingArchIDs []types.ArchetypeID

	// OpenTelemetry tracer
	tracer trace.Tracer
}

var _ Reader = &EntityCommandBuffer{}

// NewEntityCommandBuffer creates a new command buffer manager that is able to queue up a series of states changes and
// atomically commit them to the underlying redis dbStorage layer.
func NewEntityCommandBuffer(storage PrimitiveStorage[string]) (*EntityCommandBuffer, error) {
	m := &EntityCommandBuffer{
		locked: false,

		dbStorage: storage,

		compValues:          NewMapStorage[compKey, types.Component](),
		compValuesToDelete:  make([]compKey, 0),
		compNameToComponent: map[types.ComponentName]types.ComponentMetadata{},

		activeEntities: NewMapStorage[types.ArchetypeID, activeEntities](),

		nextEntityIDSaved: 0,
		pendingEntityIDs:  0,
		isEntityIDLoaded:  false,

		entityIDToArchID:       map[types.EntityID]types.ArchetypeID{},
		entityIDToOriginArchID: map[types.EntityID]types.ArchetypeID{},

		archIDToComps:  map[types.ArchetypeID][]types.ComponentName{},
		pendingArchIDs: []types.ArchetypeID{},

		tracer: otel.Tracer("ecb"),
	}

	return m, nil
}

// init performs the initial load from Redis and locks ECB such that no new components can be registered.
func (m *EntityCommandBuffer) init() error {
	if m.locked {
		return eris.New("ecb is already initialized")
	}

	err := m.loadArchetype()
	if err != nil {
		return err
	}

	// Lock ECB to prevent new components from being registered
	m.locked = true

	return nil
}

func (m *EntityCommandBuffer) isComponentRegistered(compName types.ComponentName) bool {
	_, ok := m.compNameToComponent[compName]
	return ok
}

func (m *EntityCommandBuffer) registerComponent(comp types.ComponentMetadata) error {
	if m.locked {
		return eris.New("unable to register components after ecb is initialized")
	}

	if _, ok := m.compNameToComponent[comp.Name()]; ok {
		return eris.New("component already registered")
	}
	m.compNameToComponent[comp.Name()] = comp

	return nil
}

// DiscardPending discards any pending state changes.
func (m *EntityCommandBuffer) DiscardPending() error {
	if !m.locked {
		return ErrNotInitialized
	}

	// Clears the in-memory cache of component values; this will force the next fetch of the component value to be
	// fetch from Redis, which contains the latest finalized state.
	if err := m.compValues.Clear(); err != nil {
		return err
	}

	// Any entity archetypes movements need to be undone
	if err := m.activeEntities.Clear(); err != nil {
		return err
	}

	// m.entityIDToOriginArchID tracks the mapping of entity ID to its origin archetype ID prior to an archetype move.
	for movedEntityID := range m.entityIDToOriginArchID {
		delete(m.entityIDToArchID, movedEntityID)
	}

	// Clear the entityIDToOriginArchID map
	m.entityIDToOriginArchID = map[types.EntityID]types.ArchetypeID{}

	// Zeroes out the entity ID counter since we want to refetch the last finalized entity ID counter from Redis.
	m.isEntityIDLoaded = false
	m.pendingEntityIDs = 0

	// Rollback all the archetypes that was created and was not finalized
	for _, archID := range m.pendingArchIDs {
		delete(m.archIDToComps, archID)
	}

	// Clear the pending archetypes entity operation queue
	m.pendingArchIDs = make([]types.ArchetypeID, 0)

	return nil
}

// RemoveEntity removes the given entity from the ECS data model.
func (m *EntityCommandBuffer) RemoveEntity(idToRemove types.EntityID) error {
	if !m.locked {
		return ErrNotInitialized
	}

	archID, err := m.getArchetypeForEntity(idToRemove)
	if err != nil {
		return err
	}

	active, err := m.getActiveEntities(archID)
	if err != nil {
		return err
	}

	if err := active.swapRemove(idToRemove); err != nil {
		return err
	}

	if err := m.setActiveEntities(archID, active); err != nil {
		return err
	}

	// See whether the origin archetype ID is already set (from a previous archetype move). If not, set it.
	// We don't want to set it again becuse archID would not be pointing to the actual origin archetype as it has been
	// changed.
	if _, ok := m.entityIDToOriginArchID[idToRemove]; !ok {
		m.entityIDToOriginArchID[idToRemove] = archID
	}

	delete(m.entityIDToArchID, idToRemove)

	for _, compName := range m.archIDToComps[archID] {
		key := compKey{compName, idToRemove}

		if err := m.compValues.Delete(key); err != nil {
			return err
		}

		m.compValuesToDelete = append(m.compValuesToDelete, key)
	}

	return nil
}

// CreateEntity creates a single entity with the given set of components.
func (m *EntityCommandBuffer) CreateEntity(comps ...types.Component) (types.EntityID, error) {
	if !m.locked {
		return 0, ErrNotInitialized
	}

	ids, err := m.CreateManyEntities(1, comps...)
	if err != nil {
		return 0, err
	}

	return ids[0], nil
}

// CreateManyEntities creates many entities with the given set of components.
func (m *EntityCommandBuffer) CreateManyEntities(num int, comps ...types.Component) ([]types.EntityID, error) {
	if !m.locked {
		return nil, ErrNotInitialized
	}

	// Check component is registered
	for _, comp := range comps {
		err := m.checkComponentRegistered(comp)
		if err != nil {
			return nil, err
		}
	}

	// Check for duplicate components
	seenComps := make([]types.ComponentName, 0, len(comps))
	for _, comp := range comps {
		if slices.Contains(seenComps, comp.Name()) {
			return nil, eris.New("duplicate component")
		}
		seenComps = append(seenComps, comp.Name())
	}

	archID, err := m.getOrMakeArchIDForComponents(seenComps)
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
	}

	if err := m.setActiveEntities(archID, active); err != nil {
		return nil, err
	}

	for _, id := range ids {
		for _, comp := range comps {
			err := m.SetComponentForEntity(id, comp)
			if err != nil {
				return nil, err
			}
		}
	}

	return ids, nil
}

// SetComponentForEntity sets the given entity's component data to the given value.
func (m *EntityCommandBuffer) SetComponentForEntity(id types.EntityID, compValue types.Component) error {
	if !m.locked {
		return ErrNotInitialized
	}

	err := m.checkComponentRegistered(compValue)
	if err != nil {
		return err
	}

	comps, err := m.getComponentTypesForEntity(id)
	if err != nil {
		return err
	}

	if !containsComponent(comps, compValue.Name()) {
		return ErrComponentNotOnEntity
	}

	err = m.compValues.Set(compKey{compValue.Name(), id}, compValue)
	if err != nil {
		return err
	}

	return nil
}

// GetComponentForEntity returns the saved component data for the given entity.
// In ECB, we store the component value after the first fetch of the component from Redis.
// If it is, we return it immmediately.
// If not, we will proceed to fetch the component value from Redis.
func (m *EntityCommandBuffer) GetComponentForEntity(comp types.Component, id types.EntityID) (any, error) {
	if !m.locked {
		return nil, ErrNotInitialized
	}

	err := m.checkComponentRegistered(comp)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	key := compKey{comp.Name(), id}

	// Case 1: The component value is already in the in-memory cache.
	compValue, err := m.compValues.Get(key)
	if err == nil {
		return compValue, nil
	}

	// Case 2: The component value is not in the in-memory cache.

	// Check if the component has the target component type.

	comps, err := m.getComponentTypesForEntity(id)
	if err != nil {
		return nil, err
	}

	if !containsComponent(comps, comp.Name()) {
		return nil, ErrComponentNotOnEntity
	}

	cType, ok := m.compNameToComponent[comp.Name()]
	if !ok {
		return nil, ErrComponentNotRegistered
	}

	// Fetch the value from Redis
	redisKey := storageComponentKey(comp.Name(), id)

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

	compValue, err = cType.Decode(bz)
	if err != nil {
		return nil, err
	}

	// Save the value to the in-memory cache
	if err := m.compValues.Set(key, compValue); err != nil {
		return nil, err
	}

	return compValue, nil
}

// GetComponentForEntityInRawJSON returns the saved component data as JSON encoded bytes for the given entity.
func (m *EntityCommandBuffer) GetComponentForEntityInRawJSON(comp types.Component, id types.EntityID) (
	json.RawMessage, error,
) {
	if !m.locked {
		return nil, ErrNotInitialized
	}

	value, err := m.GetComponentForEntity(comp, id)
	if err != nil {
		return nil, err
	}

	cType, ok := m.compNameToComponent[comp.Name()]
	if !ok {
		return nil, ErrComponentNotRegistered
	}

	return cType.Encode(value)
}

// GetAllComponentsForEntityInRawJSON returns all components for the given entity in JSON format.
func (m *EntityCommandBuffer) GetAllComponentsForEntityInRawJSON(id types.EntityID) (
	map[string]json.RawMessage, error,
) {
	if !m.locked {
		return nil, ErrNotInitialized
	}

	comps, err := m.getComponentTypesForEntity(id)
	if err != nil {
		return nil, err
	}

	result := map[string]json.RawMessage{}

	for _, compNames := range comps {
		comp := m.compNameToComponent[compNames]
		value, err := m.GetComponentForEntityInRawJSON(comp, id)
		if err != nil {
			return nil, err
		}
		result[compNames] = value
	}

	return result, nil
}

// AddComponentToEntity adds the given component to the given entity. An error is returned if the entity
// already has this component.
func (m *EntityCommandBuffer) AddComponentToEntity(comp types.Component, id types.EntityID) error {
	if !m.locked {
		return ErrNotInitialized
	}

	err := m.checkComponentRegistered(comp)
	if err != nil {
		return err
	}

	currentComps, err := m.getComponentTypesForEntity(id)
	if err != nil {
		return err
	}

	if containsComponent(currentComps, comp.Name()) {
		return ErrComponentAlreadyOnEntity
	}

	currentArch, err := m.getOrMakeArchIDForComponents(currentComps)
	if err != nil {
		return err
	}

	newArch, err := m.getOrMakeArchIDForComponents(append(currentComps, comp.Name()))
	if err != nil {
		return err
	}

	return m.moveEntityByArchetype(currentArch, newArch, id)
}

// RemoveComponentFromEntity removes the given component from the given entity. An error is returned if the entity
// does not have the component.
func (m *EntityCommandBuffer) RemoveComponentFromEntity(comp types.Component, id types.EntityID) error {
	if !m.locked {
		return ErrNotInitialized
	}

	err := m.checkComponentRegistered(comp)
	if err != nil {
		return err
	}

	entityComps, err := m.getComponentTypesForEntity(id)
	if err != nil {
		return err
	}

	isTargetCompOnEntity := false
	newCompSet := make([]types.ComponentName, 0, len(entityComps)-1)
	for _, entityComp := range entityComps {
		if entityComp == comp.Name() {
			isTargetCompOnEntity = true
			continue
		}
		newCompSet = append(newCompSet, entityComp)
	}

	if !isTargetCompOnEntity {
		return ErrComponentNotOnEntity
	}

	if len(newCompSet) == 0 {
		return ErrEntityMustHaveAtLeastOneComponent
	}

	key := compKey{comp.Name(), id}
	if err := m.compValues.Delete(key); err != nil {
		return err
	}
	m.compValuesToDelete = append(m.compValuesToDelete, key)

	fromArchID, err := m.getOrMakeArchIDForComponents(entityComps)
	if err != nil {
		return err
	}

	toArchID, err := m.getOrMakeArchIDForComponents(newCompSet)
	if err != nil {
		return err
	}

	return m.moveEntityByArchetype(fromArchID, toArchID, id)
}

// getComponentTypesForEntity returns all the component types that are currently on the given entity. Only types
// are returned. To get the actual component data, use GetComponentForEntity.
func (m *EntityCommandBuffer) getComponentTypesForEntity(id types.EntityID) ([]types.ComponentName, error) {
	archID, err := m.getArchetypeForEntity(id)
	if err != nil {
		return nil, err
	}
	return m.archIDToComps[archID], nil
}

// getArchIDForComponents returns the archetype ID that has been assigned to this set of components.
// If this set of components does not have an archetype ID assigned to it, an error is returned.
func (m *EntityCommandBuffer) getArchIDForComponents(components []types.ComponentName) (types.ArchetypeID, error) {
	if len(components) == 0 {
		return 0, eris.New("must provide at least 1 component")
	}

	for archID, comps := range m.archIDToComps {
		if err := isComponentSetMatch(comps, components); err == nil {
			return archID, nil
		}
	}

	return 0, ErrArchetypeNotFound
}

// GetEntitiesForArchID returns all the entities that currently belong to the given archetype EntityID.
func (m *EntityCommandBuffer) GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error) {
	active, err := m.getActiveEntities(archID)
	if err != nil {
		return nil, err
	}
	return active.ids, nil
}

// FindArchetypes returns a list of archetype IDs that fulfill the given component filter.
func (m *EntityCommandBuffer) FindArchetypes(f filter.ComponentFilter) ([]types.ArchetypeID, error) {
	archetypes := make([]types.ArchetypeID, 0)

	for archID, compNames := range m.archIDToComps {
		comps := make([]types.Component, 0, len(compNames))
		for _, compName := range compNames {
			comps = append(comps, m.compNameToComponent[compName])
		}

		if !f.MatchesComponents(comps) {
			continue
		}
		archetypes = append(archetypes, archID)
	}

	return archetypes, nil
}

// ArchetypeCount returns the number of archetypes that have been generated.
func (m *EntityCommandBuffer) ArchetypeCount() (int, error) {
	if !m.locked {
		return 0, ErrNotInitialized
	}
	return len(m.archIDToComps), nil
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
	// Check if the entity ID is already in the in-memory cache. If so, return the archetype ID.
	archID, ok := m.entityIDToArchID[id]
	if ok {
		return archID, nil
	}

	// If the entity ID is not in the in-memory cache, fetch the archetype ID from Redis.
	num, err := m.dbStorage.GetInt(context.Background(), storageArchetypeIDForEntityID(id))
	if err != nil {
		// todo: Make redis.Nil a general error on storage
		if errors.Is(err, redis.Nil) {
			return 0, eris.Wrap(redis.Nil, ErrEntityDoesNotExist.Error())
		}
		return 0, err
	}

	archID = types.ArchetypeID(num)

	// Save the archetype ID to in-memory cache
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
func (m *EntityCommandBuffer) getOrMakeArchIDForComponents(comps []types.ComponentName) (
	types.ArchetypeID, error,
) {
	archID, err := m.getArchIDForComponents(comps)
	if err == nil {
		return archID, nil
	}
	if !eris.Is(eris.Cause(err), ErrArchetypeNotFound) {
		return 0, err
	}

	// An archetype EntityID was not found. Create a pending arch ID
	id := types.ArchetypeID(len(m.archIDToComps))
	m.pendingArchIDs = append(m.pendingArchIDs, id)
	m.archIDToComps[id] = comps
	log.Debug().Int("archetype_id", int(id)).Msg("New archetype created")

	return id, nil
}

// getActiveEntities returns the entities that are currently assigned to the given archetype EntityID.
func (m *EntityCommandBuffer) getActiveEntities(archID types.ArchetypeID) (activeEntities, error) {
	active, err := m.activeEntities.Get(archID)
	// The active entities for this archetype EntityID has not yet been loaded from dbStorage
	if err == nil {
		return active, nil
	}

	var ids []types.EntityID

	bz, err := m.dbStorage.GetBytes(context.Background(), storageActiveEntityIDKey(archID))
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

	result := activeEntities{
		ids:      ids,
		modified: false,
	}

	if err = m.activeEntities.Set(archID, result); err != nil {
		return activeEntities{}, err
	}

	return result, nil
}

// setActiveEntities sets the entities that are associated with the given archetype EntityID and marks
// the information as modified so it can later be pushed to the dbStorage layer.
func (m *EntityCommandBuffer) setActiveEntities(archID types.ArchetypeID, active activeEntities) error {
	active.modified = true
	return m.activeEntities.Set(archID, active)
}

// moveEntityByArchetype moves an entity EntityID from one archetype to another archetype.
func (m *EntityCommandBuffer) moveEntityByArchetype(
	currentArchID, newArchID types.ArchetypeID, id types.EntityID,
) error {
	if _, ok := m.entityIDToOriginArchID[id]; !ok {
		m.entityIDToOriginArchID[id] = currentArchID
	}

	m.entityIDToArchID[id] = newArchID

	active, err := m.getActiveEntities(currentArchID)
	if err != nil {
		return err
	}

	if err := active.swapRemove(id); err != nil {
		return err
	}

	if err := m.setActiveEntities(currentArchID, active); err != nil {
		return err
	}

	active, err = m.getActiveEntities(newArchID)
	if err != nil {
		return err
	}
	active.ids = append(active.ids, id)

	if err := m.setActiveEntities(newArchID, active); err != nil {
		return err
	}

	return nil
}

// FinalizeTick combines all pending state changes into a single multi/exec redis transactions and commits them
// to the DB.
func (m *EntityCommandBuffer) FinalizeTick(ctx context.Context) error {
	if !m.locked {
		return ErrNotInitialized
	}

	ctx, span := m.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "ecb.tick.finalize")
	defer span.End()

	pipe, err := m.makePipeOfRedisCommands(ctx)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to make redis commands pipe")
	}

	if err := pipe.Incr(ctx, storageLastFinalizedTickKey()); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to increment latest finalized tick")
	}

	if err := pipe.EndTransaction(ctx); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to end transaction")
	}

	m.pendingArchIDs = nil

	if err := m.DiscardPending(); err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return eris.Wrap(err, "failed to discard pending state changes")
	}

	return nil
}

// makePipeOfRedisCommands return a pipeliner with all pending state changes to redis ready to be committed in an atomic
// transaction. If an error is returned, no redis changes will have been made.
func (m *EntityCommandBuffer) makePipeOfRedisCommands(ctx context.Context) (PrimitiveStorage[string], error) {
	ctx, span := m.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "ecb.tick.finalize.pipe_make")
	defer span.End()

	pipe, err := m.dbStorage.StartTransaction(ctx)
	if err != nil {
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return nil, err
	}

	if m.compNameToComponent == nil {
		err := eris.New("must call registerComponents before flushing to DB")
		span.SetStatus(codes.Error, eris.ToString(err, true))
		span.RecordError(err)
		return nil, err
	}

	operations := []struct {
		name   string
		method func(ctx context.Context, pipe PrimitiveStorage[string]) error
	}{
		{"component_changes", m.addComponentChangesToPipe},
		{"next_entity_id", m.addNextEntityIDToPipe},
		{"pending_arch_ids", m.addPendingArchIDsToPipe},
		{"entity_id_to_arch_id", m.addEntityIDToArchIDToPipe},
		{"active_entity_ids", m.addActiveEntityIDsToPipe},
	}

	for _, operation := range operations {
		ctx, pipeSpan := m.tracer.Start(ddotel.ContextWithStartOptions(ctx, //nolint:spancheck // false positive
			ddtracer.Measured()),
			"tick.span.finalize.pipe_make."+operation.name)

		// Perform the entity operation
		if err := operation.method(ctx, pipe); err != nil {
			span.SetStatus(codes.Error, eris.ToString(err, true))
			span.RecordError(err)
			pipeSpan.SetStatus(codes.Error, eris.ToString(err, true))
			pipeSpan.RecordError(err)
			return nil, eris.Wrapf(err, "failed to run step %q", operation.name) //nolint:spancheck // false positive
		}

		pipeSpan.End()
	}

	return pipe, nil
}

// addEntityIDToArchIDToPipe adds the information related to mapping an EntityID to its assigned archetype ArchetypeID.
func (m *EntityCommandBuffer) addEntityIDToArchIDToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	for id, originArchID := range m.entityIDToOriginArchID {
		key := storageArchetypeIDForEntityID(id)

		// Check whether the entity is still attached to an archetype
		archID, ok := m.entityIDToArchID[id]

		// Case 1: If an entity is not attached to an archetype, that means it hs been removed.
		if !ok {
			if err := pipe.Delete(ctx, key); err != nil {
				return err
			}
			continue
		}

		// Case 2: If an entity ID is the same as its origin archetype ID, that means it has not moved. Nothing to do.
		if archID == originArchID {
			continue
		}

		// Case 3: The current archetype ID is different from the origin archetype ID. Update the archetype ID.
		if err := pipe.Set(ctx, key, int(archID)); err != nil {
			return err
		}
	}

	return nil
}

// addNextEntityIDToPipe adds any changes to the next available entity ArchetypeID to the given redis pipe.
func (m *EntityCommandBuffer) addNextEntityIDToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	// There are no pending entity id creations, so there's nothing to commit
	if m.pendingEntityIDs == 0 {
		return nil
	}

	key := storageNextEntityIDKey()
	nextID := m.nextEntityIDSaved + m.pendingEntityIDs

	return pipe.Set(ctx, key, nextID)
}

// addComponentChangesToPipe adds updated component values for entities to the redis pipe.
func (m *EntityCommandBuffer) addComponentChangesToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	for _, key := range m.compValuesToDelete {
		if err := pipe.Delete(ctx, storageComponentKey(key.compName, key.entityID)); err != nil {
			return err
		}
	}

	keys, err := m.compValues.Keys()
	if err != nil {
		return err
	}

	for _, key := range keys {
		value, err := m.compValues.Get(key)
		if err != nil {
			return err
		}

		bz, err := codec.Encode(value)
		if err != nil {
			return err
		}

		redisKey := storageComponentKey(key.compName, key.entityID)
		if err = pipe.Set(ctx, redisKey, bz); err != nil {
			return eris.Wrap(err, "")
		}
	}

	m.compValuesToDelete = make([]compKey, 0)

	return nil
}

// addPendingArchIDsToPipe adds any newly created archetype IDs (as well as the associated sets of components) to the
// redis pipe.
func (m *EntityCommandBuffer) addPendingArchIDsToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	if len(m.pendingArchIDs) == 0 {
		return nil
	}

	archetypes := map[types.ArchetypeID][]types.ComponentName{}
	for archID, comps := range m.archIDToComps {
		var compNames []types.ComponentName
		for _, compName := range comps {
			compNames = append(compNames, compName)
		}
		archetypes[archID] = compNames
	}

	bz, err := codec.Encode(archetypes)
	if err != nil {
		return err
	}

	return pipe.Set(ctx, storageArchIDsToCompTypesKey(), bz)
}

// addActiveEntityIDsToPipe adds information about which entities are assigned to which archetype IDs to the reids pipe.
func (m *EntityCommandBuffer) addActiveEntityIDsToPipe(ctx context.Context, pipe PrimitiveStorage[string]) error {
	archIDs, err := m.activeEntities.Keys()
	if err != nil {
		return err
	}
	for _, archID := range archIDs {
		active, err := m.activeEntities.Get(archID)
		if err != nil {
			return err
		}
		if !active.modified {
			continue
		}
		bz, err := codec.Encode(active.ids)
		if err != nil {
			return err
		}
		key := storageActiveEntityIDKey(archID)
		err = pipe.Set(ctx, key, bz)
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
	return nil
}

// loadArchetype returns a mapping that contains the corresponding components for a given archetype ID.
func (m *EntityCommandBuffer) loadArchetype() error {
	// In ECB, it's not allowed to reload the archetype cache like this since it will overwrite the working state.
	if m.locked {
		return eris.New("archetype already loaded")
	}

	bz, err := m.dbStorage.GetBytes(context.Background(), storageArchIDsToCompTypesKey())
	if err != nil {
		// If no archetypes have been set, just terminate early.
		if eris.Is(eris.Cause(err), redis.Nil) {
			return nil
		}
		return err
	}

	archetypes, err := codec.Decode[map[types.ArchetypeID][]types.ComponentName](bz)
	if err != nil {
		return err
	}

	for archID, compNames := range archetypes {
		var comps []types.ComponentName

		// Validate component schemas
		for _, compName := range compNames {
			_, ok := m.compNameToComponent[compName]
			if !ok {
				return ErrComponentMismatchWithSavedState
			}
			comps = append(comps, compName)
		}

		m.archIDToComps[archID] = comps
	}

	return nil
}

// containsComponent returns true if the given slice of components contains the target component.
// Components are the same if they have the same Name.
func containsComponent(
	components []types.ComponentName,
	target types.ComponentName,
) bool {
	for _, c := range components {
		if target == c {
			return true
		}
	}
	return false
}

func (m *EntityCommandBuffer) checkComponentRegistered(comp types.Component) error {
	_, ok := m.compNameToComponent[comp.Name()]
	if !ok {
		return eris.Wrap(ErrComponentNotRegistered, fmt.Sprintf("component %q is not registered", comp.Name()))
	}
	return nil
}
