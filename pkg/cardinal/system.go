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
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type EntityID = ecs.EntityID

// System is a unit of game logic that runs once per tick phase. Run receives the world it was
// registered with; use it for searches, commands, events, entities, and the current tick. Any
// other state a system needs (a runtime, a config) lives on the implementing struct.
type System interface {
	Run(w *World)
}

// RegisterSystem registers a system for the hook in opts (Update by default). Register the
// components, commands, events, and system events a system uses before StartGame.
func (w *World) RegisterSystem(s System, opts ...SystemOption) {
	if isNilSystem(s) {
		panic(eris.Errorf("system %T is nil; register a constructed instance", s))
	}
	cfg := newSystemConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	registerSystem(w, fmt.Sprintf("%T", s), cfg.hook, func() { s.Run(w) })
}

// isNilSystem reports whether s is a nil interface or a typed nil pointer. Both would register
// under a valid name and then dereference nil inside Run on the first tick, so registration
// rejects them while the caller can still see which system it was.
func isNilSystem(s System) bool {
	if s == nil {
		return true
	}
	v := reflect.ValueOf(s)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func registerSystem(w *World, name string, hook SystemHook, run func()) {
	hookName := ecsHookToProto(uint8(hook)).String()

	// Every system run is a child span of the current tick (or init) span. The attributes are
	// fixed per system, so they are built once here. When the tick span is not recording
	// (tracing disabled or the tick sampled out) the child would be discarded anyway, so it is
	// skipped to keep the per-system cost at one interface call.
	attrs := oteltrace.WithAttributes(attrSystemName.String(name), attrSystemHook.String(hookName))
	fn := func() {
		if oteltrace.SpanFromContext(w.tickCtx).IsRecording() {
			_, span := trace.New(w.tickCtx, spanSystem, attrs)
			defer span.End()
		}
		run()
	}

	// If debug is enabled, also record the run in the performance module.
	if w.debug != nil {
		traced := fn
		fn = func() {
			ts := w.currentTick.timestamp
			startTime := ts.Add(time.Since(ts))
			traced()
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
// Tick
// -------------------------------------------------------------------------------------------------

// TickHeight returns the height of the tick currently running.
func (w *World) TickHeight() uint64 {
	return w.currentTick.height
}

// Timestamp returns the timestamp of the tick currently running.
func (w *World) Timestamp() time.Time {
	return w.currentTick.timestamp
}

// Logger returns the logger for systems in this world.
func (w *World) Logger() *zerolog.Logger {
	logger := w.tel.GetLogger("system")
	return &logger
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

type Command = command.Payload

// RegisterCommand registers a command type before world startup. Registering it again is a no-op.
// The service accepts a command from clients only once it is registered here.
func (w *World) RegisterCommand[T Command]() {
	// No codec check: T is constrained to Command (schema.Serializable), so an ungenerated command —
	// one missing its generated wire methods — does not satisfy the constraint and fails to compile
	// here. There is no codec registry to consult.
	var zero T
	name := zero.Name()

	if _, err := w.commands.Register(name, command.NewQueue[T]()); err != nil {
		panic(eris.Wrapf(err, "failed to register command %s", name))
	}
	if err := w.debug.register(introspect.Command, zero); err != nil {
		panic(eris.Wrapf(err, "failed to register command to debug module %s", name))
	}
	w.service.registerCommandHandler(name)
}

// Commands yields the commands of type T received for the current tick. It panics if T was not
// registered with RegisterCommand. Like ErrWorldStarted this is a plain panic, not assert.That:
// registration is explicit, so a missing RegisterCommand is a user error that must also fail in
// release builds instead of reading another command's queue.
func (w *World) Commands[T Command]() iter.Seq[CommandContext[T]] {
	var zero T
	id, ok := w.commands.Lookup(zero.Name())
	if !ok {
		panic(eris.Errorf("command %s is not registered; call RegisterCommand before StartGame", zero.Name()))
	}
	commands, err := w.commands.Get(id)
	assert.That(err == nil, "command %s has no queue", zero.Name())

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
func (w *World) SendToShard(to OtherWorld, cmd command.Payload) {
	if to.ShardID == "" {
		w.Logger().Error().Str("command", cmd.Name()).Msg("SendToShard: empty target shard address, dropping command")
		return
	}
	// cmd is a command.Payload, so it carries its generated MarshalWire — no registry check needed; an
	// ungenerated command wouldn't satisfy the parameter type and wouldn't compile at the call site.
	serviceAddress := micro.GetAddress(to.Region, micro.RealmWorld, to.Organization, to.Project, to.ShardID)
	w.events.Enqueue(event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    cmd.Name(),
			Persona: micro.String(w.address),
			Address: serviceAddress,
			Payload: cmd,
		},
	})
}

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

type Event = event.Payload

// RegisterEvent registers an event type before world startup so it appears in introspection
// metadata and can be sent. Registering it again is a no-op.
func (w *World) RegisterEvent[T Event]() {
	var zero T
	if err := w.debug.register(introspect.Event, zero); err != nil {
		panic(eris.Wrapf(err, "failed to register event to debug module %s", zero.Name()))
	}
	if w.eventTypes == nil {
		w.eventTypes = make(map[reflect.Type]struct{})
	}
	w.eventTypes[reflect.TypeFor[T]()] = struct{}{}
}

// checkEventRegistered panics if T was not registered with RegisterEvent. It checks the type, not
// Name(): some events derive their name from instance data (e.g. request-scoped results), so only
// the type is stable at registration. A plain panic, not assert.That, so it fires in release too.
func (w *World) checkEventRegistered[T Event]() {
	if _, ok := w.eventTypes[reflect.TypeFor[T]()]; !ok {
		panic(eris.Errorf("event %T is not registered; call RegisterEvent before StartGame", *new(T)))
	}
}

// Broadcast enqueues an event delivered to every open event stream at the end of the tick. It
// panics if T was not registered with RegisterEvent.
func (w *World) Broadcast[T Event](evt T) {
	w.checkEventRegistered[T]()
	w.events.Enqueue(event.Event{
		Kind:    event.KindDefault,
		Payload: evt,
	})
}

// SendTo enqueues a targeted event that is delivered only to the named recipient (a user ID),
// provided they have an open event stream subscribed to this event. If the recipient has no open
// stream, the event is silently dropped. It panics if T was not registered with RegisterEvent.
//
// Example:
//
//	w.SendTo(cmd.Persona, Result{OK: true})
func (w *World) SendTo[T Event](recipient string, evt T) {
	assert.That(recipient != "", "recipient must not be empty (use Broadcast for fan-out)")
	w.checkEventRegistered[T]()
	w.events.Enqueue(event.Event{
		Kind:      event.KindDefault,
		Payload:   evt,
		Recipient: recipient,
	})
}

// -------------------------------------------------------------------------------------------------
// System Events
// -------------------------------------------------------------------------------------------------

// RegisterSystemEvent registers a system event type before world startup. System events carry
// data between systems within one tick. Registering it again is a no-op.
//
// Example:
//
//	// Define a system event for player deaths.
//	type PlayerDeath struct{ Nickname string }
//
//	func (PlayerDeath) Name() string { return "player-death" }
//
//	w.RegisterSystemEvent[PlayerDeath]()
//
//	// One system emits it.
//	func (s *CombatSystem) Run(w *cardinal.World) {
//	    w.EmitSystemEvent(PlayerDeath{Nickname: "Player1"})
//	}
//
//	// Another system, later in the tick, receives it.
//	func (s *GraveyardSystem) Run(w *cardinal.World) {
//	    for death := range w.SystemEvents[PlayerDeath]() {
//	        // Process the system event.
//	    }
//	}
func (w *World) RegisterSystemEvent[T ecs.SystemEvent]() {
	var zero T
	if _, err := w.world.RegisterSystemEvent[T](); err != nil {
		panic(eris.Wrapf(err, "failed to register system event %s", zero.Name()))
	}
}

// SystemEvents yields the system events of type T emitted so far in the current tick. It panics if
// T was not registered with RegisterSystemEvent, in release builds too.
func (w *World) SystemEvents[T ecs.SystemEvent]() iter.Seq[T] {
	systemEvents, err := w.world.GetSystemEvents[T]()
	if err != nil {
		panic(eris.Wrapf(err, "system event %T is not registered; call RegisterSystemEvent before StartGame",
			*new(T)))
	}

	return func(yield func(T) bool) {
		for _, systemEvent := range systemEvents {
			if !yield(systemEvent) {
				return
			}
		}
	}
}

// EmitSystemEvent emits a system event for systems later in the tick. It panics if T was not
// registered with RegisterSystemEvent, in release builds too: an unregistered emit dropped
// silently would leave every later SystemEvents reader empty.
func (w *World) EmitSystemEvent[T ecs.SystemEvent](systemEvent T) {
	if err := w.world.EmitSystemEvent(systemEvent); err != nil {
		panic(eris.Wrapf(err, "system event %T is not registered; call RegisterSystemEvent before StartGame",
			systemEvent))
	}
}

// -------------------------------------------------------------------------------------------------
// Components
// -------------------------------------------------------------------------------------------------

// Contains returns a search for entities with every component in T, allowing extras. T is an
// archetype: a struct whose fields are registered component types. It panics if any component in
// T was not registered with RegisterComponent.
func (w *World) Contains[T any]() Search {
	return w.search[T](ecs.MatchContains)
}

// Exact returns a search for entities with exactly the components in T. T is an archetype: a
// struct whose fields are registered component types. It panics if any component in T was not
// registered with RegisterComponent.
func (w *World) Exact[T any]() Search {
	return w.search[T](ecs.MatchExact)
}

func (w *World) search[T any](match ecs.SearchMatch) Search {
	components, err := w.archetype[T]()
	if err != nil {
		panic(err)
	}
	return Search{world: w.world, components: components, match: match}
}

// Search is a resolved, world-bound query returned by World.Contains and World.Exact.
// It holds component IDs, not entity data, so it remains valid as entities change.
type Search struct {
	world      *ecs.World
	components bitmap.Bitmap
	match      ecs.SearchMatch
}

// Iter yields each matching entity as a world-bound handle.
func (q Search) Iter() SearchResult {
	return func(yield func(Entity) bool) {
		err := q.world.IterEntities(q.components, q.match, func(eid EntityID) bool {
			return yield(Entity{world: q.world, id: eid})
		})
		assert.That(err == nil, "invalid arguments sent to IterEntities")
	}
}

// GetByID returns a handle if the entity matches the query's archetype.
func (q Search) GetByID(eid EntityID) (Entity, error) {
	if err := q.world.MatchArchetype(eid, q.components, q.match); err != nil {
		return Entity{}, eris.Wrap(err, "failed to get entity")
	}
	return Entity{world: q.world, id: eid}, nil
}

// Create returns a new entity with the query's components, initialized to zero.
func (q Search) Create() Entity {
	return Entity{world: q.world, id: q.world.CreateWithArchetype(q.components)}
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
