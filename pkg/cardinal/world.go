// Package cardinal runs a World Engine game shard. A World holds the game's ECS state and runs its
// registered systems once per tick. It accepts commands from clients, delivers events to them,
// exchanges commands with other shards, and snapshots its state. Tests drive it with TestWorld,
// RunDST and RunE2E.
package cardinal

import (
	"context"
	"fmt"
	"iter"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/plugin"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const addressService = ":8080"

// World contains the game state and is the main Cardinal API.
type World struct {
	world           *ecs.World            // ECS state and systems
	commands        command.Manager       // Commands for systems
	events          event.Manager         // Events and event handlers
	address         *micro.ServiceAddress // NATS address
	service         *service              // ConnectRPC client service
	snapshotStorage snapshot.Storage      // Snapshot reader
	snapshotWriter  snapshot.Writer       // Snapshot writer
	debug           *debugModule          // Debug tools and services
	currentTick     Tick                  // Current tick
	traceCtx        context.Context       // Trace context of the running system, else of the current tick or init
	options         WorldOptions          // World options
	tel             telemetry.Telemetry   // Logs and traces
	metrics         *worldMetrics         // Tick, command and event metrics; nil records nothing

	archetypes map[reflect.Type]bitmap.Bitmap      // Component sets resolved from archetype structs
	eventTypes map[reflect.Type][]metric.AddOption // Registered events, with the attributes they are counted under
	started    bool                                // Set by init; Register* methods panic afterwards
	hooks      *plugin.Runner                      // Hooks the tick runs; nil until something registers one

	sync           *syncPublisher               // Sync state the tick publishes; nil until a sync type is registered
	commandSenders map[string]syncstate.Senders // Command name -> who may send it; absent: anyone
	judged         commandMemory                // Verdicts on players' recent command IDs; nil until the first tick
	admitted       map[string]bool              // EnableSync's verdict on each sender judged this tick
	systemEventTap func(ecs.SystemEvent)        // Sees each emitted system event; set only by TestWorld
}

// NewWorld creates a game world with the specified options.
func NewWorld(opts WorldOptions) (*World, error) {
	// Read and validate options.
	envs, err := loadWorldOptionsEnv()
	if err != nil {
		return nil, eris.Wrap(err, "failed to load world options env vars")
	}
	options := newDefaultWorldOptions()
	options.apply(envs.toOptions())
	options.apply(opts)
	if err := options.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid world options")
	}

	// Initialize telemetry.
	tel, err := telemetry.New(telemetry.Options{
		ServiceName: "cardinal",
		SentryOptions: sentry.Options{
			Tags: options.getSentryTags(),
		},
		PosthogOptions: posthog.Options{
			DistinctID:     options.Project,
			BaseProperties: options.getPosthogBaseProperties(),
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize telemetry")
	}
	defer tel.RecoverAndFlush(true)

	metrics, err := newWorldMetrics(options.tickInterval())
	if err != nil {
		return nil, err
	}

	world := &World{
		world:       ecs.NewWorld(),
		commands:    command.NewManager(),
		events:      event.NewManager(1024),
		address:     micro.GetAddress(options.Project, options.ShardID),
		currentTick: Tick{height: 0},
		traceCtx:    context.Background(),
		options:     options,
		tel:         tel,
		metrics:     metrics,
	}

	// Register components for introspection.
	world.world.OnComponentRegister(func(zero ecs.Component) error {
		return world.debug.register(introspect.Component, zero)
	})

	// Create the ConnectRPC client service.
	world.service = newService(world, options.AuthMode, options.ArgusAuthURL)

	// Connect event handlers to service publishers.
	world.events.RegisterHandler(event.KindDefault, world.service.publishDefaultEvent)
	world.events.RegisterHandler(event.KindInterShardCommand, world.service.publishInterShardCommand)

	// Initialize snapshot storage.
	switch options.SnapshotStorageType {
	case snapshot.StorageTypeJetStream:
		snapshotJS, err := snapshot.NewJetStreamStorage(snapshot.JetStreamStorageOptions{
			Logger:     tel.GetLogger("snapshot"),
			Address:    world.address,
			NATSConfig: options.NATSConfig,
		})
		if err != nil {
			return nil, eris.Wrap(err, "failed to create jetstream snapshot storage")
		}
		world.snapshotStorage = snapshotJS
	case snapshot.StorageTypeS3:
		snapshotS3, err := snapshot.NewS3Storage(snapshot.S3StorageOptions{
			Logger:  tel.GetLogger("snapshot"),
			Address: world.address,
		})
		if err != nil {
			return nil, eris.Wrap(err, "failed to create S3 snapshot storage")
		}
		world.snapshotStorage = snapshotS3
	case snapshot.StorageTypeNop:
		world.snapshotStorage = snapshot.NewNopStorage()
	case snapshot.StorageTypeUndefined:
		fallthrough
	default:
		panic("unreachable")
	}

	// Write snapshots synchronously until StartGame starts the asynchronous writer.
	world.snapshotWriter = snapshot.NewSyncWriter(world.snapshotStorage, tel.GetLogger("snapshot"))

	// Create the optional debug module.
	if *options.Debug {
		world.debug = newDebugModule(world)
	}

	return world, nil
}

// StartGame runs the game until the process receives a stop signal.
func (w *World) StartGame() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Move snapshot writes off the tick goroutine.
	w.snapshotWriter.Stop(context.Background())
	w.snapshotWriter = snapshot.NewAsyncWriter(w.snapshotStorage, &w.tel)

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	// Start the NATS connection and ConnectRPC service.
	if err := w.service.init(addressService); err != nil {
		panic(eris.Wrap(err, "failed to initialize service"))
	}

	w.tel.CaptureEvent(ctx, "Start Game", nil)

	if err := w.run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running world")
	}
}

func (w *World) restore(ctx context.Context) (err error) {
	ctx, span := trace.New(ctx, spanRestore)
	defer func() { span.EndWithErr(err) }()

	logger := w.tel.GetLogger("snapshot")

	logger.Debug().Msg("restoring from snapshot")
	data, err := w.snapshotStorage.Load(ctx)
	if err != nil {
		if eris.Is(err, snapshot.ErrSnapshotNotFound) {
			logger.Debug().Msg("no snapshot found")
			return nil
		}
		return eris.Wrap(err, "failed to load snapshot")
	}

	// Decode validates the bytes and refuses versions this build cannot read.
	snap, err := snapshot.Decode(data)
	if err != nil {
		return eris.Wrap(err, "refusing to restore snapshot")
	}

	if err := w.world.FromProto(snap.GetWorldState()); err != nil {
		return eris.Wrap(err, "failed to restore world from snapshot")
	}

	// Update the tick only after a successful restore.
	w.currentTick.height = snap.GetTickHeight() + 1

	// Publish the restored state for GetState; a no-op when the debug service is disabled.
	w.debug.publishState(data)
	return nil
}

// shutdown writes the final state and stops world services.
func (w *World) shutdown() {
	// Give all shutdown steps one shared timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down world")

	// Finish all snapshot writes before the service stops.
	if err := w.snapshotWriter.Drain(ctx); err != nil {
		w.tel.Logger.Error().Err(err).
			Msg("snapshot writes did not finish before shutdown; the last snapshot of this run may be lost")
	}
	w.snapshotWriter.Stop(ctx)

	// Drain queued commands and events.
	if err := w.service.shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("service shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	// Stop telemetry last so it can send all shutdown logs.
	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("World shutdown complete")
}

func (w *World) reset() {
	// Reset the ECS world.
	w.world.Reset()

	// Clear pending commands and events from before the reset. Init systems may
	// Broadcast/SendTo client-facing events that must survive reset and be delivered
	// on the next tick, matching cold-start behavior, so the clears run before init.
	w.commands.Clear()
	w.events.Clear()

	// Run initialization systems again. Sync publishes the replaced lifetimes on the next tick.
	w.init()

	// Reset the tick.
	w.currentTick.height = 0
	w.currentTick.timestamp = time.Time{}

	// Publish the reset state when the debug service is enabled.
	if w.debug != nil {
		w.debug.publishState(w.encodeSnapshot(0, w.currentTick.timestamp))
	}
}

// useSyncSnapshotStorage swaps the world's snapshot storage and installs a synchronous writer backed
// by it, so a test observes every Write immediately. It stops any writer the world already holds.
func (w *World) useSyncSnapshotStorage(store snapshot.Storage) {
	if w.snapshotWriter != nil {
		w.snapshotWriter.Stop(context.Background())
	}
	w.snapshotStorage = store
	w.snapshotWriter = snapshot.NewSyncWriter(store, w.tel.GetLogger("snapshot"))
}

type Tick struct {
	height    uint64
	timestamp time.Time
}

// -------------------------------------------------------------------------------------------------
// Plugins
// -------------------------------------------------------------------------------------------------

// RegisterPlugin registers a plugin with the world. Must be called before StartGame().
// Panics if the plugin fails to register, consistent with other registration functions.
func (w *World) RegisterPlugin(p Plugin) {
	if w.started {
		panic(ErrWorldStarted)
	}
	p.Register(w)
}

// -------------------------------------------------------------------------------------------------
// Systems
// -------------------------------------------------------------------------------------------------

// RegisterSystem registers a system for the hook in opts (Update by default). Register the
// components, commands, events, and system events a system uses before StartGame.
func (w *World) RegisterSystem(s System, opts ...SystemOption) {
	if w.started {
		panic(ErrWorldStarted)
	}
	// Reject a nil interface or typed nil pointer. Both would register under a valid name and then
	// dereference nil inside Run on the first tick, so registration rejects them while the caller
	// can still see which system it was.
	if v := reflect.ValueOf(s); s == nil || (v.Kind() == reflect.Pointer && v.IsNil()) {
		panic(eris.Errorf("system %T is nil; register a constructed instance", s))
	}
	cfg := systemConfig{phase: Update}
	for _, opt := range opts {
		opt(&cfg)
	}
	name := fmt.Sprintf("%T", s)
	phaseName := phaseToProto(cfg.phase).String()

	// The attributes are fixed per system, so they are built once here.
	attrs := oteltrace.WithAttributes(attrSystemName.String(name), attrSystemHook.String(phaseName))
	fn := func() { w.runSystem(s, name, attrs) }

	if err := w.world.RegisterSystem(name, cfg.phase, fn); err != nil {
		panic(eris.Wrapf(err, "error registering system"))
	}
}

// runSystem runs s as a child span of the current tick (or init) span, named after the system so
// trace views and per-span latency tell systems apart, and which s sees through Context while it
// runs. When the tick span is not recording (tracing disabled or the tick sampled out) the child
// would be discarded anyway, so it is skipped to keep the per-system cost at one interface call.
func (w *World) runSystem(s System, name string, attrs oteltrace.SpanStartEventOption) {
	if tickCtx := w.traceCtx; oteltrace.SpanFromContext(tickCtx).IsRecording() {
		ctx, span := trace.New(tickCtx, name, attrs)
		w.traceCtx = ctx
		defer func() { w.traceCtx = tickCtx }()
		defer span.End()
	}
	s.Run(w)
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

// Context returns the trace context of the running system. Spans started from it nest under the
// system's span in the tick's trace:
//
//	ctx, span := otel.Tracer("mygame").Start(w.Context(), "pathfind")
//	defer span.End()
//
// Pass it to instrumented clients (HTTP, SQL) so their spans join the tick's trace too. It carries
// trace context only: it is never cancelled and has no deadline. When tracing is disabled or the
// tick is sampled out, spans started from it are no-ops.
func (w *World) Context() context.Context {
	return w.traceCtx
}

// Logger returns the logger for systems in this world. In a traced system its entries carry the
// system span's trace_id and span_id, so a log line leads to the tick that wrote it.
func (w *World) Logger() *zerolog.Logger {
	logger := w.tel.GetLoggerWithTrace(w.traceCtx, "system")
	return &logger
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

// RegisterCommand registers a command type before world startup. Registering it again is a no-op.
// The service accepts a command from clients only once it is registered here.
//
// senders limits who may send it, such as sync.FromPlayers; without one, players and shards both
// may. A command that embeds sync.Target reaches handlers only when its sender has authority over
// the target. Cardinal replies to each player whose command it drops with sync.CommandRejected,
// and judges a player's repeat of a Target.CommandID only once.
func (w *World) RegisterCommand[T Command](senders ...syncstate.Senders) {
	if w.started {
		panic(ErrWorldStarted)
	}
	var zero T
	name := zero.Name()
	var allowed syncstate.Senders
	for _, s := range senders {
		allowed |= s
	}
	if w.commandSenders == nil {
		w.commandSenders = map[string]syncstate.Senders{}
	}
	if _, declared := w.commandSenders[name]; !declared {
		w.commandSenders[name] = allowed
	}
	w.RegisterEvent[syncstate.CommandRejected]()
	w.hookSync()

	if _, err := w.commands.Register(name, command.NewQueue[T]()); err != nil {
		panic(eris.Wrapf(err, "failed to register command %s", name))
	}
	if err := w.debug.register(introspect.Command, zero); err != nil {
		panic(eris.Wrapf(err, "failed to register command to debug module %s", name))
	}
	w.service.registerCommandHandler(name)
	w.metrics.registerCommand(name)
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
			// The queue stores the decoded value as a Payload; recover the concrete type. Value
			// semantics, no pointer: Serializable is satisfied by the value type.
			payload, isT := cmd.Payload.(T)
			assert.That(isT, "mismatched command type passed to command context")
			var target Entity
			if t, hasTarget := any(payload).(targeted); hasTarget {
				target = w.Entity(EntityID(t.SyncTarget().Entity))
			}
			if !yield(CommandContext[T]{Payload: payload, sender: cmd.Sender, target: target}) {
				return
			}
		}
	}
}

// SendToShard sends cmd to another shard, addressed by to. This is the first-class shard-to-shard send: the
// world performs the send (it owns the event queue) and the address `to` is plain data with no behavior of
// its own. It mirrors the client-facing SendCommand RPC — a shard sending to a shard is the same operation,
// initiated in-engine.
//
// Fire-and-forget: cmd is encoded when events flush at end-of-tick, so it must not be mutated after this
// call — a *Command whose fields change before the flush would send the mutated value. The send then
// happens in the background, in order per target shard. A send that fails is not returned but is logged
// at error level, because a dropped shard-to-shard command is serious.
func (w *World) SendToShard(to OtherWorld, cmd command.Payload) {
	if to.ShardID == "" {
		w.Logger().Error().Str("command", cmd.Name()).Msg("SendToShard: empty target shard address, dropping command")
		return
	}
	// cmd is not checked against the registry here: it is encoded with MessagePack when events flush,
	// so any command.Payload can be sent.
	serviceAddress := micro.GetAddress(to.Project, to.ShardID)
	w.events.Enqueue(event.Event{
		Kind: event.KindInterShardCommand,
		Payload: command.Command{
			Name:    cmd.Name(),
			Address: serviceAddress,
			Payload: cmd,
		},
	})
}

// -------------------------------------------------------------------------------------------------
// Events
// -------------------------------------------------------------------------------------------------

// RegisterEvent registers an event type before world startup so it appears in introspection
// metadata and can be sent. Registering it again is a no-op.
func (w *World) RegisterEvent[T Event]() {
	if w.started {
		panic(ErrWorldStarted)
	}
	var zero T
	if err := w.debug.register(introspect.Event, zero); err != nil {
		panic(eris.Wrapf(err, "failed to register event to debug module %s", zero.Name()))
	}
	if w.eventTypes == nil {
		w.eventTypes = make(map[reflect.Type][]metric.AddOption)
	}
	w.eventTypes[reflect.TypeFor[T]()] = addOptions(attrEventType.String(fmt.Sprintf("%T", zero)))
}

// registeredEvent returns the attributes T's sends are counted under, and panics if T was not
// registered with RegisterEvent. It checks the type, not Name(): some events derive their name from
// instance data (e.g. request-scoped results), so only the type is stable at registration. A plain
// panic, not assert.That, so it fires in release too.
func (w *World) registeredEvent[T Event]() []metric.AddOption {
	attrs, ok := w.eventTypes[reflect.TypeFor[T]()]
	if !ok {
		panic(eris.Errorf("event %T is not registered; call RegisterEvent before StartGame", *new(T)))
	}
	return attrs
}

// Broadcast enqueues an event delivered to every open event stream at the end of the tick. It
// panics if T was not registered with RegisterEvent.
func (w *World) Broadcast[T Event](evt T) {
	attrs := w.registeredEvent[T]()
	audience := w.hooks.Emit(plugin.Broadcast, &evt, "")
	w.events.Enqueue(event.Event{
		Kind:     event.KindDefault,
		Payload:  evt,
		Audience: audience,
	})
	w.metrics.countEvent(w.traceCtx, attrs)
}

// SendTo enqueues a targeted event that is delivered only to the named recipient (a player ID),
// provided they have an open event stream subscribed to this event or a pending SendCommandWithReply
// waiting for it. Otherwise the event is silently dropped. It panics if T was not registered with
// RegisterEvent.
//
// Example:
//
//	if player, ok := cmd.Player(); ok {
//		w.SendTo(player, Result{OK: true})
//	}
func (w *World) SendTo[T Event](recipient string, evt T) {
	assert.That(recipient != "", "recipient must not be empty (use Broadcast for fan-out)")
	attrs := w.registeredEvent[T]()
	audience := w.hooks.Emit(plugin.Targeted, &evt, recipient)
	w.events.Enqueue(event.Event{
		Kind:      event.KindDefault,
		Payload:   evt,
		Recipient: recipient,
		Audience:  audience,
	})
	w.metrics.countEvent(w.traceCtx, attrs)
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
	if w.started {
		panic(ErrWorldStarted)
	}
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
	w.hooks.Emit(plugin.SystemEvent, &systemEvent, "")
	if err := w.world.EmitSystemEvent(systemEvent); err != nil {
		panic(eris.Wrapf(err, "system event %T is not registered; call RegisterSystemEvent before StartGame",
			systemEvent))
	}
	if w.systemEventTap != nil {
		w.systemEventTap(systemEvent)
	}
}

// -------------------------------------------------------------------------------------------------
// Components
// -------------------------------------------------------------------------------------------------

// RegisterComponent registers a component type before world startup. Every component
// used by a system, archetype, or snapshot must be registered here; nothing registers
// components implicitly. Register components before StartGame.
//
// It panics if the component, or any field a snapshot stores, cannot be stored:
//   - a slice, map, pointer, or interface, which would share memory with the stored component;
//   - a chan, func, [unsafe.Pointer], method interface such as error, or non-empty struct with no
//     exported fields, which a snapshot cannot hold;
//   - a uintptr or complex number;
//   - a type with its own MessagePack codec whose encoding method has a pointer receiver, that
//     encodes but does not decode itself or the reverse, or that holds a reference;
//   - an embedded field that supplies the struct's codec, since Go promotes it and the whole struct
//     is then stored as that one value. Declare the struct's own codec, or give the field a name;
//   - an embedded field of an unexported type that is not a struct, such as a named int, which a
//     snapshot stores but a restore cannot write back.
//
// Hold a list in a fixed-size array or an immutable.Slice instead. Fields tagged msgpack:"-" are
// not stored, so they are not checked.
func (w *World) RegisterComponent[T ecs.Component]() {
	if w.started {
		panic(ErrWorldStarted)
	}
	cid, err := w.world.RegisterComponent[T]()
	if err != nil {
		panic(eris.Wrapf(err, "failed to register component %T", *new(T)))
	}
	var zero T
	w.registerSync(reflect.TypeFor[T](), zero.Name(), cid)
}

// archetype resolves the registered component IDs declared by T's fields and caches the
// result per type. The first call for a type walks the struct with reflection; later calls
// are one map lookup.
func (w *World) archetype[T any]() (bitmap.Bitmap, error) {
	typ := reflect.TypeFor[T]()
	if components, ok := w.archetypes[typ]; ok {
		return components, nil
	}
	if typ.Kind() != reflect.Struct {
		return nil, eris.Errorf("entity archetype must be a struct, got %v", typ)
	}
	var components bitmap.Bitmap
	// Ranging over typ.Fields() heap-allocates on every call, cache hits included
	// (TestEntity_SteadyStateAllocations).
	for i := range typ.NumField() { //nolint:modernize // see above
		field := typ.Field(i)
		component, ok := reflect.TypeAssert[ecs.Component](reflect.Zero(field.Type))
		if !ok || field.Type.Kind() != reflect.Struct {
			return nil, eris.Errorf("field %s of archetype %v must be a component struct, got %v",
				field.Name, typ, field.Type)
		}
		id, err := w.world.ComponentIDOf(component)
		if err != nil {
			return nil, eris.Wrapf(err, "cannot resolve component field %s of archetype %v", field.Name, typ)
		}
		components.Set(id)
	}
	if w.archetypes == nil {
		w.archetypes = make(map[reflect.Type]bitmap.Bitmap)
	}
	w.archetypes[typ] = components
	return components, nil
}

// -------------------------------------------------------------------------------------------------
// Entities
// -------------------------------------------------------------------------------------------------

// Entity binds a numeric ID to this world. It does not require the entity to exist.
// Use Alive or Has to check before accessing an optional entity.
func (w *World) Entity(id EntityID) Entity {
	return Entity{world: w.world, id: id}
}

// Create creates an entity with the zero-valued components declared by T, an archetype
// struct whose fields are component types. It panics if any of them was not registered
// with World.RegisterComponent.
func (w *World) Create[T any]() Entity {
	components, err := w.archetype[T]()
	if err != nil {
		panic(err)
	}
	return w.Entity(w.world.CreateWithArchetype(components))
}

// -------------------------------------------------------------------------------------------------
// Search
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
