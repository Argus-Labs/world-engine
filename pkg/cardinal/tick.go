package cardinal

import (
	"context"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/plugin"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// The tick lifecycle: the loop that drives ticks, and the frame each tick runs in. Hooks registered
// by plugins run at fixed points of the frame: see step.

func (w *World) run(ctx context.Context) error {
	// Initialize the world and run initialization systems.
	w.init()

	if err := w.restore(ctx); err != nil {
		return eris.Wrap(err, "failed to restore state from snapshot")
	}

	logger := w.tel.GetLogger("shard")
	logger.Info().Msg("starting core shard loop")

	ticker := time.NewTicker(w.options.tickInterval())
	defer ticker.Stop()

	// A single select listens on every control channel regardless of pause state. This
	// prevents a handler send from becoming unreachable after a state transition (which
	// caused uncancellable hangs and late out-of-band Step/Reset fulfillment). The isPaused
	// guards inside the step/reset/ticker arms ensure stale requests that arrive while the
	// world is running are NACKed (reply channel closed) instead of mutating state.
	for {
		select {
		case <-ticker.C:
			if !w.debug.isPaused() {
				w.Tick(time.Now())
			}
		case replyCh := <-w.debug.pauseChan():
			w.debug.setPaused(true)

			replyCh <- w.currentTick.height
		case <-w.debug.resumeChan():
			if w.debug.isPaused() {
				w.debug.setPaused(false)
			}
		case replyCh := <-w.debug.stepChan():
			if !w.debug.isPaused() {
				close(replyCh)
				continue
			}

			w.Tick(time.Now())

			replyCh <- w.currentTick.height
		case replyCh := <-w.debug.resetChan():
			if !w.debug.isPaused() {
				close(replyCh)
				continue
			}

			w.reset()

			// Persist the reset state now. Shutdown writes nothing at height 0, so a world stopped
			// before its next tick would otherwise restart from the pre-reset snapshot.
			data := w.encodeSnapshot(labelBeforeTickZero, w.currentTick.timestamp)
			w.snapshotWriter.Write(labelBeforeTickZero, data)

			replyCh <- struct{}{}
		case <-ctx.Done():
			// Capture only on normal shutdown, between ticks. A defer would also capture
			// partially mutated state while unwinding a panic and overwrite the good snapshot.
			if w.currentTick.height > 0 {
				// Label the last completed tick, matching regular snapshots and restore's +1.
				label := w.currentTick.height - 1
				w.snapshotWriter.Write(label, w.encodeSnapshot(label, time.Now()))
			}

			return ctx.Err()
		}
	}
}

// init runs the init systems under a root span so their child spans have a parent. traceCtx is
// restored by defer, as in Tick, so a recovered init panic or a nested run does not leave it on an
// ended span.
func (w *World) init() {
	ctx, span := trace.New(context.Background(), spanInit)
	defer span.End()

	prevCtx := w.traceCtx

	w.traceCtx = ctx
	defer func() { w.traceCtx = prevCtx }()

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
	start := time.Now()

	// Drain before starting the span: links must be passed at start for a sampler to see them.
	commands := w.commands.Drain()

	// Link each drained command whose enqueuing request was sampled, up to maxCommandLinks. The
	// cap is the tracer provider's resolved span link limit (OTEL_SPAN_LINK_COUNT_LIMIT, default
	// 128), threaded in from telemetry.New, so the cap and the provider share one source of truth:
	// links are never built only to be dropped by the SDK, under any supported configuration. Links,
	// not children: that request finished before this tick. A request the sampler dropped was never
	// exported, so a link to it would dangle; skipping it also skips the zero SpanContext of an
	// untraced caller. Nil when no command qualifies, so an untraced tick pays no allocation here.
	maxCommandLinks := w.tel.MaxCommandLinks()

	var links []oteltrace.Link

	for _, cmd := range commands {
		if !cmd.Span.IsSampled() {
			continue
		}

		if len(links) == maxCommandLinks {
			break
		}

		links = append(links, oteltrace.Link{SpanContext: cmd.Span, Attributes: []attribute.KeyValue{
			attrCommandName.String(cmd.Name), attrCommandSender.String(cmd.Sender.ID())}})
	}

	ctx, span := trace.New(context.Background(), spanTick,
		oteltrace.WithAttributes(
			attrTickHeight.Int64(int64(w.currentTick.height)), //nolint:gosec // tick height stays far below int64 max
			attrTickCommands.Int(len(commands))),
		oteltrace.WithLinks(links...))
	defer span.End()

	prevCtx := w.traceCtx

	w.traceCtx = ctx
	defer func() { w.traceCtx = prevCtx }()

	for _, cmd := range commands {
		w.metrics.countCommand(ctx, cmd.Name)
	}

	w.currentTick.timestamp = timestamp

	// Drop the commands a hook does not admit, before any system reads them.
	w.hooks.Begin()
	w.admitCommands()

	// Advance the ECS world.
	run()

	w.hooks.Commit()
	w.dispatchEvents(ctx)
	w.hooks.Settle()

	// Serialize the world for snapshots and the debug service. Encoding cannot fail (a value that
	// cannot be encoded panics inside the ECS), so there is no retry path.
	snapshotDue := w.currentTick.height%uint64(w.options.SnapshotRate) == 0
	if snapshotDue || w.debug != nil {
		_, persistSpan := trace.New(ctx, spanPersistState,
			oteltrace.WithAttributes(attrSnapshotDue.Bool(snapshotDue)))
		defer persistSpan.End()

		data := w.encodeSnapshot(w.currentTick.height, timestamp)

		// Hand the debug service the same frozen bytes. Nobody writes to them, so sharing with the
		// writer below is safe.
		w.debug.publishState(data)

		if snapshotDue {
			w.snapshotWriter.Write(w.currentTick.height, data)
		}
	}

	// Increase the tick height.
	w.currentTick.height++

	w.metrics.recordTick(ctx, time.Since(start))
}

// dispatchEvents runs the tick's event handlers under their own span. The span is ended by a
// direct defer so a panicking handler (encoding panics on unencodable payloads) still closes
// it with the panic recorded.
func (w *World) dispatchEvents(ctx context.Context) {
	ctx, span := trace.New(ctx, spanEventDispatch)
	defer span.End()

	// Dispatch publishes what the tick decided. Its handlers, and the audiences they ask, only read.
	previous := w.world.ReadOnly("event dispatch")
	defer w.world.ReadOnly(previous)

	if err := w.events.Dispatch(ctx); err != nil {
		span.SetError(err)
		w.tel.Logger.Warn().Err(err).Msg("errors encountered dispatching events")
	}

	w.service.drainInterShardCommands()
}

// admitCommands asks the hooks about each of the tick's commands before any system reads them. It
// removes every command a hook does not admit, and tells each rejected player why.
func (w *World) admitCommands() {
	if w.hooks == nil {
		return
	}

	type rejection struct {
		cmd     command.Command
		verdict plugin.Verdict
	}

	var rejected []rejection

	w.commands.Retain(func(cmd command.Command) bool {
		verdict := w.hooks.Admit(cmd)
		if verdict.Outcome == plugin.Rejected {
			rejected = append(rejected, rejection{cmd, verdict})
		}

		return verdict.Outcome == plugin.Admitted
	})
	// Reply only once every command is judged: sending publishes pending changes first.
	for _, r := range rejected {
		if player, ok := r.cmd.Sender.Player(); ok {
			w.SendTo(player, syncstate.CommandRejected{
				Command: r.cmd.Name, Reason: syncstate.Reason(r.verdict.Reason), CommandID: r.verdict.CommandID,
			})

			continue
		}

		w.tel.Logger.Warn().Str("command", r.cmd.Name).Str("sender", r.cmd.Sender.ID()).
			Str("reason", r.verdict.Reason).Msg("rejected a command from another shard")
	}
}

// encodeSnapshot encodes component payloads, then marshals the complete protobuf container once.
// Encoding completes on the tick before owned bytes are handed to the writer. Unencodable state
// fails fast, before any partial snapshot can be published or stored.
//
// tick is the label stamped into the envelope's tick_height, supplied by the caller so the same
// world state can be labeled with the pre-increment height (persistState, the final shutdown
// snapshot) rather than always reading w.currentTick.height.
func (w *World) encodeSnapshot(tick uint64, timestamp time.Time) []byte {
	state, err := w.world.ToProto()
	if err != nil {
		panic(err)
	}

	data, err := snapshot.Encode(tick, timestamp, state)
	if err != nil {
		panic(err)
	}

	return data
}
