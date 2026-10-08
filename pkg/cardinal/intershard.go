package cardinal

import (
	"context"
	"sync"
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
// drain at its start, and the tick queues outgoing commands for the senders, draining at its end.
//
// Each target with unsent commands has one sender goroutine, which sends them one at a time and waits
// for each ack before the next. A target that cannot ack one command gets no more until it does or the
// send times out, nothing counts as sent without an ack, and order to that target holds. Targets do not
// wait on each other, so probing many shards takes at most one send timeout however many of them hang.
// A sender removes its target's entry and exits when its queue is empty, so idle targets cost nothing;
// a hung target's backlog grows in memory until it drains, by design.
//
// enqueue, drain and stop must be called from the tick goroutine, which owns queued.
type interShard struct {
	address *micro.ServiceAddress
	client  *micro.Client
	inbox   *command.Manager // Shared with client commands
	log     zerolog.Logger
	queued  []queuedCommand

	mu      sync.Mutex              // Guards senders and every targetQueue; never held during a send
	senders map[string]*targetQueue // Exactly the targets whose sender is running
	running sync.WaitGroup          // Running senders, for stop
}

// targetQueue holds one target's drained batches, oldest first.
type targetQueue struct {
	pending [][]queuedCommand
}

// queuedCommand keeps the enqueuing span as parent, so the send joins the tick's trace after the tick ends.
type queuedCommand struct {
	parent oteltrace.SpanContext
	cmd    *iscv1.Command
}

func newInterShard(
	address *micro.ServiceAddress, client *micro.Client, inbox *command.Manager, log zerolog.Logger,
) *interShard {
	return &interShard{
		address: address,
		client:  client,
		inbox:   inbox,
		log:     log,
		senders: make(map[string]*targetQueue),
	}
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

func (s *interShard) enqueue(ctx context.Context, cmd *iscv1.Command) {
	s.queued = append(s.queued, queuedCommand{parent: oteltrace.SpanContextFromContext(ctx), cmd: cmd})
}

func (s *interShard) drain() {
	if len(s.queued) == 0 {
		return
	}
	batches := make(map[string][]queuedCommand)
	for _, c := range s.queued {
		target := micro.String(c.cmd.GetAddress())
		batches[target] = append(batches[target], c)
	}
	s.queued = nil

	s.mu.Lock()
	defer s.mu.Unlock()
	for target, batch := range batches {
		q, ok := s.senders[target]
		if !ok {
			q = &targetQueue{}
			s.senders[target] = q
			s.running.Add(1)
			go s.sendLoop(target, q)
		}
		q.pending = append(q.pending, batch)
	}
}

// sendLoop sends target's batches until its queue is empty, then removes the entry and exits. The check
// and the removal share drain's lock, so drain either appends to a queue this loop will still see or
// starts a new sender after this one has sent everything, which keeps order to the target.
func (s *interShard) sendLoop(target string, q *targetQueue) {
	defer s.running.Done()
	for {
		s.mu.Lock()
		if len(q.pending) == 0 {
			delete(s.senders, target)
			s.mu.Unlock()
			return
		}
		batch := q.pending[0]
		q.pending[0] = nil
		q.pending = q.pending[1:]
		s.mu.Unlock()

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
	done := make(chan struct{})
	go func() {
		s.running.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.log.Error().Msg("inter-shard commands not sent before the shutdown deadline")
	}
}
