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

const interShardSendTimeout = 10 * time.Second

// interShard receives commands from other shards and sends this shard's commands to them, without the
// tick ever waiting on NATS. Receiving mirrors sending: handlers queue incoming commands for the tick to
// drain at its start, and the tick queues outgoing commands for the send loop, draining at its end.
//
// The buffer goroutine between drain and the send loop does no network work, so it always takes the
// tick's batch at once. The send loop sends one command per round trip, in order; a backlog beyond that
// grows in memory, by design.
//
// One pipeline serves every target, so a hung target delays the others. It is not split per address:
// an address is only a NATS subject, so the sender cannot tell which addresses fail together, and
// per-address pipelines would keep state for every address ever used.
//
// enqueue, drain and stop must be called from the tick goroutine, which owns queued.
type interShard struct {
	address *micro.ServiceAddress
	client  *micro.Client
	inbox   *command.Manager // Shared with client commands
	log     zerolog.Logger
	queued  []queuedCommand
	in      chan []queuedCommand // One batch per tick; closed by stop
	done    chan struct{}        // Closed when the send loop has sent everything
}

// queuedCommand keeps the enqueuing span as parent, so the send joins the tick's trace after the tick ends.
type queuedCommand struct {
	parent oteltrace.SpanContext
	cmd    *iscv1.Command
}

// newInterShard starts the send goroutines; stop ends them.
func newInterShard(
	address *micro.ServiceAddress, client *micro.Client, inbox *command.Manager, log zerolog.Logger,
) *interShard {
	s := &interShard{
		address: address,
		client:  client,
		inbox:   inbox,
		log:     log,
		in:      make(chan []queuedCommand),
		done:    make(chan struct{}),
	}
	out := make(chan []queuedCommand)
	go bufferLoop(s.in, out)
	go s.sendLoop(out, s.done)
	return s
}

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
		return micro.NewErrorResponse(req, eris.Wrap(err, "command persona is not a shard address"), codes.InvalidArgument)
	}

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return micro.NewErrorResponse(req, eris.New("command address doesn't match shard address"), codes.InvalidArgument)
	}

	oteltrace.SpanFromContext(ctx).SetAttributes(attrCommandName.String(cmd.GetName()))
	if err := s.inbox.Enqueue(ctx, cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), codes.InvalidArgument)
	}

	return micro.NewSuccessResponse(req, nil)
}

func (s *interShard) enqueue(ctx context.Context, cmd *iscv1.Command) {
	s.queued = append(s.queued, queuedCommand{parent: oteltrace.SpanContextFromContext(ctx), cmd: cmd})
}

func (s *interShard) drain() {
	if len(s.queued) == 0 {
		return
	}
	s.in <- s.queued // Ownership of the batch moves to the pipeline.
	s.queued = nil
}

// bufferLoop holds batches until out takes them, oldest first. It only moves slices, so it is always
// ready to receive from in. When in closes, it hands over what it holds and closes out.
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

// stop waits until ctx for drained commands to be sent. Commands enqueued but not drained belong to a tick
// that did not finish and are dropped. Call it once, after the tick loop has stopped.
func (s *interShard) stop(ctx context.Context) {
	close(s.in)
	select {
	case <-s.done:
	case <-ctx.Done():
		s.log.Error().Msg("inter-shard commands not sent before the shutdown deadline")
	}
}
