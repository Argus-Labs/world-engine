package cardinal

import (
	"context"
	"time"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	otelcodes "go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
)

// interShardSendTimeout bounds how long one send waits for the target shard to accept it.
const interShardSendTimeout = 10 * time.Second

// interShard is this shard's link to other shards. It receives their commands into the command queues
// and sends this shard's commands to them, and neither direction makes the tick wait on NATS.
//
// Receiving: handle runs on NATS handler goroutines. It validates each command, enqueues it into the
// command queues, which client commands share, and acks. The tick drains those queues at its start.
//
// Sending mirrors receiving: the tick enqueues each command during event dispatch and drains once at the
// end of the tick. Drain hands each target its batch for the tick through that target's pipeline: a
// buffer goroutine that holds the backlog, feeding a send loop that sends one command at a time. The
// buffer never does network work, so it always takes the tick's batch at once, however slow the target.
//
// Pipelines are partitioned by target so commands to a healthy target are not held behind a slow or
// hung one. Order is kept per target only. One shared line would not be safer: a failed send does not
// stop the commands after it, so sharing a line only delays them, and targets process on their own
// ticks anyway.
//
// enqueue, drain and stop must all be called from the tick goroutine, which owns queued and pipes.
type interShard struct {
	address *micro.ServiceAddress
	client  *micro.Client
	inbox   *command.Manager // Received commands go here, alongside client commands
	log     zerolog.Logger
	queued  map[string][]queuedCommand // This tick's outbound commands, by target address
	pipes   map[string]*targetPipe     // One pipeline per target, created on first send
}

// queuedCommand is one command to send. parent is the span that enqueued it, so the send span joins
// the tick's trace even though the tick has moved on by the time it runs.
type queuedCommand struct {
	parent oteltrace.SpanContext
	cmd    *iscv1.Command
}

// targetPipe is the pipeline to one target shard.
type targetPipe struct {
	in   chan []queuedCommand // Tick to buffer: one batch per tick; closed by stop
	done chan struct{}        // Closes when the send loop has sent everything and exited
}

func newInterShard(
	address *micro.ServiceAddress, client *micro.Client, inbox *command.Manager, log zerolog.Logger,
) *interShard {
	return &interShard{
		address: address,
		client:  client,
		inbox:   inbox,
		log:     log,
		queued:  make(map[string][]queuedCommand),
		pipes:   make(map[string]*targetPipe),
	}
}

// start registers the receive endpoint for each named command on svc.
func (s *interShard) start(svc *micro.Service, commands map[string]struct{}) error {
	for name := range commands {
		if err := svc.AddGroup("command").AddEndpoint(name, s.handle); err != nil {
			return eris.Wrapf(err, "failed to register %s command handler", name)
		}
	}
	return nil
}

// handle receives one command from another shard. The ack it returns means the command was queued,
// not that a tick has processed it.
func (s *interShard) handle(ctx context.Context, req *micro.Request) *micro.Response {
	select {
	case <-ctx.Done():
		return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
	default:
	}

	cmd := &iscv1.Command{}
	if err := req.Payload.UnmarshalTo(cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.InvalidArgument)
	}

	if err := protovalidate.Validate(cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to validate command"), codes.InvalidArgument)
	}
	if _, err := micro.ParseAddress(cmd.GetPersona().GetId()); err != nil {
		return micro.NewErrorResponse(
			req,
			eris.Wrap(err, "command persona is not a shard address"),
			codes.InvalidArgument,
		)
	}

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return micro.NewErrorResponse(
			req,
			eris.New("command address doesn't match shard address"),
			codes.InvalidArgument,
		)
	}

	oteltrace.SpanFromContext(ctx).SetAttributes(attrCommandName.String(cmd.GetName()))
	if err := s.inbox.Enqueue(ctx, cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), codes.InvalidArgument)
	}

	return micro.NewSuccessResponse(req, nil)
}

// enqueue adds cmd to this tick's batch for its target. Nothing is sent until drain.
func (s *interShard) enqueue(ctx context.Context, cmd *iscv1.Command) {
	target := micro.String(cmd.GetAddress())
	s.queued[target] = append(s.queued[target],
		queuedCommand{parent: oteltrace.SpanContextFromContext(ctx), cmd: cmd})
}

// drain hands each target its batch for this tick. It returns without waiting for any send.
func (s *interShard) drain() {
	for target, batch := range s.queued {
		pipe, ok := s.pipes[target]
		if !ok {
			pipe = s.startPipe()
			s.pipes[target] = pipe
		}
		pipe.in <- batch // Ownership of batch moves to the pipeline.
		delete(s.queued, target)
	}
}

func (s *interShard) startPipe() *targetPipe {
	pipe := &targetPipe{in: make(chan []queuedCommand), done: make(chan struct{})}
	out := make(chan []queuedCommand)
	go bufferLoop(pipe.in, out)
	go s.sendLoop(out, pipe.done)
	return pipe
}

// bufferLoop holds batches from in until out is ready for them, oldest first. It only moves slices,
// so it is always ready to receive from in. When in is closed it hands over what it holds and closes out.
func bufferLoop(in <-chan []queuedCommand, out chan<- []queuedCommand) {
	var pending [][]queuedCommand
	for {
		// A nil channel is never ready, which disables the hand-over case while pending is empty.
		var outCh chan<- []queuedCommand
		var next []queuedCommand
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

func (s *interShard) sendLoop(out <-chan []queuedCommand, done chan<- struct{}) {
	defer close(done)
	for batch := range out {
		for _, c := range batch {
			s.send(c)
		}
	}
}

func (s *interShard) send(c queuedCommand) {
	// The NATS client injects this span into the request headers, so the receiving shard's handler
	// span (and the tick that drains the command there) joins the sending tick's trace.
	ctx := oteltrace.ContextWithSpanContext(context.Background(), c.parent)
	ctx, span := trace.New(ctx, spanInterShardSend, oteltrace.WithAttributes(
		attrCommandName.String(c.cmd.GetName()), attrCommandTarget.String(micro.String(c.cmd.GetAddress()))))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, interShardSendTimeout)
	defer cancel()

	if _, err := s.client.Request(ctx, c.cmd.GetAddress(), "command."+c.cmd.GetName(), c.cmd); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "send failed")
		s.log.Error().Err(err).Str("command", c.cmd.GetName()).Msg("inter-shard command dropped: send failed")
	}
}

// stop closes every pipeline and waits, until ctx is done, for the drained commands to be sent. Commands
// enqueued but not drained belong to a tick that did not finish and are dropped. Call it once, after the
// tick loop has stopped.
func (s *interShard) stop(ctx context.Context) {
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
