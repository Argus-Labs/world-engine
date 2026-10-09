package transport

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	otelcodes "go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
)

type streamSubscriber struct {
	ctx    context.Context
	cancel context.CancelCauseFunc // Ends the stream's handler; the cause says why (superseded, lagged).
	events map[string]struct{}     // Guarded by clientService.mu.
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse]
	lagged *time.Timer // Armed during each write; ends the stream as lagged when it fires.

	// Events waiting for the stream's handler, its only writer, which drains them in order.
	queueMu     sync.Mutex
	queue       []queuedEvent // Guarded by queueMu.
	queuedBytes int           // Guarded by queueMu. Counts events until they are written.
	queueReady  chan struct{} // Signalled after each enqueue; capacity 1.
}

type queuedEvent struct {
	response *cardinalv1.StartEventStreamResponse
	queuedAt time.Time
	size     int // Charged against streamQueueBytes until written.
}

// enqueue hands an event of size bytes to the stream's writer without blocking. A stream already
// holding streamQueueBytes of unwritten events ends as lagged instead, so a stopped reader cannot
// grow without bound. A reader that is merely behind a large tick is ended by age, not by this
// limit: see streamLagLimit.
func (s *streamSubscriber) enqueue(response *cardinalv1.StartEventStreamResponse, size int) error {
	size += queuedEventOverhead
	s.queueMu.Lock()
	if s.queuedBytes+size > streamQueueBytes {
		s.queueMu.Unlock()
		s.cancel(errStreamLagged)
		return errStreamLagged
	}
	s.queue = append(s.queue, queuedEvent{response: response, queuedAt: time.Now(), size: size})
	s.queuedBytes += size
	s.queueMu.Unlock()

	select {
	case s.queueReady <- struct{}{}:
	default: // The writer is already signalled.
	}
	return nil
}

// writeQueued writes every waiting event, oldest first, releasing each one's charge once written.
// It stops at a failed write, or once the stream is cancelled, leaving the rest unwritten.
func (s *streamSubscriber) writeQueued() error {
	s.queueMu.Lock()
	queued := s.queue
	s.queue = nil
	s.queueMu.Unlock()

	for _, evt := range queued {
		if s.ctx.Err() != nil { // Superseded, lagged, or closed.
			return streamEndError(s.ctx)
		}
		if err := s.write(evt.response, evt.queuedAt); err != nil {
			return err
		}
		s.queueMu.Lock()
		s.queuedBytes -= evt.size
		s.queueMu.Unlock()
	}
	return nil
}

// write sends one response; only the stream's handler calls it. A write still pending
// streamLagLimit after its event was queued ends the stream as lagged, and the cancellation then
// forces the write to fail after streamTeardownGrace.
func (s *streamSubscriber) write(response *cardinalv1.StartEventStreamResponse, queuedAt time.Time) error {
	s.lagged.Reset(streamLagLimit - time.Since(queuedAt))
	defer s.lagged.Stop()
	if err := s.stream.Send(response); err != nil {
		if s.ctx.Err() != nil {
			return streamEndError(s.ctx)
		}
		return err
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Event streams
// -------------------------------------------------------------------------------------------------

// errStreamSuperseded is the cancel cause of an event stream replaced by a newer stream for the
// same player. Clients see it as CodeAborted and may reconnect.
var errStreamSuperseded = eris.New("event stream superseded by a newer stream for this player")

// errStreamLagged is the cancel cause of an event stream whose client fell too far behind: its
// queue was full, or an event waited streamLagLimit to be written. Clients see it as
// CodeUnavailable and may reconnect.
var errStreamLagged = eris.New("event stream lagged: the client is not reading events fast enough")

// streamTeardownGrace is how long a cancelled stream may stay blocked writing before its response
// is forced closed. See withStreamWriteDeadline.
const streamTeardownGrace = time.Second

// streamQueueBytes bounds the unwritten events one stream may hold, in encoded bytes plus
// queuedEventOverhead each. It is a memory guard, not the lag signal: one tick can queue thousands
// of events for a stream (a resync, a large wave) faster than any writer drains them, and a reader
// that keeps up must survive that. A reader that stopped normally ends by streamLagLimit first;
// this bound ends it sooner only when more than 16 MiB is sent to it within that time.
const streamQueueBytes = 16 << 20

// queuedEventOverhead is charged per queued event on top of its encoded size, for the queue entry
// and response wrapper, so a burst of empty events is bounded too.
const queuedEventOverhead = 128

// streamLagLimit is how long an event may wait, queued or mid-write, before its stream ends as
// lagged. The writer drains oldest first and times each write from its event's enqueue, so the
// oldest unwritten event always carries the deadline.
const streamLagLimit = 5 * time.Second

// StartEventStream opens a player's event stream. The newest stream wins: opening one while the
// player already has a stream replaces it, and the old one ends with CodeAborted ("superseded").
// A stream whose client falls behind ends with CodeUnavailable ("lagged"); see streamLagLimit.
func (s *clientService) StartEventStream(
	ctx context.Context,
	req *connect.Request[cardinalv1.StartEventStreamRequest],
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) error {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated stream context")
	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID),
		attrEventSubscriptions.Int(countSubscriptions(req.Msg.GetSubscriptions())))

	for _, subscription := range req.Msg.GetSubscriptions() {
		if micro.String(s.address) != micro.String(subscription.GetAddress()) {
			return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// Once the stream is cancelled, for any reason, give its pending writes a grace period, then
	// force them to fail. Otherwise a client that stopped reading pins this handler in a
	// flow-controlled write forever.
	if deadline := streamWriteDeadlineFromContext(ctx); deadline != nil {
		context.AfterFunc(ctx, func() { deadline.expireAfter(streamTeardownGrace) })
	}

	// The initial subscriptions belong to this stream, so they are set before it is published.
	subscriber := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     make(map[string]struct{}),
		stream:     stream,
		lagged:     time.AfterFunc(streamLagLimit, func() { cancel(errStreamLagged) }),
		queueReady: make(chan struct{}, 1),
	}
	subscriber.lagged.Stop()
	defer subscriber.lagged.Stop()
	addSubscriptions(subscriber, req.Msg.GetSubscriptions())
	s.addSubscriber(player.ID, subscriber)
	defer s.removeSubscriber(player.ID, subscriber)

	// This handler is the stream's only writer, so writes keep queue order and none follows its
	// return.
	if err := subscriber.write(&cardinalv1.StartEventStreamResponse{}, time.Now()); err != nil {
		if ctx.Err() != nil {
			return err
		}
		return connect.NewError(connect.CodeInternal, eris.Wrap(err, "failed to send initial empty event to client"))
	}

	// Send periodic keepalive messages to prevent ALB idle timeouts.
	// Empty responses are safely ignored by the client SDK (ShardEvent.Name is null → HandleEvent skips it).
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return streamEndError(ctx)
		case <-s.stopped:
			return connect.NewError(connect.CodeUnavailable, errStopping)
		case <-subscriber.queueReady:
			if err := subscriber.writeQueued(); err != nil {
				return err
			}
		case <-ticker.C:
			if err := subscriber.write(&cardinalv1.StartEventStreamResponse{}, time.Now()); err != nil {
				return err
			}
		}
	}
}

// streamEndError is the status a cancelled stream's handler returns.
func streamEndError(ctx context.Context) error {
	switch cause := context.Cause(ctx); {
	case eris.Is(cause, errStreamSuperseded):
		return connect.NewError(connect.CodeAborted, cause)
	case eris.Is(cause, errStreamLagged):
		return connect.NewError(connect.CodeUnavailable, cause)
	case !eris.Is(ctx.Err(), context.Canceled):
		return connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "stream cancelled"))
	default:
		return nil
	}
}

func (s *clientService) SubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.SubscribeEventsRequest],
) (*connect.Response[cardinalv1.SubscribeEventsResponse], error) {
	if err := s.updateSubscriptions(ctx, req.Msg.GetSubscriptions(), addSubscriptions); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cardinalv1.SubscribeEventsResponse{}), nil
}

func (s *clientService) UnsubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.UnsubscribeEventsRequest],
) (*connect.Response[cardinalv1.UnsubscribeEventsResponse], error) {
	if err := s.updateSubscriptions(ctx, req.Msg.GetSubscriptions(), removeSubscriptions); err != nil {
		return nil, err
	}
	return connect.NewResponse(&cardinalv1.UnsubscribeEventsResponse{}), nil
}

// updateSubscriptions applies a subscribe or unsubscribe request to the player's current stream.
// The lookup and the update share one lock, so a request racing a stream's replacement or close
// either updates the stream registered at that moment or fails with CodeFailedPrecondition.
func (s *clientService) updateSubscriptions(
	ctx context.Context,
	subscriptions []*cardinalv1.EventSubscription,
	update func(*streamSubscriber, []*cardinalv1.EventSubscription),
) error {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")
	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID),
		attrEventSubscriptions.Int(countSubscriptions(subscriptions)))

	for _, subscription := range subscriptions {
		if micro.String(s.address) != micro.String(subscription.GetAddress()) {
			return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber, ok := s.subscribers[player.ID]
	if !ok {
		return connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}
	update(subscriber, subscriptions)
	return nil
}

// addSubscriber registers subscriber as the player's stream and cancels the stream it replaces.
func (s *clientService) addSubscriber(playerID string, subscriber *streamSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if previous, ok := s.subscribers[playerID]; ok {
		previous.cancel(errStreamSuperseded)
	}
	s.subscribers[playerID] = subscriber
}

// removeSubscriber unregisters subscriber unless a newer stream has already replaced it.
func (s *clientService) removeSubscriber(playerID string, subscriber *streamSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.subscribers[playerID] == subscriber {
		delete(s.subscribers, playerID)
	}
}

// addSubscriptions and removeSubscriptions require clientService.mu, or a subscriber not yet registered.
func addSubscriptions(subscriber *streamSubscriber, subscriptions []*cardinalv1.EventSubscription) {
	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			subscriber.events[eventName] = struct{}{}
		}
	}
}

func removeSubscriptions(subscriber *streamSubscriber, subscriptions []*cardinalv1.EventSubscription) {
	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			delete(subscriber.events, eventName)
		}
	}
}

// countSubscriptions returns the number of event names across all subscriptions.
func countSubscriptions(subscriptions []*cardinalv1.EventSubscription) int {
	n := 0
	for _, subscription := range subscriptions {
		n += len(subscription.GetEvents())
	}
	return n
}

// -------------------------------------------------------------------------------------------------
// Event publishers
// -------------------------------------------------------------------------------------------------

// TODO: move away from this centralized approach to a actor model for easier(?) synchronization.

//nolint:gocognit // Put everything here so you can understand the logic in one place.
func (s *clientService) publish(ctx context.Context, evt Payload, recipient string) {
	// Ended by a direct defer so an encoding panic below is recorded on this span, which is the
	// one that names the event.
	_, span := trace.New(ctx, spanEventPublish, oteltrace.WithAttributes(
		attrEventName.String(evt.Name()), attrEventRecipient.String(recipient)))
	defer span.End()

	eventPb := &iscv1.Event{
		Name:    evt.Name(),
		Payload: evt.AppendWire(make([]byte, 0, evt.SizeWire())),
	}

	s.mu.RLock()
	var subscribers []*streamSubscriber
	//nolint:nestif // It's fine
	if recipient != "" {
		if subscriber, exists := s.subscribers[recipient]; exists {
			for subscription := range subscriber.events {
				if matchesEvent(subscription, eventPb.GetName()) {
					subscribers = []*streamSubscriber{subscriber}
					break
				}
			}
		} else {
			s.log.Debug().
				Str("recipient", recipient).
				Str("event", eventPb.GetName()).
				Msg("recipient has no open stream")
		}
	} else {
		subscribers = make([]*streamSubscriber, 0, len(s.subscribers))
		for _, subscriber := range s.subscribers {
			for subscription := range subscriber.events {
				if matchesEvent(subscription, eventPb.GetName()) {
					subscribers = append(subscribers, subscriber)
					break
				}
			}
		}
	}
	// A targeted reply resolves only its recipient's waiters. A broadcast is visible to every
	// player, so it resolves every waiter for the event name.
	var waiters []chan *iscv1.Event
	if recipient != "" {
		waiters = append(waiters, s.replyWaiters[eventPb.GetName()][recipient]...)
	} else {
		for _, playerWaiters := range s.replyWaiters[eventPb.GetName()] {
			waiters = append(waiters, playerWaiters...)
		}
	}
	s.mu.RUnlock()
	span.SetAttributes(attrEventSubscribers.Int(len(subscribers)), attrEventWaiters.Int(len(waiters)))

	// Send events for SendCommandWithReply channels.
	for _, waiter := range waiters {
		select {
		case waiter <- eventPb:
		default:
		}
	}

	// Queue events for stream subscribers. Queuing never blocks: a stream too far behind ends as
	// lagged and is recorded on the span, and delivery to the others continues.
	eventSize := proto.Size(eventPb)
	sendFailures := 0
	for _, subscriber := range subscribers {
		select {
		case <-subscriber.ctx.Done():
			continue
		default:
		}

		err := subscriber.enqueue(&cardinalv1.StartEventStreamResponse{
			Address: s.address,
			Event:   eventPb,
		}, eventSize)
		if err != nil {
			sendFailures++
			span.RecordError(err)
			s.log.Warn().Err(err).Str("event", eventPb.GetName()).Msg("ended a lagging event stream")
		}
	}
	span.SetAttributes(attrEventSendFailures.Int(sendFailures))
	if sendFailures > 0 {
		span.SetStatus(otelcodes.Error, "some subscriber sends failed")
	}
}

func matchesEvent(subscription string, eventName string) bool {
	return subscription == eventName ||
		subscription == "*" ||
		subscription == ">" ||
		(strings.HasSuffix(subscription, ".>") && strings.HasPrefix(eventName, strings.TrimSuffix(subscription, ">")))
}

// -------------------------------------------------------------------------------------------------
// Forced stream teardown
// -------------------------------------------------------------------------------------------------

// Cancelling a stream handler's context does not unblock a write in progress. Over HTTP/2, a
// response write to a client that stopped reading waits for flow-control window that never
// arrives, so a superseded or lagged stream could stay blocked forever. Connect hides the
// http.ResponseWriter, so this middleware captures an http.ResponseController for each
// event stream request. After the grace period, StartEventStream sets a write deadline in the past:
// the HTTP/2 server then resets the stream (RST_STREAM) and every blocked or later write on it
// fails. Over HTTP/1.1 the deadline applies to the connection, so every write on it fails.
// A client whose whole TCP connection is stalled is not covered: the reset frame itself cannot be
// written, so only the connection's own failure releases the writes.

type streamWriteDeadlineKey struct{}

// withStreamWriteDeadline makes a streamWriteDeadline available to StartEventStream handlers.
func withStreamWriteDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cardinalv1connect.CardinalServiceStartEventStreamProcedure {
			next.ServeHTTP(w, r)
			return
		}
		deadline := &streamWriteDeadline{controller: http.NewResponseController(w)}
		defer deadline.handlerDone()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), streamWriteDeadlineKey{}, deadline)))
	})
}

func streamWriteDeadlineFromContext(ctx context.Context) *streamWriteDeadline {
	deadline, _ := ctx.Value(streamWriteDeadlineKey{}).(*streamWriteDeadline)
	return deadline
}

// streamWriteDeadline forces one response's writes to fail. A ResponseController must not be used
// after its handler returns, so the timer only touches it while the handler is still running.
type streamWriteDeadline struct {
	mu         sync.Mutex
	controller *http.ResponseController
	timer      *time.Timer
	done       bool
}

// expireAfter forces the response's writes to fail after grace, unless the handler returns first.
func (d *streamWriteDeadline) expireAfter(grace time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.done || d.timer != nil {
		return
	}
	d.timer = time.AfterFunc(grace, d.expire)
}

func (d *streamWriteDeadline) expire() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.done {
		return
	}
	// A deadline in the past fails the pending write immediately (HTTP/2 resets the stream).
	_ = d.controller.SetWriteDeadline(time.Unix(1, 0))
}

func (d *streamWriteDeadline) handlerDone() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.done = true
	if d.timer != nil {
		d.timer.Stop()
	}
}
