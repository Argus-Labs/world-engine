package transport

import (
	"context"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	otelcodes "go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	parent  oteltrace.SpanContext
	to      *micro.ServiceAddress
	name    string
	payload []byte
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
		if err := svc.AddCommand(name, s.handle); err != nil {
			return eris.Wrapf(err, "failed to register %s command handler", name)
		}
	}
	return nil
}

// handle receives one command from another shard. The ack it returns means the handler accepted the
// command, not that it has been processed.
func (s *interShard) handle(ctx context.Context, cmd micro.Command) error {
	if err := ctx.Err(); err != nil {
		return status.Error(codes.Canceled, eris.Wrap(err, "context cancelled").Error())
	}

	oteltrace.SpanFromContext(ctx).SetAttributes(attrCommandName.String(cmd.Name))
	received := &iscv1.Command{Name: cmd.Name, Address: s.address, Payload: cmd.Payload}
	if err := s.dispatch(ctx, received, ShardSender(cmd.From)); err != nil {
		code := codes.InvalidArgument
		if eris.Is(err, errStopping) {
			code = codes.Unavailable
		}
		return status.Error(code, eris.Wrap(err, "failed to enqueue command").Error())
	}
	return nil
}

func (s *interShard) enqueue(ctx context.Context, to *micro.ServiceAddress, name string, payload []byte) {
	c := queuedCommand{parent: oteltrace.SpanContextFromContext(ctx), to: to, name: name, payload: payload}
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
		target := micro.String(c.to)
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
	ctx := oteltrace.ContextWithSpanContext(context.Background(), c.parent)
	ctx, span := trace.New(ctx, spanInterShardSend, oteltrace.WithAttributes(
		attrCommandName.String(c.name), attrCommandTarget.String(micro.String(c.to))))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, interShardSendTimeout)
	defer cancel()

	if err := s.client.SendCommand(ctx, s.address, c.to, c.name, c.payload); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "send failed")
		s.log.Error().Err(err).Str("command", c.name).Msg("inter-shard command dropped: send failed")
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
