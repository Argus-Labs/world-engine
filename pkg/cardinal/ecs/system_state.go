package ecs

import (
	"iter"
	"math"
	"reflect"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// systemStateField defines the interface for system state initialization and basic entity
// operations. All system state fields must implement this interface.
type systemStateField interface {
	init(*World) (bitmap.Bitmap, error)
	tag() systemStateFieldType
}

var _ systemStateField = &BaseSystemState{}
var _ systemStateField = &WithCommand[Command]{}
var _ systemStateField = &WithEvent[Event]{}
var _ systemStateField = &WithSystemEventReceiver[SystemEvent]{}
var _ systemStateField = &WithSystemEventEmitter[SystemEvent]{}
var _ systemStateField = &search[any]{}
var _ systemStateField = &Contains[any]{}
var _ systemStateField = &Exact[any]{}

// -------------------------------------------------------------------------------------------------
// Base System State Field
// -------------------------------------------------------------------------------------------------

// BaseSystemState is a barebones system state field that can be embedded in your custom system
// state types to allow your systems to access the world state. It provides a raw event emitter that
// can be used to emit custom events.
//
// Example:
//
//	// Define your system state by embedding BaseState.
//	type DebugSystemState struct {
//	    ecs.BaseSystemState
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func DebugSystem(state *DebugSystemState) error {
//	    state.EmitRawEvent(EventKindCustom, "my custom event")
//	    return nil
//	}
type BaseSystemState struct {
	world *World
}

// init initializes the base system state.
func (b *BaseSystemState) init(w *World) (bitmap.Bitmap, error) {
	b.world = w
	return bitmap.Bitmap{}, nil
}

// tag returns the type of system state field.
func (b *BaseSystemState) tag() systemStateFieldType {
	return FieldBase
}

// UnsafeWorldState returns a pointer to the world's underlying state. Use this only when you know
// what you're doing as it's possible to mess up the world state.
func (b *BaseSystemState) UnsafeWorldState() *worldState { //nolint:revive // it's ok
	return b.world.state
}

// EmitRawEvent emits a raw event to the world with the given event kind and payload.
func (b *BaseSystemState) EmitRawEvent(kind EventKind, payload any) {
	b.world.events.enqueue(kind, payload)
}

// -------------------------------------------------------------------------------------------------
// Commands Fields
// -------------------------------------------------------------------------------------------------

// WithCommand is a generic system state field that allows systems to receive commands of type T.
// The commands are automatically registered when the system is registered.
//
// Example:
//
//	// Define a command type for spawning players.
//	type SpawnPlayer struct{ Nickname string }
//
//	func (SpawnPlayer) Name() string { return "spawn-player" }
//
//	// Define your system state.
//	type SpawnSystemState struct {
//	    SpawnPlayerCommands ecs.WithCommand[SpawnPlayer]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func SpawnSystem(state *SpawnSystemState) error {
//	    for cmd := range state.SpawnPlayerCommands.Iter() {
//	        persona := cmd.Persona()
//	        spawnData := cmd.Payload()
//	        // Process spawn commands based on persona and payload.
//	    }
//	    return nil
//	}
type WithCommand[T Command] struct {
	world *World
}

// init initializes the command state field.
func (m *WithCommand[T]) init(w *World) (bitmap.Bitmap, error) {
	var zero T

	id, err := w.commands.register(zero.Name())
	if err != nil {
		return bitmap.Bitmap{}, eris.Wrapf(err, "failed to register command %s", zero.Name())
	}
	m.world = w

	// Set the command ID in the bitmap so we can check that a system doesn't contain multiple
	// WithCommand fields with the same command type.
	deps := bitmap.Bitmap{}
	deps.Set(uint32(id))

	return deps, nil
}

// tag returns the type of system state field.
func (m *WithCommand[T]) tag() systemStateFieldType {
	return FieldCommand
}

// Iter returns an iterator over all commands of type T.
//
// Example usage:
//
//	for cmd := range state.SpawnPlayerCommands.Iter() {
//	    persona := cmd.Persona()
//	    payload := cmd.Payload()
//	    // Process each command
//	}
func (m *WithCommand[T]) Iter() iter.Seq[CommandContext[T]] {
	var zero T
	commands, err := m.world.commands.get(zero.Name())
	assert.That(err == nil, "command not automatically registered %s", zero.Name())

	return func(yield func(CommandContext[T]) bool) {
		for _, command := range commands {
			ctx := newCommandContext[T](&command)
			if !yield(ctx) {
				return
			}
		}
	}
}

// CommandContext wraps a micro.Command and provides typed access to command data and metadata.
type CommandContext[T Command] struct {
	raw *micro.Command
}

// newCommandContext creates a new CommandContext wrapping the given micro.Command.
func newCommandContext[T Command](raw *micro.Command) CommandContext[T] {
	return CommandContext[T]{raw: raw}
}

// Payload returns the strongly-typed command payload.
func (c CommandContext[T]) Payload() T {
	payload, ok := c.raw.Command.Body.Payload.(T)
	assert.That(ok, "mismatched command type passed to ecs")
	return payload
}

// Persona returns the persona (sender) of the command.
func (c CommandContext[T]) Persona() string {
	return c.raw.Command.Body.Persona
}

// -------------------------------------------------------------------------------------------------
// Events Fields
// -------------------------------------------------------------------------------------------------

// WithEvent is a generic system state field that allows systems to emit events of type T.
//
// Example:
//
//	// Define an event type for level ups.
//	type LevelUp struct{ Nickname string }
//
//	func (LevelUp) Name() string { return "level-up" }
//
//	type LevelUpSystemState struct {
//	    LevelUpEvents ecs.WithEvent[LevelUp]
//	    // Other fields...
//	}
//
//	// Your system function receives a pointer to your system state.
//	func LevelUpSystem(state *LevelUpSystemState) error {
//	    // Emit a level up event.
//	    state.LevelUpEvents.Emit(LevelUp{Nickname: "Player1"})
//	    return nil
//	}
type WithEvent[T Event] struct {
	world *World
}

// init initializes the event state field.
func (e *WithEvent[T]) init(w *World) (bitmap.Bitmap, error) {
	var zero T

	id, err := w.events.register(zero.Name())
	if err != nil {
		return bitmap.Bitmap{}, eris.Wrapf(err, "failed to register event %s", zero.Name())
	}
	e.world = w

	deps := bitmap.Bitmap{}
	deps.Set(id)
	return deps, nil
}

// tag returns the type of system state field.
func (e *WithEvent[T]) tag() systemStateFieldType {
	return FieldEvent
}

// Emit emits an event of tpe T.
//
// Example:
//
//	state.LevelUpEvents.Emit(LevelUp{Nickname: "Player1"})
func (e *WithEvent[T]) Emit(event T) {
	e.world.events.enqueue(EventKindDefault, event)
}

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
	world *World
}

// init initializes the system event state field.
func (s *WithSystemEventReceiver[T]) init(w *World) (bitmap.Bitmap, error) {
	var zero T

	id, err := w.systemEvents.register(zero.Name())
	if err != nil {
		return bitmap.Bitmap{}, eris.Wrapf(err, "failed to register system event")
	}
	s.world = w

	// Set the system event ID in the bitmap so the scheduler can order the systems correctly.
	deps := bitmap.Bitmap{}
	deps.Set(id)

	return deps, nil
}

// tag returns the type of system state field.
func (s *WithSystemEventReceiver[T]) tag() systemStateFieldType {
	return FieldSystemEventReceiver
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
	systemEvents, err := s.world.systemEvents.get(zero.Name())
	assert.That(err == nil, "system event not automatically registered %s", zero.Name())

	return func(yield func(T) bool) {
		for _, systemEvent := range systemEvents {
			if !yield(systemEvent.(T)) { //nolint:errcheck // We know the type
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
	world *World
}

// init initializes the system event state field.
func (s *WithSystemEventEmitter[T]) init(w *World) (bitmap.Bitmap, error) {
	var zero T

	id, err := w.systemEvents.register(zero.Name())
	if err != nil {
		return bitmap.Bitmap{}, eris.Wrapf(err, "failed to register system event")
	}
	s.world = w

	// Set the system event ID in the bitmap so the scheduler can order the systems correctly.
	deps := bitmap.Bitmap{}
	deps.Set(id)

	return deps, nil
}

// tag returns the type of system state field.
func (s *WithSystemEventEmitter[T]) tag() systemStateFieldType {
	return FieldSystemEventEmitter
}

// Emit emits a system event of type T.
//
// Example:
//
//	state.PlayerDeathEvents.Emit(PlayerDeath{Nickname: "Player1"})
func (s *WithSystemEventEmitter[T]) Emit(systemEvent T) {
	var zero T
	err := s.world.systemEvents.enqueue(zero.Name(), systemEvent)
	assert.That(err == nil, "system event not automatically registered %s", zero.Name())
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
func (s *search[T]) init(w *World) (bitmap.Bitmap, error) {
	var zero T
	resultType := reflect.TypeOf(zero)
	resultValue := reflect.ValueOf(&s.result).Elem()

	s.world = w
	s.fields = make([]ref, resultType.NumField())

	for i := range resultType.NumField() {
		// Store a ref of the field in the search to be initialized during Iter.
		field := resultType.Field(i)
		fieldRef, ok := resultValue.Field(i).Addr().Interface().(ref)
		if !ok {
			return bitmap.Bitmap{}, eris.Errorf("field %s must be of type Ref[Component], got %s", field.Name, field.Type)
		}
		s.fields[i] = fieldRef

		// Register the component.
		cid, err := fieldRef.register(w)
		if err != nil {
			return bitmap.Bitmap{}, err
		}

		// Set the component ID in the bitmap so the scheduler can order the systems correctly.
		s.components.Set(cid)
	}

	return s.components, nil
}

// tag returns the type of system state field.
func (s *search[T]) tag() systemStateFieldType {
	return FieldComponent
}

// Create creates a new entity with the given components. Returns an error if any of the components
// are not defined in the search field.
//
// This is the recommended system-friendly alternative to ecs.Create() for creating entities within systems.
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
// This is the recommended system-friendly alternative to ecs.Destroy() for destroying entities within systems.
func (s *search[T]) Destroy(eid EntityID) bool {
	return Destroy(s.world.state, eid)
}

func (s *search[T]) GetByID(eid EntityID) (T, bool) {
	ws := s.world.state

	if !Alive(ws, eid) {
		var zero T
		return zero, false
	}

	for i := range s.fields {
		s.fields[i].attach(ws, eid) // Attach the entity and world state buffer to the ref
	}
	return s.result, true
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

// -------------------------------------------------------------------------------------------------
// Internal
// -------------------------------------------------------------------------------------------------

// systemStateFieldType is an enum type for system state field types.
type systemStateFieldType uint8

const (
	// FieldComponent is the systemStateFieldType for Contains and Exact.
	FieldComponent systemStateFieldType = iota
	// FieldSystemEventReceiver is the systemStateFieldType for WithSystemEventReceiver.
	FieldSystemEventReceiver
	// FieldSystemEventEmitter is the systemStateFieldType for WithSystemEventEmitter.
	FieldSystemEventEmitter
	// FieldBase is the systemStateFieldType for BaseSystemState.
	FieldBase
	// FieldEvent is the systemStateFieldType for WithEvent.
	FieldEvent
	// FieldCommand is the systemStateFieldType for WithCommand.
	FieldCommand
)

// Helper function to initialize fields when registering systems.
func initializeSystemState[T any]( //nolint:gocognit // Will refactor after things are stable
	w *World,
	state *T,
	modifiers map[systemStateFieldType]func(any) error,
) (bitmap.Bitmap, error) {
	// Bitmaps used by the scheduler as the system's dependencies.
	var componentDeps bitmap.Bitmap
	var systemEventDeps bitmap.Bitmap

	// Bitmaps to check for duplicate fields that operate on the same type. A system cannot process
	// multiples of the same type, e.g. multiple WithCommand[T] with the same T type.
	var commandDeps bitmap.Bitmap
	var eventDeps bitmap.Bitmap
	var systemEventReceiverDeps bitmap.Bitmap
	var systemEventEmitterDeps bitmap.Bitmap

	// For each field in the system state, initialize the field and collect its dependencies.
	value := reflect.ValueOf(state).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		fieldType := value.Type().Field(i)

		// If the field is not exported, return an error.
		if !field.CanAddr() {
			return componentDeps, eris.Errorf("field %s must be exported", fieldType.Name)
		}

		// If the field doesn't implement systemStateField, return an error. This shouldn't happen
		// as long as the user sticks to the provided system state field types.
		fieldInstance := field.Addr().Interface()
		stateField, ok := fieldInstance.(systemStateField)
		if !ok {
			return componentDeps, eris.Errorf("field %s must implement SystemStateField", fieldType.Name)
		}

		// Initialize the field and collect its dependencies.
		deps, err := stateField.init(w)
		if err != nil {
			return componentDeps, eris.Wrapf(err, "failed to initialize field %s", fieldType.Name)
		}

		// Add field dependencies to the system dependencies.
		tag := stateField.tag()
		switch tag {
		case FieldComponent:
			componentDeps.Or(deps)
		case FieldSystemEventReceiver:
			if hasDuplicate(systemEventReceiverDeps, deps) {
				return componentDeps, eris.New(
					"systems cannot declare multiple WithSystemEventReceiver fields of the same system event type")
			}
			systemEventReceiverDeps.Or(deps) // Add to seen list
			systemEventDeps.Or(deps)         // Add to scheduler deps
		case FieldSystemEventEmitter:
			if hasDuplicate(systemEventEmitterDeps, deps) {
				return componentDeps, eris.New(
					"systems cannot declare multiple WithSystemEventEmitter fields of the same system event type")
			}
			systemEventEmitterDeps.Or(deps) // Add to seen list
			systemEventDeps.Or(deps)        // Add to scheduler deps
		case FieldCommand:
			if hasDuplicate(commandDeps, deps) {
				return componentDeps, eris.New("systems cannot process multiple commands of the same type")
			}
			commandDeps.Or(deps) // Add to seen list
		case FieldEvent:
			if hasDuplicate(eventDeps, deps) {
				return componentDeps, eris.New("systems cannot declare multiple WithEvent fields of the same event type")
			}
			eventDeps.Or(deps) // Add to seen list
		case FieldBase:
		}

		// Run the field modifier functions if they're set.
		for t, modifier := range modifiers {
			if t == tag {
				if err := modifier(fieldInstance); err != nil {
					return componentDeps, eris.Wrapf(err, "error initializing field %s", fieldType.Name)
				}
			}
		}
	}

	// Add system event deps to component deps.
	n := w.state.components.nextID
	assert.That(systemEventDeps.Count()+int(n) <= math.MaxUint32-1, "system dependencies exceed max limit")
	systemEventDeps.Range(func(x uint32) {
		componentDeps.Set(n + x)
	})

	return componentDeps, nil
}

// hasDuplicate checks if any bits in deps are already set in aggregate.
func hasDuplicate(aggregate, deps bitmap.Bitmap) bool {
	clone := deps.Clone(nil)
	clone.And(aggregate)
	return clone.Count() != 0
}
