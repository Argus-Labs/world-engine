package cardinal

import (
	"context"
	"math"
	"net/http"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/performance"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

const perfBatchIntervalSec = 1 // Target wall-clock seconds between perf batches.

// debugModule provides introspection and debugging capabilities for a World instance.
type debugModule struct {
	world      *World
	server     *http.Server
	control    *tickControl
	reflector  *jsonschema.Reflector
	commands   map[string]*structpb.Struct
	events     map[string]*structpb.Struct
	components map[string]*structpb.Struct
	perf       *performance.Collector
}

var _ cardinalv1connect.DebugServiceHandler = (*debugModule)(nil)

// newDebugModule creates a new debugModule bound to the given World.
func newDebugModule(world *World) debugModule {
	batchSize := max(int(math.Round(world.options.TickRate))*perfBatchIntervalSec, 1)
	perf := performance.NewCollector(batchSize)

	return debugModule{
		world:      world,
		control:    newTickControl(),
		commands:   make(map[string]*structpb.Struct),
		events:     make(map[string]*structpb.Struct),
		components: make(map[string]*structpb.Struct),
		reflector: &jsonschema.Reflector{
			Anonymous:      true, // Don't add $id based on package path
			ExpandedStruct: true, // Inline the struct fields directly
		},
		perf: perf,
	}
}

// Init initializes and starts the connect server for the debug service.
func (d *debugModule) Init(addr string) {
	if d == nil {
		return
	}

	logger := d.world.tel.GetLogger("debug")

	mux := http.NewServeMux()
	mux.Handle(cardinalv1connect.NewDebugServiceHandler(d, connect.WithInterceptors(validate.NewInterceptor())))

	d.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info().Str("addr", addr).Msg("Debug service initialized")

	go func() {
		_ = d.server.ListenAndServe()
	}()
}

// Shutdown gracefully shuts down the debug server.
func (d *debugModule) Shutdown(ctx context.Context) error {
	if d == nil || d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

// -------------------------------------------------------------------------------------------------
// Introspect
// -------------------------------------------------------------------------------------------------

// register records the JSON schema of a command, event, or component type for introspection.
func (d *debugModule) register(kind string, value schema.Serializable) error {
	if d == nil {
		return nil
	}

	var catalog map[string]*structpb.Struct
	switch kind {
	case "command":
		catalog = d.commands
	case "event":
		catalog = d.events
	case "component":
		catalog = d.components
	default:
		panic("this is an internal function, this should never panic")
	}

	name := value.Name()
	if _, exists := catalog[name]; exists {
		return nil
	}

	jsonSchema := d.reflector.Reflect(value)
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		return eris.Wrap(err, "failed to marshal json schema")
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		return eris.Wrap(err, "failed to unmarshal json schema")
	}

	// Remove redundant fields.
	delete(schemaMap, "$schema")
	delete(schemaMap, "type")
	delete(schemaMap, "additionalProperties")

	schemaStruct, err := structpb.NewStruct(schemaMap)
	if err != nil {
		return eris.Wrap(err, "failed to create struct from schema")
	}

	catalog[name] = schemaStruct
	return nil
}

// Introspect returns metadata about the registered types in the world.
func (d *debugModule) Introspect(
	_ context.Context,
	_ *connect.Request[cardinalv1.IntrospectRequest],
) (*connect.Response[cardinalv1.IntrospectResponse], error) {
	return connect.NewResponse(&cardinalv1.IntrospectResponse{
		Commands:   d.buildTypeSchemas(d.commands),
		Components: d.buildTypeSchemas(d.components),
		Events:     d.buildTypeSchemas(d.events),
		TickRateHz: uint32(math.Round(d.world.options.TickRate)),
		Schedules:  d.buildSchedules(),
	}), nil
}

// buildSchedules converts the ECS scheduler dependency graphs to proto messages.
func (d *debugModule) buildSchedules() []*cardinalv1.SystemSchedule {
	ecsSchedules := d.world.world.Schedules()
	schedules := make([]*cardinalv1.SystemSchedule, 0, len(ecsSchedules))
	for _, s := range ecsSchedules {
		if len(s.Systems) == 0 {
			continue
		}
		nodes := make([]*cardinalv1.SystemNode, len(s.Systems))
		for i, sys := range s.Systems {
			depsOn := make([]uint32, len(sys.DependsOn))
			for j, dep := range sys.DependsOn {
				depsOn[j] = uint32(dep) //nolint:gosec // bounded by system count
			}
			nodes[i] = &cardinalv1.SystemNode{
				Id:        uint32(sys.ID), //nolint:gosec // bounded by system count
				Name:      sys.Name,
				DependsOn: depsOn,
			}
		}
		schedules = append(schedules, &cardinalv1.SystemSchedule{
			Hook:    ecsHookToProto(uint8(s.Hook)),
			Systems: nodes,
		})
	}
	return schedules
}

// -------------------------------------------------------------------------------------------------
// Performance
// -------------------------------------------------------------------------------------------------

func ecsHookToProto(hook uint8) cardinalv1.SystemHook {
	mapping := [4]cardinalv1.SystemHook{
		cardinalv1.SystemHook_SYSTEM_HOOK_PRE_UPDATE,
		cardinalv1.SystemHook_SYSTEM_HOOK_UPDATE,
		cardinalv1.SystemHook_SYSTEM_HOOK_POST_UPDATE,
		cardinalv1.SystemHook_SYSTEM_HOOK_INIT,
	}
	if int(hook) < len(mapping) {
		return mapping[hook]
	}
	return cardinalv1.SystemHook_SYSTEM_HOOK_UNSPECIFIED
}

// StreamPerf streams batches of per-tick timing data to connected clients.
func (d *debugModule) StreamPerf(
	ctx context.Context,
	_ *connect.Request[cardinalv1.StreamPerfRequest],
	stream *connect.ServerStream[cardinalv1.PerfBatch],
) error {
	ch := d.perf.Subscribe()
	defer d.perf.Unsubscribe(ch)

	for {
		select {
		case batch := <-ch:
			proto := batchToProto(batch)
			if err := stream.Send(proto); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func batchToProto(b performance.Batch) *cardinalv1.PerfBatch {
	ticks := make([]*cardinalv1.TickTimeline, 0, len(b.Ticks))
	for _, ts := range b.Ticks {
		spans := make([]*cardinalv1.SystemSpan, 0, len(ts.Spans))
		for _, span := range ts.Spans {
			startOffset := span.StartTime.Sub(ts.TickStart).Nanoseconds()
			duration := span.EndTime.Sub(span.StartTime).Nanoseconds()
			if startOffset < 0 {
				startOffset = 0
			}
			if duration < 0 {
				duration = 0
			}
			spans = append(spans, &cardinalv1.SystemSpan{
				SystemHook:    ecsHookToProto(span.SystemHook),
				System:        span.SystemName,
				StartOffsetNs: uint64(startOffset), //nolint:gosec // clamped to >= 0
				DurationNs:    uint64(duration),    //nolint:gosec // clamped to >= 0
			})
		}
		ticks = append(ticks, &cardinalv1.TickTimeline{
			TickHeight: ts.TickHeight,
			TickStart:  timestamppb.New(ts.TickStart),
			Spans:      spans,
		})
	}
	return &cardinalv1.PerfBatch{
		DroppedSpans:   b.DroppedSpans,
		DroppedBatches: b.DroppedBatches,
		Ticks:          ticks,
	}
}

// recordTick records a completed tick. Nil-safe.
func (d *debugModule) recordTick(tickHeight uint64, tickStart time.Time) {
	if d == nil {
		return
	}
	d.perf.RecordTick(tickHeight, tickStart)
}

// startPerfTick initializes span storage for a new tick. Nil-safe.
func (d *debugModule) startPerfTick() {
	if d == nil {
		return
	}
	d.perf.StartTick()
}

// resetPerf clears all buffered performance data. Nil-safe.
func (d *debugModule) resetPerf() {
	if d == nil {
		return
	}
	d.perf.Reset()
}

// recordSpan records a per-system span. Nil-safe.
func (d *debugModule) recordSpan(span performance.TickSpan) {
	if d == nil {
		return
	}
	d.perf.RecordSpan(span)
}

// buildTypeSchemas converts the internal schema cache to proto TypeSchema messages.
func (d *debugModule) buildTypeSchemas(cache map[string]*structpb.Struct) []*cardinalv1.TypeSchema {
	schemas := make([]*cardinalv1.TypeSchema, 0, len(cache))
	for name, schemaStruct := range cache {
		schemas = append(schemas, &cardinalv1.TypeSchema{
			Name:   name,
			Schema: schemaStruct,
		})
	}
	return schemas
}

// -------------------------------------------------------------------------------------------------
// Debugger
// -------------------------------------------------------------------------------------------------

// tickControl coordinates pause, resume, step, and reset signaling for the tick loop.
type tickControl struct {
	pauseCh   chan chan uint64   // Request pause, receives tick height when paused
	resumeCh  chan struct{}      // Signal to resume
	stepCh    chan chan uint64   // Request step, receives tick height after step
	resetCh   chan chan struct{} // Request reset
	isPaused  atomic.Bool        // Current pause state
	stepReady chan struct{}      // Signals that step result is ready to be read
}

// newTickControl creates a tickControl with initialized channels.
func newTickControl() *tickControl {
	return &tickControl{
		pauseCh:   make(chan chan uint64),
		resumeCh:  make(chan struct{}),
		stepCh:    make(chan chan uint64),
		resetCh:   make(chan chan struct{}),
		stepReady: make(chan struct{}),
	}
}

// Pause stops tick execution and returns the current tick height.
func (d *debugModule) Pause(
	_ context.Context,
	_ *connect.Request[cardinalv1.PauseRequest],
) (*connect.Response[cardinalv1.PauseResponse], error) {
	if d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world is already paused"))
	}

	replyCh := make(chan uint64, 1)
	d.control.pauseCh <- replyCh
	tickHeight := <-replyCh

	return connect.NewResponse(&cardinalv1.PauseResponse{
		TickHeight: tickHeight,
	}), nil
}

// Resume continues tick execution after a pause.
func (d *debugModule) Resume(
	_ context.Context,
	_ *connect.Request[cardinalv1.ResumeRequest],
) (*connect.Response[cardinalv1.ResumeResponse], error) {
	if !d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world is not paused"))
	}

	d.control.resumeCh <- struct{}{}

	return connect.NewResponse(&cardinalv1.ResumeResponse{}), nil
}

// Step executes a single tick. Only works when paused.
func (d *debugModule) Step(
	_ context.Context,
	_ *connect.Request[cardinalv1.StepRequest],
) (*connect.Response[cardinalv1.StepResponse], error) {
	if !d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world must be paused to step"))
	}

	replyCh := make(chan uint64, 1)
	d.control.stepCh <- replyCh
	tickHeight := <-replyCh

	return connect.NewResponse(&cardinalv1.StepResponse{
		TickHeight: tickHeight,
	}), nil
}

// Reset restores the world to its initial state (before tick 0).
func (d *debugModule) Reset(
	_ context.Context,
	_ *connect.Request[cardinalv1.ResetRequest],
) (*connect.Response[cardinalv1.ResetResponse], error) {
	if !d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world must be paused to reset"))
	}

	replyCh := make(chan struct{}, 1)
	d.control.resetCh <- replyCh
	<-replyCh

	return connect.NewResponse(&cardinalv1.ResetResponse{}), nil
}

// GetState returns the current world state snapshot.
func (d *debugModule) GetState(
	_ context.Context,
	_ *connect.Request[cardinalv1.GetStateRequest],
) (*connect.Response[cardinalv1.GetStateResponse], error) {
	worldState, err := d.world.world.ToProto()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, eris.Wrap(err, "failed to serialize world state"))
	}

	return connect.NewResponse(&cardinalv1.GetStateResponse{
		IsPaused: d.control.isPaused.Load(),
		Snapshot: &cardinalv1.Snapshot{
			TickHeight: d.world.currentTick.height,
			Timestamp:  timestamppb.New(d.world.currentTick.timestamp),
			WorldState: worldState,
		},
	}), nil
}

// isPaused returns whether the world is currently paused. Returns false if d is nil.
func (d *debugModule) isPaused() bool {
	if d == nil {
		return false
	}
	return d.control.isPaused.Load()
}

// setPaused sets the paused state. No-op if d is nil.
func (d *debugModule) setPaused(v bool) {
	if d == nil {
		return
	}
	d.control.isPaused.Store(v)
}

// pauseChan returns the pause request channel, or nil if d is nil.
func (d *debugModule) pauseChan() <-chan chan uint64 {
	if d == nil {
		return nil
	}
	return d.control.pauseCh
}

// resumeChan returns the resume signal channel, or nil if d is nil.
func (d *debugModule) resumeChan() <-chan struct{} {
	if d == nil {
		return nil
	}
	return d.control.resumeCh
}

// stepChan returns the step request channel, or nil if d is nil.
func (d *debugModule) stepChan() <-chan chan uint64 {
	if d == nil {
		return nil
	}
	return d.control.stepCh
}

// resetChan returns the reset request channel, or nil if d is nil.
func (d *debugModule) resetChan() <-chan chan struct{} {
	if d == nil {
		return nil
	}
	return d.control.resetCh
}
