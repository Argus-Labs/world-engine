package ecs

import (
	"iter"
	"math"
	"reflect"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// SystemHook defines when a system should be executed in the update cycle.
type SystemHook uint8

const (
	// PreUpdate runs before the main update.
	PreUpdate SystemHook = 0
	// Update runs during the main update phase.
	Update SystemHook = 1
	// PostUpdate runs after the main update.
	PostUpdate SystemHook = 2
	// Init runs once during world initialization.
	Init SystemHook = 3
)

// initSystem represents a system that should be run once during world initialization.
type initSystem struct {
	name string // The name of the system
	fn   func() // Function that wraps a System
}

func RegisterSystem[T any](world *World, state *T, name string, system func(), hook SystemHook) error {
	deps, err := initSystemFields(state, world)
	if err != nil {
		return eris.Wrap(err, "failed to init ecs fields")
	}

	switch hook {
	case Init:
		world.initSystems = append(world.initSystems, initSystem{name: name, fn: system})
	case PreUpdate, Update, PostUpdate:
		world.scheduler[hook].register(name, deps, system)
	default:
		return eris.Errorf("invalid system hook %d", hook)
	}

	return nil
}

type systemInitMetadata struct {
	world *World
	// Bitmaps used by the scheduler as the system's dependencies.
	depsComponent   bitmap.Bitmap
	depsSystemEvent bitmap.Bitmap
	systemEvents    map[string]struct{}
}

func initSystemFields[T any](state *T, world *World) (bitmap.Bitmap, error) {
	meta := systemInitMetadata{
		world:        world,
		systemEvents: make(map[string]struct{}),
	}

	value := reflect.ValueOf(state).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		fieldType := value.Type().Field(i)

		assert.That(field.CanAddr(), "ecs.RegisterSystem must be called by cardinal.RegisterSystem")

		fieldInstance := field.Addr().Interface()
		ecsField, ok := fieldInstance.(SystemField)

		// We move the bulk of system field checking to cardinal.initSytemFields so we just have to
		// initialize the ecs.SystemFields in this function.
		if !ok {
			continue
		}

		// Initialize the field and collect its dependencies.
		if err := ecsField.init(&meta); err != nil {
			return bitmap.Bitmap{}, eris.Wrapf(err, "failed to initialize field %s", fieldType.Name)
		}
	}

	// Add system event deps to component deps.
	deps := meta.depsComponent.Clone(nil)
	n := world.state.components.nextID
	assert.That(meta.depsSystemEvent.Count()+int(n) <= math.MaxUint32-1, "system dependencies exceed max limit")
	meta.depsSystemEvent.Range(func(x uint32) {
		deps.Set(n + x)
	})

	return deps, nil
}

type SystemField interface {
	init(meta *systemInitMetadata) error
}

var _ SystemField = &WithSystemEventReceiver[SystemEvent]{}
var _ SystemField = &WithSystemEventEmitter[SystemEvent]{}
var _ SystemField = &search[any]{}
var _ SystemField = &Contains[any]{}
var _ SystemField = &Exact[any]{}

// -------------------------------------------------------------------------------------------------
// System Event Fields
// -------------------------------------------------------------------------------------------------

// WithSystemEventReceiver is a generic system state field that allows systems to receive system
// events of type T. System events are automatically registered when the system is registered.
//
// Example:
//
//	// Define a system event for player deaths.
//	type PlayerDeath struct{ Nickname string }
//
//	func (PlayerDeath) Name() string { return "player-death" }
//
//	type GraveyardSystemState struct {
//	    PlayerDeathSystemEvents ecs.WithSystemEventReceiver[PlayerDeath]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func GraveyardSystem(state *GraveyardSystemState) error {
//	    // Receive system events emitted from another system.
//	    for systemEvent := range state.PlayerDeathSystemEvents.Iter() {
//	        // Process the system event.
//	    }
//	    return nil
//	}
type WithSystemEventReceiver[T SystemEvent] struct {
	manager *systemEventManager
}

// init initializes the system event state field.
func (s *WithSystemEventReceiver[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.systemEvents[name]; ok {
		return eris.Errorf("systems cannot process multiple system events of the same type: %s", name)
	}

	id, err := meta.world.systemEvents.register(name)
	if err != nil {
		return eris.Wrapf(err, "failed to register system event %s", name)
	}
	s.manager = &meta.world.systemEvents

	meta.systemEvents[name] = struct{}{} // Add to system's system events set for duplicate field check
	meta.depsSystemEvent.Set(id)         // Add the system event ID to the system event dependencies
	return nil
}

// Iter returns an iterator over all system events of type T.
//
// Example usage:
//
//	for systemEvent := range state.PlayerDeathEvents.Iter() {
//	    // Process each system event
//	}
func (s *WithSystemEventReceiver[T]) Iter() iter.Seq[T] {
	var zero T
	systemEvents := s.manager.get(zero.Name())

	return func(yield func(T) bool) {
		for _, systemEvent := range systemEvents {
			payload, ok := systemEvent.(T)
			assert.That(ok, "mismatched system event type")
			if !yield(payload) {
				return
			}
		}
	}
}

// WithSystemEventEmitter is a generic system state field that allows systems to emit system events
// of type T. System events are automatically registered when the system is registered.
//
// Example:
//
//	// Define a system event for player deaths.
//	type PlayerDeath struct{ Nickname string }
//
//	func (PlayerDeath) Name() string { return "player-death" }
//
//	type CombatSystemState struct {
//	    PlayerDeathSystemEvents ecs.WithSystemEventEmitter[PlayerDeath]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func CombatSystem(state *CombatSystemState) error {
//	    // Emit a player death event to be handled in another system.
//	    state.PlayerDeathEvents.Emit(PlayerDeath{Nickname: "Player1"})
//	    return nil
//	}
type WithSystemEventEmitter[T SystemEvent] struct {
	manager *systemEventManager
}

// init initializes the system event state field.
func (s *WithSystemEventEmitter[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.systemEvents[name]; ok {
		return eris.Errorf("systems cannot process multiple system events of the same type: %s", name)
	}

	id, err := meta.world.systemEvents.register(name)
	if err != nil {
		return eris.Wrapf(err, "failed to register system event %s", name)
	}
	s.manager = &meta.world.systemEvents

	meta.systemEvents[name] = struct{}{} // Add to system's system events set for duplicate field check
	meta.depsSystemEvent.Set(id)         // Add the system event ID to the system event dependencies
	return nil
}

// Emit emits a system event of type T.
//
// Example:
//
//	state.PlayerDeathEvents.Emit(PlayerDeath{Nickname: "Player1"})
func (s *WithSystemEventEmitter[T]) Emit(systemEvent T) {
	var zero T
	s.manager.enqueue(zero.Name(), systemEvent)
}

// -------------------------------------------------------------------------------------------------
// Component Search Fields
// -------------------------------------------------------------------------------------------------

// search provides type-safe component queries for entities in the world state. It uses reflection
// during initialization to figure out which components to include in the query. T must be a struct
// type composed of fields of only the type Ref[Component], e.g.:
//
//	type Particle struct {
//	    Position ecs.Ref[Position]
//	    Velocity ecs.Ref[Velocity]
//	}
//
// search is used as the base implementation for ecs.Contains and ecs.Exact which provide the
// matching behaviors for finding entities with specific component combinations. Every component
// type used in T will be automatically registered when the system is registered.
type search[T any] struct {
	world      *World        // Reference to the world
	components bitmap.Bitmap // Bitmap of component types this search looks for
	result     T             // Reusable instance of the result type
	fields     []ref         // Cached references to result's fields to be initialized in Iter
}

// init initializes the search by analyzing the generic type's struct fields and caching its
// component dependencies.
func (s *search[T]) init(meta *systemInitMetadata) error {
	var zero T
	resultType := reflect.TypeOf(zero)
	resultValue := reflect.ValueOf(&s.result).Elem()

	s.world = meta.world
	s.fields = make([]ref, resultType.NumField())

	for i := range resultType.NumField() {
		// Store a ref of the field in the search to be initialized during Iter.
		field := resultType.Field(i)
		fieldRef, ok := resultValue.Field(i).Addr().Interface().(ref)
		if !ok {
			return eris.Errorf("field %s must be of type Ref[Component], got %s", field.Name, field.Type)
		}
		s.fields[i] = fieldRef

		// Register the component.
		cid, err := fieldRef.register(s.world)
		if err != nil {
			return eris.Wrapf(err, "failed to register component %d", cid)
		}

		s.components.Set(cid)       // Add to local component set (used for archetype lookups)
		meta.depsComponent.Set(cid) // Add to system component deps (used by scheduler)
	}
	return nil
}

// Create creates a new entity with the given components. Returns an error if any of the components
// are not defined in the search field.
//
// Example:
//
//	entity, err := state.Mob.Create(Health{Value: 100}, Position{X: 0, Y: 0})
//	if err != nil {
//	    state.Logger().Error().Err(err).Msg("Failed to create entity")
//	}
//	// Use entity...
func (s *search[T]) Create() (EntityID, T) {
	ws := s.world.state
	eid := ws.newEntityWithArchetype(s.components)

	for i := range s.fields {
		s.fields[i].attach(ws, eid) // Attach the entity and world state buffer to the ref
	}

	return eid, s.result
}

// Destroy deletes an entity and all its components from the world.
//
// Example:
//
//	ok := state.Mob.Destroy(entityID)
//	if !ok {
//	    state.Logger().Warn().Msg("Entity doesn't exist or is already destroyed")
//	}
func (s *search[T]) Destroy(eid EntityID) bool {
	return Destroy(s.world.state, eid)
}

// getByID retrieves an entity's components by its ID using the provided match function to validate
// that the entity's archetype matches the search criteria.
func (s *search[T]) getByID(eid EntityID, match func(*archetype) bool) (T, error) {
	ws := s.world.state

	aid, exists := ws.entityArch.get(eid)
	if !exists {
		var zero T
		return zero, ErrEntityNotFound
	}

	arch := ws.archetypes[aid]
	if !match(arch) {
		var zero T
		return zero, ErrArchetypeMismatch
	}

	for i := range s.fields {
		s.fields[i].attach(ws, eid) // Attach the entity and world state buffer to the ref
	}
	return s.result, nil
}

// iter returns an iterator over all entities that match the given archetypes.
func (s *search[T]) iter(archetypeIDs []archetypeID) iter.Seq2[EntityID, T] {
	ws := s.world.state
	return func(yield func(EntityID, T) bool) {
		for _, id := range archetypeIDs {
			arch := ws.archetypes[id]
			for _, eid := range arch.entities {
				for i := range s.fields {
					s.fields[i].attach(ws, eid) // Attach the entity and world state buffer to the ref
				}

				if !yield(eid, s.result) {
					return
				}
			}
		}
	}
}

// Contains provides a search that matches archetypes containing all specified component types,
// potentially along with additional components.
//
// Example:
//
//	type MovementSystemState struct {
//	    Movers ecs.Contains[struct {
//	        Position ecs.Ref[Position]
//	        Velocity ecs.Ref[Velocity]
//	    }]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func MovementSystem(state *MovementSystemState) error {
//	    for entity, mover := range state.Movers.Iter() {
//	        // Process entity and compnents.
//	    }
//	    return nil
//	}
type Contains[T any] struct{ search[T] }

// Iter returns an iterator over entities and their components that match the Contains search.
//
// Example:
//
//	for _, mover := range state.Movers.Iter() {
//	    pos := mover.Position.Get()
//	    vel := mover.Velocity.Get()
//	    mover.Position.Set(Position{X: pos.X + vel.X, Y: pos.Y + vel.Y})
//	}
func (c *Contains[T]) Iter() iter.Seq2[EntityID, T] {
	return c.iter(c.world.state.archContains(c.components))
}

// GetByID retrieves an entity's components by its ID. Returns ErrEntityNotFound if the entity
// doesn't exist, or ErrArchetypeMismatch if the entity doesn't contain all the required components.
//
// Example:
//
//	mob, err := state.Mob.GetByID(entityID)
//	if err != nil {
//	    state.Logger().Warn().Err(err).Msg("Entity not found or doesn't match")
//	    return err
//	}
//	health := mob.Health.Get()
func (c *Contains[T]) GetByID(eid EntityID) (T, error) {
	return c.getByID(eid, func(arch *archetype) bool {
		return arch.contains(c.components)
	})
}

// Exact provides a search that matches archetypes containing exactly the specified component types,
// without any additional components.
//
// Example:
//
//	type PlayerSystemState struct {
//	    Players ecs.Exact[struct {
//	        Tag    ecs.Ref[PlayerTag]
//	        Health ecs.Ref[Health]
//	    }]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func PlayerSystem(state *PlayerSystemState) error {
//	    for entity, player := range state.Players.Iter() {
//	        // Process entity and compnents.
//	    }
//	    return nil
//	}
type Exact[T any] struct{ search[T] }

// Iter returns an iterator over entities and their components that match the Exact query.
//
// Example:
//
//	for _, player := range state.Players.Iter() {
//	    health := player.Health.Get()
//	    player.Health.Set(Health{HP: health.HP + 100})
//	}
func (c *Exact[T]) Iter() iter.Seq2[EntityID, T] {
	archetypes := make([]int, 0, 1)
	if id, ok := c.world.state.archExact(c.components); ok {
		archetypes = append(archetypes, id)
	}
	return c.iter(archetypes)
}

// GetByID retrieves an entity's components by its ID. Returns ErrEntityNotFound if the entity
// doesn't exist, or ErrArchetypeMismatch if the entity doesn't have exactly the required components.
//
// Example:
//
//	player, err := state.Players.GetByID(entityID)
//	if err != nil {
//	    state.Logger().Warn().Err(err).Msg("Entity not found or doesn't match")
//	    return err
//	}
//	health := player.Health.Get()
func (c *Exact[T]) GetByID(eid EntityID) (T, error) {
	return c.getByID(eid, func(arch *archetype) bool {
		return arch.exact(c.components)
	})
}

// -------------------------------------------------------------------------------------------------
// Component Handles
// -------------------------------------------------------------------------------------------------

// ref is an internal interface for component references.
type ref interface {
	attach(*worldState, EntityID)
	register(*World) (componentID, error)
}

var _ ref = &Ref[Component]{}

// Ref provides a type-safe handle to a component on an entity.
type Ref[T Component] struct {
	ws     *worldState // Internal reference to the world state
	entity EntityID    // The entity's ID
}

// attach sets the entity and world state to the Ref so that Get and Set works properly.
func (r *Ref[T]) attach(ws *worldState, eid EntityID) {
	r.ws = ws
	r.entity = eid
}

// TODO: might be possible to get the read/write type of the component in the query so we can
// optimize the scheduler by running read-only systems in parallel. e.g., we can have two different
// ref types, ReadOnlyRef and ReadWriteRef. For the read-only ref, we don't have to set its ID in
// the system bitmap.

// register returns the registerAndGetComponent type for this Ref.
func (r *Ref[T]) register(w *World) (componentID, error) {
	return registerComponent[T](w.state)
}

// Get retrieves the component value for this Ref's entity.
//
// This is the recommended system-friendly alternative to ecs.Get() for accessing components within systems.
//
// Example:
//
//	for _, player := range state.Players.Iter() {
//	    health := player.Health.Get()
//	}
func (r *Ref[T]) Get() T {
	component, err := Get[T](r.ws, r.entity)
	assert.That(err == nil, "entity doesn't exist or doesn't contain the component") // Shouldn't happen
	return component
}

// Set updates the component value for this Ref's entity.
//
// This is the recommended system-friendly alternative to ecs.Set() for modifying components within systems.
//
// Example:
//
//	for _, player := range state.Players.Iter() {
//	    player.Health.Set(Health{HP: 100})
//	}
func (r *Ref[T]) Set(component T) {
	err := Set(r.ws, r.entity, component)
	assert.That(err == nil, "entity doesn't exist") // Shouldn't happen
}
