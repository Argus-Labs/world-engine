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
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
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
	tickCtx         context.Context       // Parent context for spans started by systems in the current tick
	options         WorldOptions          // World options
	tel             telemetry.Telemetry   // Logs and traces

	archetypes map[reflect.Type]bitmap.Bitmap // Component sets resolved from archetype structs
	eventTypes map[reflect.Type]struct{}      // Events registered with RegisterEvent
	started    bool                           // Set by init; Register* methods panic afterwards

	systemEventTap func(ecs.SystemEvent) // Sees each emitted system event; set only by TestWorld
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
			DistinctID:     options.Organization,
			BaseProperties: options.getPosthogBaseProperties(),
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize telemetry")
	}
	defer tel.RecoverAndFlush(true)

	world := &World{
		world:    ecs.NewWorld(),
		commands: command.NewManager(),
		events:   event.NewManager(1024),
		address: micro.GetAddress(
			options.Region, micro.RealmWorld, options.Organization, options.Project, options.ShardID),
		currentTick: Tick{height: 0},
		tickCtx:     context.Background(),
		options:     options,
		tel:         tel,
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

func (w *World) run(ctx context.Context) error {
	// Initialize the world and run initialization systems.
	w.init()

	if err := w.restore(ctx); err != nil {
		return eris.Wrap(err, "failed to restore state from snapshot")
	}
	// Final snapshot.
	defer func() {
		w.snapshotWriter.Write(w.currentTick.height, w.encodeSnapshot(time.Now()))
	}()

	logger := w.tel.GetLogger("shard")
	logger.Info().Msg("starting core shard loop")

	ticker := time.NewTicker(time.Duration(float64(time.Second) / w.options.TickRate))
	defer ticker.Stop()

	for {
		if w.debug.isPaused() {
			select {
			case <-w.debug.resumeChan():
				w.debug.setPaused(false)
			case replyCh := <-w.debug.stepChan():
				w.Tick(time.Now())
				replyCh <- w.currentTick.height
			case replyCh := <-w.debug.resetChan():
				w.reset()
				replyCh <- struct{}{}
			case <-ctx.Done():
				return ctx.Err()
			}
			continue
		}

		select {
		case <-ticker.C:
			w.Tick(time.Now())
		case replyCh := <-w.debug.pauseChan():
			w.debug.setPaused(true)
			replyCh <- w.currentTick.height
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// init runs the init systems under a root span so their child spans have a parent. tickCtx is
// restored by defer, as in Tick, so a recovered init panic does not leave it on an ended span.
func (w *World) init() {
	ctx, span := trace.New(context.Background(), spanInit)
	defer span.End()

	w.tickCtx = ctx
	defer func() { w.tickCtx = context.Background() }()
	w.started = true // Closes registration before the first system runs; never reopened by reset.
	w.world.Init()
}

// Tick advances the world by one step, running every PreUpdate, Update and PostUpdate system.
func (w *World) Tick(timestamp time.Time) {
	w.step(timestamp, w.world.Tick)
}

// step is the frame around one tick. It drains commands, opens the tick span, calls run to advance
// the ECS, then dispatches events, persists state and increments the height. Tick passes the full
// schedule; TestWorld.RunSystem passes one system.
//
// Each tick is a root trace: ticks are driven by the clock, not by a request, so command spans from
// the ConnectRPC service are not their parents.
func (w *World) step(timestamp time.Time, run func()) {
	// Drain before starting the span: links must be passed at start for a sampler to see them.
	commands := w.commands.Drain()

	// Link each drained command whose enqueuing request was sampled, up to maxCommandLinks. Links,
	// not children: that request finished before this tick. A request the sampler dropped was never
	// exported, so a link to it would dangle; skipping it also skips the zero SpanContext of an
	// untraced caller. Nil when no command qualifies, so an untraced tick pays no allocation here.
	var links []oteltrace.Link
	for _, cmd := range commands {
		if !cmd.Span.IsSampled() {
			continue
		}
		if len(links) == maxCommandLinks {
			break
		}
		links = append(links, oteltrace.Link{SpanContext: cmd.Span, Attributes: []attribute.KeyValue{
			attrCommandName.String(cmd.Name), attrCommandPersona.String(cmd.Persona)}})
	}

	ctx, span := trace.New(context.Background(), spanTick,
		oteltrace.WithAttributes(
			attrTickHeight.Int64(int64(w.currentTick.height)), //nolint:gosec // tick height stays far below int64 max
			attrTickCommands.Int(len(commands))),
		oteltrace.WithLinks(links...))
	defer span.End()
	w.tickCtx = ctx
	defer func() { w.tickCtx = context.Background() }()

	w.currentTick.timestamp = timestamp

	// Advance the ECS world.
	run()

	w.dispatchEvents(ctx)

	// Serialize the world for snapshots and the debug service. Encoding cannot fail (a value that
	// cannot be encoded panics inside the ECS), so there is no retry path.
	snapshotDue := w.currentTick.height%uint64(w.options.SnapshotRate) == 0
	if snapshotDue || w.debug != nil {
		_, persistSpan := trace.New(ctx, spanPersistState,
			oteltrace.WithAttributes(attrSnapshotDue.Bool(snapshotDue)))
		defer persistSpan.End()

		data := w.encodeSnapshot(timestamp)

		// Hand the debug service the same frozen bytes. Nobody writes to them, so sharing with the
		// writer below is safe.
		w.debug.publishState(data)

		if snapshotDue {
			w.snapshotWriter.Write(w.currentTick.height, data)
		}
	}

	// Increase the tick height.
	w.currentTick.height++
}

// maxCommandLinks caps the links on a tick span. It matches the OpenTelemetry SDK's default link
// limit (OTEL_SPAN_LINK_COUNT_LIMIT); links past it would only be built to be dropped, and the SDK
// drops them one memmove at a time.
const maxCommandLinks = 128

// dispatchEvents runs the tick's event handlers under their own span. The span is ended by a
// direct defer so a panicking handler (encoding panics on unencodable payloads) still closes
// it with the panic recorded.
func (w *World) dispatchEvents(ctx context.Context) {
	ctx, span := trace.New(ctx, spanEventDispatch)
	defer span.End()
	if err := w.events.Dispatch(ctx); err != nil {
		span.SetError(err)
		w.tel.Logger.Warn().Err(err).Msg("errors encountered dispatching events")
	}
	w.service.drainInterShardCommands()
}

// encodeSnapshot produces the complete snapshot bytes for the current tick: the ECS sizes and
// streams its world state directly into one exactly-sized buffer, and the envelope is hand-encoded
// around it. No intermediate proto graph exists; the buffer is the freeze-frame.
func (w *World) encodeSnapshot(timestamp time.Time) []byte {
	bodySize := w.world.StateWireSize()
	return snapshot.Encode(w.currentTick.height, timestamp, bodySize, w.world.AppendStateWire)
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
	// Reset the ECS world and run initialization systems again.
	w.world.Reset()
	w.init()

	// Clear pending commands and events.
	w.commands.Clear()
	w.events.Clear()

	// Reset the tick.
	w.currentTick.height = 0
	w.currentTick.timestamp = time.Time{}

	// Publish the reset state when the debug service is enabled.
	if w.debug != nil {
		w.debug.publishState(w.encodeSnapshot(w.currentTick.timestamp))
	}
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
func (w *World) RegisterPlugin(plugin Plugin) {
	if w.started {
		panic(ErrWorldStarted)
	}
	plugin.Register(w)
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
	cfg := systemConfig{hook: Update}
	for _, opt := range opts {
		opt(&cfg)
	}
	name := fmt.Sprintf("%T", s)
	hookName := ecsHookToProto(uint8(cfg.hook)).String()

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
		s.Run(w)
	}

	if err := w.world.RegisterSystem(name, cfg.hook, fn); err != nil {
		panic(eris.Wrapf(err, "error registering system"))
	}
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

// RegisterCommand registers a command type before world startup. Registering it again is a no-op.
// The service accepts a command from clients only once it is registered here.
func (w *World) RegisterCommand[T Command]() {
	if w.started {
		panic(ErrWorldStarted)
	}
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
			// The queue stores the decoded value as a Payload; recover the concrete type. Value
			// semantics, no pointer: Serializable is satisfied by the value type.
			payload, isT := cmd.Payload.(T)
			assert.That(isT, "mismatched command type passed to command context")
			if !yield(CommandContext[T]{Payload: payload, Persona: cmd.Persona}) {
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
// happens in the background, in order. A send that fails is not returned but is logged at error level,
// because a dropped shard-to-shard command is serious.
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
func (w *World) RegisterComponent[T ecs.Component]() {
	if w.started {
		panic(ErrWorldStarted)
	}
	if _, err := w.world.RegisterComponent[T](); err != nil {
		panic(eris.Wrapf(err, "failed to register component %T", *new(T)))
	}
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
