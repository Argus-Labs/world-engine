package cardinal

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/performance"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

const perfBatchIntervalSec = 1 // Target wall-clock seconds between perf batches.

// debugModule provides introspection and debugging capabilities for a World instance.
// Its DebugService handler is mounted on the service port (see service.init).
type debugModule struct {
	world    *World
	control  *tickControl
	catalog  *introspect.Catalog
	perf     *performance.Collector
	snapshot atomic.Pointer[cardinalv1.Snapshot]
}

var _ cardinalv1connect.DebugServiceHandler = (*debugModule)(nil)

// newDebugModule creates a new debugModule bound to the given World.
func newDebugModule(world *World) *debugModule {
	batchSize := max(int(math.Round(world.options.TickRate))*perfBatchIntervalSec, 1)
	perf := performance.NewCollector(batchSize)

	d := &debugModule{
		world:   world,
		control: newTickControl(),
		catalog: introspect.NewCatalog(),
		perf:    perf,
	}
	d.publishSnapshot(&cardinalv1.Snapshot{WorldState: &cardinalv1.WorldState{}})
	return d
}

func (d *debugModule) publishSnapshot(snapshot *cardinalv1.Snapshot) {
	if d == nil {
		return
	}
	d.snapshot.Store(snapshot)
}

// publishState decodes encoded snapshot bytes and publishes them for GetState. No-op when the
// debug service is disabled. The decode allocates, so callers should skip encoding entirely when
// d is nil rather than relying on this method's guard.
func (d *debugModule) publishState(data []byte) {
	if d == nil {
		return
	}
	snap, err := snapshot.Decode(data)
	if err != nil {
		d.world.tel.Logger.Warn().Err(err).Msg("failed to decode snapshot for the debug service")
		return
	}
	d.snapshot.Store(snap)
}

// -------------------------------------------------------------------------------------------------
// Introspect
// -------------------------------------------------------------------------------------------------

// register adds a world type to introspection. It is a no-op when debug is disabled.
func (d *debugModule) register(kind introspect.Kind, value schema.Serializable) error {
	if d == nil {
		return nil
	}
	return d.catalog.Register(kind, value)
}

// finalizeCatalog freezes registration and builds the metadata returned by Introspect.
func (d *debugModule) finalizeCatalog() error {
	if d == nil {
		return nil
	}
	return d.catalog.Finalize()
}

// Introspect returns metadata about the registered types in the world.
func (d *debugModule) Introspect(
	_ context.Context,
	_ *connect.Request[cardinalv1.IntrospectRequest],
) (*connect.Response[cardinalv1.IntrospectResponse], error) {
	return connect.NewResponse(&cardinalv1.IntrospectResponse{
		Commands:           d.catalog.Commands(),
		Components:         d.catalog.Components(),
		Events:             d.catalog.Events(),
		TickRateHz:         d.world.options.TickRate,
		Schedules:          d.buildSchedules(),
		ProtoDescriptorSet: d.catalog.DescriptorSet(),
	}), nil
}

// buildSchedules converts the ECS system lists to proto messages.
func (d *debugModule) buildSchedules() []*cardinalv1.SystemSchedule {
	ecsSchedules := d.world.world.Schedules()
	schedules := make([]*cardinalv1.SystemSchedule, 0, len(ecsSchedules))
	for _, s := range ecsSchedules {
		if len(s.Systems) == 0 {
			continue
		}
		nodes := make([]*cardinalv1.SystemNode, len(s.Systems))
		for i, sys := range s.Systems {
			nodes[i] = &cardinalv1.SystemNode{
				Id:   uint32(sys.ID), //nolint:gosec // bounded by system count
				Name: sys.Name,
			}
		}
		schedules = append(schedules, &cardinalv1.SystemSchedule{
			Hook:    ecsHookToProto(uint8(s.Hook)),
			Systems: nodes,
		})
	}
	return schedules
}

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

// -------------------------------------------------------------------------------------------------
// Performance
// -------------------------------------------------------------------------------------------------

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
		Ticks: ticks,
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

// -------------------------------------------------------------------------------------------------
// Debugger
// -------------------------------------------------------------------------------------------------

// tickControl coordinates pause, resume, step, and reset signaling for the tick loop.
type tickControl struct {
	pauseCh  chan chan uint64   // Request pause, receives tick height when paused
	resumeCh chan struct{}      // Signal to resume
	stepCh   chan chan uint64   // Request step, receives tick height after step
	resetCh  chan chan struct{} // Request reset
	isPaused atomic.Bool        // Current pause state
}

// newTickControl creates a tickControl with initialized channels.
func newTickControl() *tickControl {
	return &tickControl{
		pauseCh:  make(chan chan uint64),
		resumeCh: make(chan struct{}),
		stepCh:   make(chan chan uint64),
		resetCh:  make(chan chan struct{}),
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

// GetState returns the most recent published world state, at most one tick old.
func (d *debugModule) GetState(
	_ context.Context,
	_ *connect.Request[cardinalv1.GetStateRequest],
) (*connect.Response[cardinalv1.GetStateResponse], error) {
	return connect.NewResponse(&cardinalv1.GetStateResponse{
		IsPaused: d.control.isPaused.Load(),
		Snapshot: d.snapshot.Load(),
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
