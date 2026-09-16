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
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/performance"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

type EntityID = ecs.EntityID

// System is a stateful Cardinal system. Implement it with a Run method on a pointer to a struct
// that embeds BaseSystemState.
type System interface {
	Run()
}

func (w *World) RegisterSystem[T any](system func(*T), opts ...SystemOption) {
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

	if err := initSystemFields(reflect.ValueOf(state).Elem(), w); err != nil {
		panic(eris.Wrapf(err, "error initializing system fields"))
	}

	name := fmt.Sprintf("%T", system)
	registerSystem(w, name, cfg.hook, func() { system(state) })
}

// RegisterSystemV2 registers a caller-owned system instance. The instance must be a non-nil pointer
// to a struct that embeds BaseSystemState.
func (w *World) RegisterSystemV2[S System](s S, opts ...SystemOption) {
	cfg := newSystemConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	value := reflect.ValueOf(s)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		panic(eris.Errorf("system %T must be a non-nil pointer to a struct", s))
	}

	state := value.Elem()
	stateType := state.Type()
	if _, ok := stateType.MethodByName("Run"); ok {
		panic(eris.Errorf("system %T Run method must use a pointer receiver", s))
	}

	baseField, ok := stateType.FieldByName("BaseSystemState")
	if !ok || len(baseField.Index) != 1 || !baseField.Anonymous ||
		baseField.Type != reflect.TypeFor[BaseSystemState]() {
		panic(eris.Errorf("system %T must embed cardinal.BaseSystemState", s))
	}

	if err := initSystemFields(state, w); err != nil {
		panic(eris.Wrapf(err, "error initializing system fields"))
	}

	registerSystem(w, fmt.Sprintf("%T", s), cfg.hook, s.Run)
}

func registerSystem(w *World, name string, hook SystemHook, run func()) {
	fn := run

	// If debug is enabled, wrap the system with performance instrumentation.
	if w.debug != nil {
		fn = func() {
			ts := w.currentTick.timestamp
			startTime := ts.Add(time.Since(ts))
			run()
			endTime := ts.Add(time.Since(ts))
			w.debug.recordSpan(performance.TickSpan{
				TickHeight: w.currentTick.height,
				SystemName: name,
				SystemHook: uint8(hook),
				StartTime:  startTime,
				EndTime:    endTime,
			})
		}
	}

	err := w.world.RegisterSystem(name, hook, fn)
	if err != nil {
		panic(eris.Wrapf(err, "error registering system"))
	}
}

func initSystemFields(state reflect.Value, w *World) error {
	meta := systemInitMetadata{
		world:        w,
		commands:     make(map[string]struct{}),
		events:       make(map[string]struct{}),
		systemEvents: make(map[string]struct{}),
	}

	// For each field in the system state, initialize the field and collect its dependencies.
	for i := range state.NumField() {
		field := state.Field(i)
		fieldType := state.Type().Field(i)

		if !fieldType.IsExported() {
			systemFieldType := reflect.TypeFor[systemField]()
			if field.Type().Implements(systemFieldType) ||
				field.Addr().Type().Implements(systemFieldType) {
				return eris.Errorf("field %s must be exported", fieldType.Name)
			}
			continue
		}

		if field.Type().Implements(reflect.TypeFor[systemField]()) {
			return eris.Errorf("field %s must be declared as a value", fieldType.Name)
		}

		fieldInstance := field.Addr().Interface()

		cardinalField, ok := fieldInstance.(systemField)
		if ok {
			if err := cardinalField.init(&meta); err != nil {
				return eris.Wrapf(err, "failed to initialize field %s", fieldType.Name)
			}
		}
		// For now we'll ignore other fields in the system state struct.
	}

	// Register commands to the service.
	for name := range meta.commands {
		w.service.registerCommandHandler(name)
	}

	return nil
}

type systemInitMetadata struct {
	world        *World
	commands     map[string]struct{}
	events       map[string]struct{}
	systemEvents map[string]struct{}
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
	// No codec check: T is constrained to Command (schema.Serializable), so an ungenerated command —
	// one missing its generated wire methods — does not satisfy the constraint and fails to compile
	// here. There is no codec registry to consult.
	var zero T
	name := zero.Name()

	if _, ok := meta.commands[name]; ok {
		return eris.Errorf("systems cannot process multiple commands of the same type: %s", name)
	}

	id, err := meta.world.commands.Register(name, command.NewQueue[T]())
	if err != nil {
		return eris.Wrapf(err, "failed to register command %s", name)
	}

	if err := meta.world.debug.register(introspect.Command, zero); err != nil {
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
	// The queue stores the decoded value as a Payload; recover the concrete type. Value semantics —
	// no pointer, because Serializable is satisfied by the value type (all value receivers).
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

// SendToShard sends cmd to another shard, addressed by to. This is the first-class shard-to-shard send: the
// world performs the send (it owns the event queue) and the address `to` is plain data with no behavior of
// its own. It mirrors the client-facing SendCommand RPC — a shard sending to a shard is the same operation,
// initiated in-engine.
//
// Fire-and-forget: the actual network send happens when events flush at end-of-tick, so cmd must not be
// mutated after this call — a *Command whose fields change before the flush would send the mutated value.
// A send that fails is not returned (it must not block the tick) but is logged at error level, because a
// dropped shard-to-shard command is serious.
func (b *BaseSystemState) SendToShard(to OtherWorld, cmd command.Payload) {
	if to.ShardID == "" {
		b.Logger().Error().Str("command", cmd.Name()).Msg("SendToShard: empty target shard address, dropping command")
		return
	}
	// cmd is a command.Payload, so it carries its generated MarshalWire — no registry check needed; an
	// ungenerated command wouldn't satisfy the parameter type and wouldn't compile at the call site.
	serviceAddress := micro.GetAddress(to.Region, micro.RealmWorld, to.Organization, to.Project, to.ShardID)
	b.world.events.Enqueue(event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    cmd.Name(),
			Persona: micro.String(b.world.address),
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

	if err := meta.world.debug.register(introspect.Event, zero); err != nil {
		return eris.Wrapf(err, "failed to register event to debug module %s", name)
	}

	meta.events[name] = struct{}{} // Add to system events set for duplicate field check

	e.manager = &meta.world.events
	return nil
}

func (e *WithEvent[T]) Broadcast(evt T) {
	e.manager.Enqueue(event.Event{
		Kind:    event.KindDefault,
		Payload: evt,
	})
}

// SendTo enqueues a targeted event that is delivered only to the named recipient (a user ID),
// provided they have an open event stream subscribed to this event. If the recipient has no open
// stream, the event is silently dropped.
//
// Example:
//
//	state.Results.SendTo(cmdCtx.Persona, Result{OK: true})
func (e *WithEvent[T]) SendTo(recipient string, evt T) {
	assert.That(recipient != "", "recipient must not be empty (use Broadcast for fan-out)")
	e.manager.Enqueue(event.Event{
		Kind:      event.KindDefault,
		Payload:   evt,
		Recipient: recipient,
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

	_, err := meta.world.world.RegisterSystemEvent[T]()
	if err != nil {
		return eris.Wrapf(err, "failed to register system event %s", name)
	}
	s.world = meta.world.world

	meta.systemEvents[name] = struct{}{} // Add to system's system events set for duplicate field check
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
	systemEvents, err := s.world.GetSystemEvents[T]()
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

	_, err := meta.world.world.RegisterSystemEvent[T]()
	if err != nil {
		return eris.Wrapf(err, "failed to register system event %s", name)
	}
	s.world = meta.world.world

	meta.systemEvents[name] = struct{}{} // Add to system's system events set for duplicate field check
	return nil
}

// Emit emits a system event of type T.
//
// Example:
//
//	state.PlayerDeathEvents.Emit(PlayerDeath{Nickname: "Player1"})
func (s *WithSystemEventEmitter[T]) Emit(systemEvent T) {
	err := s.world.EmitSystemEvent(systemEvent)
	assert.That(err == nil, "tried to emit unregistered system event")
}

// -------------------------------------------------------------------------------------------------
// Components
// -------------------------------------------------------------------------------------------------

// search caches an archetype registered during system initialization.
type search[T any] struct {
	world      *ecs.World
	components bitmap.Bitmap
}

func (s *search[T]) init(meta *systemInitMetadata) error {
	components, err := meta.world.registerArchetype[T]()
	if err != nil {
		return err
	}
	s.world, s.components = meta.world.world, components
	return nil
}

func (s *search[T]) getByID(eid EntityID, match ecs.SearchMatch) (Entity, error) {
	if err := s.world.MatchArchetype(eid, s.components, match); err != nil {
		return Entity{}, eris.Wrap(err, "failed to get entity")
	}
	return Entity{world: s.world, id: eid}, nil
}

func (s *search[T]) iter(match ecs.SearchMatch) SearchResult {
	return func(yield func(Entity) bool) {
		err := s.world.IterEntities(s.components, match, func(eid EntityID) bool {
			return yield(Entity{world: s.world, id: eid})
		})
		assert.That(err == nil, "invalid arguments sent to IterEntities")
	}
}

// Create returns a new entity with the components declared in T, initialized to zero.
func (s *search[T]) Create() Entity {
	eid := s.world.CreateWithArchetype(s.components)
	return Entity{world: s.world, id: eid}
}

// Contains matches entities with all components declared in T, allowing extras.
// T is a struct of WithComponent[C] fields, registered before the world starts.
type Contains[T any] struct{ search[T] }

// Iter yields each matching entity as a world-bound handle.
func (c *Contains[T]) Iter() SearchResult { return c.iter(ecs.MatchContains) }

// GetByID returns a handle if the entity contains every declared component.
func (c *Contains[T]) GetByID(eid EntityID) (Entity, error) {
	return c.getByID(eid, ecs.MatchContains)
}

// Exact matches entities with exactly the components declared in T.
// T is a struct of WithComponent[C] fields, registered before the world starts.
type Exact[T any] struct{ search[T] }

// Iter yields each matching entity as a world-bound handle.
func (c *Exact[T]) Iter() SearchResult { return c.iter(ecs.MatchExact) }

// GetByID returns a handle if the entity has exactly the declared components.
func (c *Exact[T]) GetByID(eid EntityID) (Entity, error) {
	return c.getByID(eid, ecs.MatchExact)
}

// -------------------------------------------------------------------------------------------------
// Component Search Result Modifiers
// -------------------------------------------------------------------------------------------------

var (
	ErrSingleNoResult       = eris.New("expected exactly 1 result, got 0")
	ErrSingleMultipleResult = eris.New("expected exactly 1 result, got more than 1")
)

// SearchResult is a chainable iterator over entities.
type SearchResult func(yield func(Entity) bool)

// Filter returns a new iterator that only yields values that satisfy predicate. A nil predicate
// returns the original iterator unchanged.
func (s SearchResult) Filter(predicate func(Entity) bool) SearchResult {
	if predicate == nil {
		return s
	}

	return func(yield func(Entity) bool) {
		s(func(c Entity) bool {
			return !predicate(c) || yield(c)
		})
	}
}

// Limit returns a new iterator that yields at most limit values. A limit <= 0 yields no values.
func (s SearchResult) Limit(limit uint32) SearchResult {
	return func(yield func(Entity) bool) {
		if limit == 0 {
			return
		}
		yielded := uint32(0)
		s(func(c Entity) bool {
			yielded++
			return yield(c) && yielded < limit
		})
	}
}

// Single returns the single value in the iterator. It returns an error if the iterator yields
// zero or more than one result.
func (s SearchResult) Single() (Entity, error) {
	// A function-valued iterator can retain its callback. Keep the captured result
	// together so escape analysis needs one record rather than separate captured allocations.
	result := struct {
		entity Entity
		err    error
	}{err: ErrSingleNoResult}
	s(func(c Entity) bool {
		if result.err == nil {
			result.err = ErrSingleMultipleResult
			return false
		}
		result.entity, result.err = c, nil
		return true
	})
	return result.entity, result.err
}

// -------------------------------------------------------------------------------------------------
// Re-exported immutable types
// -------------------------------------------------------------------------------------------------

// Slice is world-engine's sequence with a private backing array, re-exported so a component can
// declare one without a second import. It is an alias, so cardinal.Slice[T] and immutable.Slice[T]
// are the same type and either spelling works.
//
// Its derivations edit in place; see immutable.Slice for what that means at a call site.
type Slice[T any] = immutable.Slice[T]

// SliceOf returns a Slice holding a copy of items. See immutable.SliceOf.
func SliceOf[T any](items ...T) Slice[T] { return immutable.SliceOf(items...) }
