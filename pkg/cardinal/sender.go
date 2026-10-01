package cardinal

import (
	"context"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	otelcodes "go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// interShardSendTimeout bounds how long one send waits for the target shard to accept it.
const interShardSendTimeout = 10 * time.Second

// interShardSender sends inter-shard commands off the tick goroutine, so the tick never waits on NATS.
//
// The tick stages each command during event dispatch and flushes once at the end of the tick. Flush
// hands each target its batch for the tick through that target's pipeline: a buffer goroutine that
// holds the backlog, feeding a send loop that sends one command at a time. The buffer never does network
// work, so it always takes the tick's batch at once, however slow the target.
//
// Pipelines are partitioned by target so commands to a healthy target are not held behind a slow or
// hung one. Order is kept per target only. One shared line would not be safer: a failed send does not
// stop the commands after it, so sharing a line only delays them, and targets process on their own
// ticks anyway.
//
// stage, flush and stop must all be called from the tick goroutine, which owns staged and pipes.
type interShardSender struct {
	client *micro.Client
	log    zerolog.Logger
	staged map[string][]outboundCommand // This tick's commands, by target address
	pipes  map[string]*shardPipe        // One pipeline per target, created on first send
}

// outboundCommand is one command to send. parent is the span that staged it, so the send span joins
// the tick's trace even though the tick has moved on by the time it runs.
type outboundCommand struct {
	parent oteltrace.SpanContext
	cmd    *iscv1.Command
}

// shardPipe is the pipeline to one target shard.
type shardPipe struct {
	in   chan []outboundCommand // Tick to buffer: one batch per tick; closed by stop
	done chan struct{}          // Closes when the send loop has sent everything and exited
}

func newInterShardSender(client *micro.Client, log zerolog.Logger) *interShardSender {
	return &interShardSender{
		client: client,
		log:    log,
		staged: make(map[string][]outboundCommand),
		pipes:  make(map[string]*shardPipe),
	}
}

// stage adds cmd to this tick's batch for its target. Nothing is sent until flush.
func (s *interShardSender) stage(ctx context.Context, cmd *iscv1.Command) {
	target := micro.String(cmd.GetAddress())
	s.staged[target] = append(s.staged[target],
		outboundCommand{parent: oteltrace.SpanContextFromContext(ctx), cmd: cmd})
}

// flush hands each target its batch for this tick. It returns without waiting for any send.
func (s *interShardSender) flush() {
	for target, batch := range s.staged {
		pipe, ok := s.pipes[target]
		if !ok {
			pipe = s.startPipe()
			s.pipes[target] = pipe
		}
		pipe.in <- batch // Ownership of batch moves to the pipeline.
		delete(s.staged, target)
	}
}

func (s *interShardSender) startPipe() *shardPipe {
	pipe := &shardPipe{in: make(chan []outboundCommand), done: make(chan struct{})}
	out := make(chan []outboundCommand)
	go bufferBatches(pipe.in, out)
	go s.sendLoop(out, pipe.done)
	return pipe
}

// bufferBatches holds batches from in until out is ready for them, oldest first. It only moves slices,
// so it is always ready to receive from in. When in is closed it hands over what it holds and closes out.
func bufferBatches(in <-chan []outboundCommand, out chan<- []outboundCommand) {
	var pending [][]outboundCommand
	for {
		// A nil channel is never ready, which disables the hand-over case while pending is empty.
		var outCh chan<- []outboundCommand
		var next []outboundCommand
		if len(pending) > 0 {
			outCh, next = out, pending[0]
		}
		select {
		case batch, ok := <-in:
			if !ok {
				for _, b := range pending {
					out <- b
				}
				close(out)
				return
			}
			pending = append(pending, batch)
		case outCh <- next:
			pending[0] = nil // Let the sent batch be collected before pending reallocates.
			pending = pending[1:]
			if len(pending) == 0 {
				pending = nil
			}
		}
	}
}

func (s *interShardSender) sendLoop(out <-chan []outboundCommand, done chan<- struct{}) {
	defer close(done)
	for batch := range out {
		for _, cmd := range batch {
			s.send(cmd)
		}
	}
}

func (s *interShardSender) send(out outboundCommand) {
	// The NATS client injects this span into the request headers, so the receiving shard's handler
	// span (and the tick that drains the command there) joins the sending tick's trace.
	ctx := oteltrace.ContextWithSpanContext(context.Background(), out.parent)
	ctx, span := trace.New(ctx, spanInterShardSend, oteltrace.WithAttributes(
		attrCommandName.String(out.cmd.GetName()), attrCommandTarget.String(micro.String(out.cmd.GetAddress()))))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, interShardSendTimeout)
	defer cancel()

	if _, err := s.client.Request(ctx, out.cmd.GetAddress(), "command."+out.cmd.GetName(), out.cmd); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "send failed")
		s.log.Error().Err(err).Str("command", out.cmd.GetName()).Msg("inter-shard command dropped: send failed")
	}
}

// stop closes every pipeline and waits, until ctx is done, for the flushed commands to be sent. Commands
// staged but not flushed belong to a tick that did not finish and are dropped. Call it once, after the
// tick loop has stopped.
func (s *interShardSender) stop(ctx context.Context) {
	for _, pipe := range s.pipes {
		close(pipe.in)
	}
	for target, pipe := range s.pipes {
		select {
		case <-pipe.done:
		case <-ctx.Done():
			s.log.Error().Str("target", target).
				Msg("inter-shard commands not sent before the shutdown deadline")
		}
	}
}
