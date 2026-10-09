package transport

import (
	"context"
	"sync"
	"time"

	"buf.build/go/protovalidate"
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
// caller ever waiting on NATS. Received commands go to the dispatch handler; outgoing commands are staged
// by enqueue and handed to the senders by drain.
//
// Each target with unsent commands has one sender goroutine, which sends them one at a time and waits
// for each ack before the next. A target that cannot ack one command gets no more until it does or the
// send times out, nothing counts as sent without an ack, and order to that target holds. Targets do not
// wait on each other, so probing many shards takes at most one send timeout however many of them hang.
// A sender removes its target's entry and exits when its queue is empty, so idle targets cost nothing;
// a hung target's backlog grows in memory until it drains, by design.
//
// enqueue and drain may be called from any goroutine: a service that handles commands concurrently
// sends from inside its handlers. queued sits behind mu rather than a channel: a channel's fixed capacity
// would make enqueue wait or drop once full, and mu is already taken by drain.
type interShard struct {
	address  *micro.ServiceAddress
	client   *micro.Client
	dispatch Handler // Same as client commands
	log      zerolog.Logger

	mu      sync.Mutex              // Guards queued, stopped, senders and every targetQueue; never held during a send
	queued  []queuedCommand         // Enqueued, not yet drained
	stopped bool                    // Set by stop; drain drops commands afterwards
	senders map[string]*targetQueue // Exactly the targets whose sender is running
	running sync.WaitGroup          // Running senders, for stop
}

// targetQueue holds one target's drained batches, oldest first.
type targetQueue struct {
	pending [][]queuedCommand
}

// queuedCommand keeps the enqueuing span as parent, so the send joins the enqueuer's trace after it ends.
type queuedCommand struct {
	parent oteltrace.SpanContext
	isc    *iscv1.InterShardCommand
}

func newInterShard(
	address *micro.ServiceAddress, client *micro.Client, dispatch Handler, log zerolog.Logger,
) *interShard {
	return &interShard{
		address:  address,
		client:   client,
		dispatch: dispatch,
		log:      log,
		senders:  make(map[string]*targetQueue),
	}
}

func (s *interShard) start(svc *micro.Service, handlers map[string]Handler) error {
	for name := range handlers {
		if err := svc.AddGroup("command").AddEndpoint(name, s.handle); err != nil {
			return eris.Wrapf(err, "failed to register %s command handler", name)
		}
	}
	return nil
}

// handle receives one command from another shard. The ack it returns means the handler accepted the
// command, not that it has been processed.
func (s *interShard) handle(ctx context.Context, req *micro.Request) *micro.Response {
	select {
	case <-ctx.Done():
		return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
	default:
	}

	isc := &iscv1.InterShardCommand{}
	if err := req.Payload.UnmarshalTo(isc); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.InvalidArgument)
	}

	if err := protovalidate.Validate(isc); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to validate command"), codes.InvalidArgument)
	}
	cmd := isc.GetCommand()

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return micro.NewErrorResponse(
			req,
			eris.New("command address doesn't match shard address"),
			codes.InvalidArgument,
		)
	}

	oteltrace.SpanFromContext(ctx).SetAttributes(attrCommandName.String(cmd.GetName()))
	if err := s.dispatch(ctx, cmd, ShardSender(isc.GetSender())); err != nil {
		code := codes.InvalidArgument
		if eris.Is(err, errStopping) {
			code = codes.Unavailable
		}
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), code)
	}

	return micro.NewSuccessResponse(req, nil)
}

func (s *interShard) enqueue(ctx context.Context, isc *iscv1.InterShardCommand) {
	c := queuedCommand{parent: oteltrace.SpanContextFromContext(ctx), isc: isc}
	s.mu.Lock()
	s.queued = append(s.queued, c)
	s.mu.Unlock()
}

// drain hands every queued command to its target's sender. Taking queued and handing it over happen under
// one lock: if two drains could interleave there, the later one could hand over its commands first, and a
// goroutine's commands to one target would go out of order.
func (s *interShard) drain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queued) == 0 {
		return
	}
	if s.stopped {
		s.log.Error().Int("commands", len(s.queued)).Msg("inter-shard commands dropped: flushed after stop")
		s.queued = nil
		return
	}
	batches := make(map[string][]queuedCommand)
	for _, c := range s.queued {
		target := micro.String(c.isc.GetCommand().GetAddress())
		batches[target] = append(batches[target], c)
	}
	s.queued = nil

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
	// span (and whatever processes the command there) joins the enqueuer's trace.
	cmd := c.isc.GetCommand()
	ctx := oteltrace.ContextWithSpanContext(context.Background(), c.parent)
	ctx, span := trace.New(ctx, spanInterShardSend, oteltrace.WithAttributes(
		attrCommandName.String(cmd.GetName()), attrCommandTarget.String(micro.String(cmd.GetAddress()))))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, interShardSendTimeout)
	defer cancel()

	if _, err := s.client.Request(ctx, cmd.GetAddress(), "command."+cmd.GetName(), c.isc); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "send failed")
		s.log.Error().Err(err).Str("command", cmd.GetName()).Msg("inter-shard command dropped: send failed")
	}
}

// stop waits until ctx for drained commands to be sent. Commands enqueued but not drained are dropped,
// and so are commands drained after stop begins.
func (s *interShard) stop(ctx context.Context) {
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()

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
