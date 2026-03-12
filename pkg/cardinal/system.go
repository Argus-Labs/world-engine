package cardinal

import (
	"fmt"
	"iter"
	"reflect"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type EntityID = ecs.EntityID

func RegisterSystem[T any](world *World, system func(*T), opts ...SystemOption) {
	cfg := newSystemConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	// Check that the system stateType embeds BaseSystemState.
	var zero T
	stateType := reflect.TypeOf(zero)
	if _, ok := stateType.FieldByName("BaseSystemState"); !ok {
		panic(eris.Errorf("system %T must embed cardinal.BaseSystemState", system))
	}

	// Initialize the fields in the system state.
	state := new(T)

	deps1, deps2, err := initSystemFields(state, world)
	if err != nil {
		panic(eris.Wrapf(err, "error initializing system fields"))
	}

	err = ecs.RegisterSystem(world.world, ecs.RegisterSystemOptions[T]{
		Name:            fmt.Sprintf("%T", system),
		State:           state,
		System:          func() { system(state) },
		Hook:            cfg.hook,
		DepsComponent:   deps1,
		DepsSystemEvent: deps2,
	})
	if err != nil {
		panic(eris.Wrapf(err, "error registering system"))
	}
}

func initSystemFields[T any](state *T, world *World) (bitmap.Bitmap, bitmap.Bitmap, error) {
	meta := systemInitMetadata{
		world:        world,
		commands:     make(map[string]struct{}),
		events:       make(map[string]struct{}),
		systemEvents: make(map[string]struct{}),
	}

	// For each field in the system state, initialize the field and collect its dependencies.
	value := reflect.ValueOf(state).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		fieldType := value.Type().Field(i)

		// If the field is not exported, return an error.
		if !field.CanAddr() {
			return bitmap.Bitmap{}, bitmap.Bitmap{}, eris.Errorf("field %s must be exported", fieldType.Name)
		}

		fieldInstance := field.Addr().Interface()

		cardinalField, ok := fieldInstance.(systemField)
		if ok {
			if err := cardinalField.init(&meta); err != nil {
				return bitmap.Bitmap{}, bitmap.Bitmap{}, eris.Wrapf(err, "failed to initialize field %s", fieldType.Name)
			}
		}
		// For now we'll ignore other fields in the system state struct.
	}

	return meta.depsComponent, meta.depsSystemEvent, nil
}

type systemInitMetadata struct {
	world           *World
	commands        map[string]struct{}
	events          map[string]struct{}
	systemEvents    map[string]struct{}
	depsComponent   bitmap.Bitmap
	depsSystemEvent bitmap.Bitmap
}

type systemField interface {
	init(meta *systemInitMetadata) error
}

var _ systemField = (*BaseSystemState)(nil)
var _ systemField = (*WithCommand[Command])(nil)
var _ systemField = (*WithEvent[Event])(nil)
var _ systemField = (*WithSystemEventReceiver[ecs.Component])(nil)
var _ systemField = (*WithSystemEventEmitter[ecs.Component])(nil)
var _ systemField = (*search[ecs.Component])(nil)
var _ systemField = (*Contains[ecs.Component])(nil)
var _ systemField = (*Exact[ecs.Component])(nil)

// TODO: how would a All[ecs.Component] look like? it must be typesafe too.

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// systemConfig holds all configurable options for system registration.
type systemConfig struct {
	// The hook that determines when the system should be executed.
	hook ecs.SystemHook
}

// newSystemConfig creates a new system config with default values.
func newSystemConfig() systemConfig {
	return systemConfig{
		hook: Update,
	}
}

// SystemOption is a function that configures a SystemConfig.
type SystemOption func(*systemConfig)

// SystemHook defines when a system should be executed in the update cycle.
type SystemHook = ecs.SystemHook

const (
	// PreUpdate runs before the main update.
	PreUpdate = ecs.PreUpdate
	// Update runs during the main update phase.
	Update = ecs.Update
	// PostUpdate runs after the main update.
	PostUpdate = ecs.PostUpdate
	// Init runs once during world initialization.
	Init = ecs.Init
)

// WithHook returns an option to set the system hook.
func WithHook(hook SystemHook) SystemOption {
	return func(cfg *systemConfig) { cfg.hook = hook }
}

// -------------------------------------------------------------------------------------------------
// Base
// -------------------------------------------------------------------------------------------------

type BaseSystemState struct {
	world *World
}

func (b *BaseSystemState) init(meta *systemInitMetadata) error {
	b.world = meta.world
	return nil
}

// TODO: pass init args (similar to boot info) to get system name in logger.
// Logger returns the logger for the world.
func (b *BaseSystemState) Logger() *zerolog.Logger {
	logger := b.world.tel.GetLogger("system")
	return &logger
}

// Tick returns the current tick of the world.
func (b *BaseSystemState) Tick() uint64 {
	return b.world.currentTick.height
}

// Timestamp returns the current timestamp of the world.
func (b *BaseSystemState) Timestamp() time.Time {
	return b.world.currentTick.timestamp
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

type Command = command.Payload

type WithCommand[T Command] struct {
	manager *command.Manager
	id      command.ID
}

func (c *WithCommand[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.commands[name]; ok {
		return eris.Errorf("systems cannot process multiple commands of the same type: %s", name)
	}

	id, err := meta.world.commands.Register(name, command.NewQueue[T]())
	if err != nil {
		return eris.Wrapf(err, "failed to register command %s", name)
	}

	// Register the command handler with NATS. NOTE: this just adds to the service's command name set,
	// it doesn't create the NATS subscription/request handler immediately. This method is free of
	// side effects so we can test without NATS.
	meta.world.service.registerCommandHandler(name)

	if err := meta.world.debug.register("command", zero); err != nil {
		return eris.Wrapf(err, "failed to register command to debug module %s", name)
	}

	meta.commands[name] = struct{}{} // Add to system commands set for duplicate field check

	c.manager = &meta.world.commands
	c.id = id
	return nil
}

func (c *WithCommand[T]) Iter() iter.Seq[CommandContext[T]] {
	var zero T
	commands, err := c.manager.Get(c.id)
	assert.That(err == nil, "command not automatically registered %s", zero.Name())

	return func(yield func(CommandContext[T]) bool) {
		for _, cmd := range commands {
			if !yield(newCommandContext[T](cmd)) {
				return
			}
		}
	}
}

type CommandContext[T Command] struct {
	Payload T
	Persona string
}

func newCommandContext[T Command](cmd command.Command) CommandContext[T] {
	payload, ok := cmd.Payload.(T)
	assert.That(ok, "mismatched command type passed to command context")

	return CommandContext[T]{
		Payload: payload,
		Persona: cmd.Persona,
	}
}

// -------------------------------------------------------------------------------------------------
// Inter-Shard Commands
// -------------------------------------------------------------------------------------------------

// OtherWorld is a type that represents the address of an external service.
type OtherWorld struct {
	Region       string
	Organization string
	Project      string
	ShardID      string
}

// SendCommand sends a command to an external service.
//
// Example:
//
// import external "another-game-shard/system"
//
// // Define a shard address of one of your game shards.
// const MatchmakingService ecs.ServiceAddress = "world.argus.rampage.matchmaking"
//
//	func GameSystem(state *GameSystem) error {
//	  MatchmakingService.Send(state, &external.EndGameCommand{
//	    Winner: "Team 1",
//	  })
//	}
func (o OtherWorld) SendCommand(state *BaseSystemState, cmd command.Payload) {
	serviceAddress := micro.GetAddress(o.Region, micro.RealmWorld, o.Organization, o.Project, o.ShardID)
	state.world.events.Enqueue(event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    cmd.Name(),
			Persona: micro.String(state.world.address),
			Address: serviceAddress,
			Payload: cmd,
		},
	})
}

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

type Event = event.Payload

type WithEvent[T Event] struct {
	manager *event.Manager
}

func (e *WithEvent[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.events[name]; ok {
		return eris.Errorf("systems cannot process multiple events of the same type: %s", name)
	}

	if err := meta.world.debug.register("event", zero); err != nil {
		return eris.Wrapf(err, "failed to register command to debug module %s", name)
	}

	meta.events[name] = struct{}{} // Add to system events set for duplicate field check

	e.manager = &meta.world.events
	return nil
}

func (e *WithEvent[T]) Emit(evt T) {
	e.manager.Enqueue(event.Event{
		Kind:    event.KindDefault,
		Payload: evt,
	})
}

// -------------------------------------------------------------------------------------------------
// System Events
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
type WithSystemEventReceiver[T ecs.SystemEvent] struct {
	world *ecs.World
}

// init initializes the system event state field.
func (s *WithSystemEventReceiver[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.systemEvents[name]; ok {
		return eris.Errorf("systems cannot process multiple system events of the same type: %s", name)
	}

	id, err := ecs.RegisterSystemEvent[T](meta.world.world)
	if err != nil {
		return eris.Wrapf(err, "failed to register system event %s", name)
	}
	s.world = meta.world.world

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
	systemEvents, err := ecs.GetSystemEvents[T](s.world)
	assert.That(err == nil, "tried to get unregisterd system event")

	return func(yield func(T) bool) {
		for _, systemEvent := range systemEvents {
			if !yield(systemEvent) {
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
type WithSystemEventEmitter[T ecs.SystemEvent] struct {
	world *ecs.World
}

// init initializes the system event state field.
func (s *WithSystemEventEmitter[T]) init(meta *systemInitMetadata) error {
	var zero T
	name := zero.Name()

	if _, ok := meta.systemEvents[name]; ok {
		return eris.Errorf("systems cannot process multiple system events of the same type: %s", name)
	}

	id, err := ecs.RegisterSystemEvent[T](meta.world.world)
	if err != nil {
		return eris.Wrapf(err, "failed to register system event %s", name)
	}
	s.world = meta.world.world

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
	err := ecs.EmitSystemEvent(s.world, systemEvent)
	assert.That(err == nil, "tried to emit unregistered system event")
}

// -------------------------------------------------------------------------------------------------
// Components
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
	world      *ecs.World    // Reference to the world
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

	s.world = meta.world.world
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

// getByID retrieves an entity's components by its ID using the provided match function to validate
// that the entity's archetype matches the search criteria.
func (s *search[T]) getByID(eid EntityID, match ecs.SearchMatch) (T, error) {
	if err := ecs.MatchArchetype(s.world, eid, s.components, match); err != nil {
		var zero T
		return zero, eris.Wrap(err, "failed to get entity")
	}
	for i := range s.fields {
		s.fields[i].attach(s.world, eid) // Attach the entity and world state buffer to the ref
	}
	return s.result, nil
}

// iter returns an iterator over all entities that match the given archetypes.
func (s *search[T]) iter(match ecs.SearchMatch) SearchResult[EntityID, T] {
	return func(yield func(EntityID, T) bool) {
		err := ecs.IterEntities(s.world, s.components, match, func(eid EntityID) bool {
			for i := range s.fields {
				s.fields[i].attach(s.world, eid) // Attach the entity and world state buffer to the ref
			}

			return yield(eid, s.result)
		})
		assert.That(err == nil, "invalid arguments sent to IterEntities")
	}
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
	eid := ecs.CreateWithArchetype(s.world, s.components)

	for i := range s.fields {
		s.fields[i].attach(s.world, eid) // Attach the entity and world state buffer to the ref
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
	return ecs.Destroy(s.world, eid)
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
func (c *Contains[T]) Iter() SearchResult[EntityID, T] {
	return c.iter(ecs.MatchContains)
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
	return c.getByID(eid, ecs.MatchContains)
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
func (c *Exact[T]) Iter() SearchResult[EntityID, T] {
	return c.iter(ecs.MatchExact)
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
	return c.getByID(eid, ecs.MatchExact)
}

// -------------------------------------------------------------------------------------------------
// Component Handles
// -------------------------------------------------------------------------------------------------

// ref is an internal interface for component references.
type ref interface {
	attach(*ecs.World, EntityID)
	register(*ecs.World) (ecs.ComponentID, error)
}

var _ ref = &Ref[ecs.Component]{}

// Ref provides a type-safe handle to a component on an entity.
type Ref[T ecs.Component] struct {
	ws     *ecs.World // Internal reference to the world state
	entity EntityID   // The entity's ID
}

// attach sets the entity and world state to the Ref so that Get and Set works properly.
func (r *Ref[T]) attach(ws *ecs.World, eid EntityID) {
	r.ws = ws
	r.entity = eid
}

// TODO: might be possible to get the read/write type of the component in the query so we can
// optimize the scheduler by running read-only systems in parallel. e.g., we can have two different
// ref types, ReadOnlyRef and ReadWriteRef. For the read-only ref, we don't have to set its ID in
// the system bitmap.

// register returns the registerAndGetComponent type for this Ref.
func (r *Ref[T]) register(w *ecs.World) (ecs.ComponentID, error) {
	return ecs.RegisterComponent[T](w)
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
	component, err := ecs.Get[T](r.ws, r.entity)
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
	err := ecs.Set(r.ws, r.entity, component)
	assert.That(err == nil, "entity doesn't exist") // Shouldn't happen
}

// Remove removes the component from this Ref's entity.
//
// This is the recommended system-friendly alternative to ecs.Remove() for removing components within systems.
//
// Example:
//
//	for _, player := range state.Players.Iter() {
//	    player.Shield.Remove()
//	}
func (r *Ref[T]) Remove() {
	err := ecs.Remove[T](r.ws, r.entity)
	assert.That(err == nil, "entity doesn't exist or doesn't contain the component") // Shouldn't happen
}

// -------------------------------------------------------------------------------------------------
// Component Search Result Modifiers
// -------------------------------------------------------------------------------------------------

var (
	ErrSingleNoResult       = eris.New("expected exactly 1 result, got 0")
	ErrSingleMultipleResult = eris.New("expected exactly 1 result, got more than 1")
)

// SearchResult is a chainable iterator over key-value pairs.
type SearchResult[E EntityID, C any] func(yield func(E, C) bool)

// Filter returns a new iterator that only yields values that satisfy predicate. A nil predicate
// returns the original iterator unchanged.
func (s SearchResult[E, C]) Filter(predicate func(E, C) bool) SearchResult[E, C] {
	if predicate == nil {
		return s
	}

	return func(yield func(E, C) bool) {
		for e, c := range s {
			if !predicate(e, c) {
				continue
			}
			if !yield(e, c) {
				return
			}
		}
	}
}

// Limit returns a new iterator that yields at most limit values. A limit <= 0 yields no values.
func (s SearchResult[E, C]) Limit(limit uint32) SearchResult[E, C] {
	return func(yield func(E, C) bool) {
		yielded := uint32(0)
		for e, c := range s {
			if !yield(e, c) {
				return
			}

			yielded++
			if yielded >= limit {
				return
			}
		}
	}
}

// Single returns the single value in the iterator. It returns an error if the iterator yields
// zero or more than one result.
func (s SearchResult[E, C]) Single() (E, C, error) {
	var re E
	var rc C
	count := 0
	for e, c := range s {
		if count == 1 {
			return re, rc, ErrSingleMultipleResult
		}
		re, rc = e, c
		count++
	}
	if count == 0 {
		return re, rc, ErrSingleNoResult
	}
	return re, rc, nil
}
