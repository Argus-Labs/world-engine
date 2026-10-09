package transport

import (
	"context"
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
	"github.com/rs/zerolog"
	otelcodes "go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// clientService implements CardinalService: commands from clients, event streams, and publishing.
type clientService struct {
	address      *micro.ServiceAddress
	replyTimeout time.Duration // Zero means SendCommandWithReply waits without a cap
	dispatch     Handler
	log          zerolog.Logger
	subscribers  map[string]*streamSubscriber
	replyWaiters map[string]map[string][]chan *iscv1.Event // event name -> player ID -> waiters
	mu           sync.RWMutex
	stopped      chan struct{} // Closed by stop; ends open event streams and reply waits
	stopOnce     sync.Once
}

var _ cardinalv1connect.CardinalServiceHandler = (*clientService)(nil)

func newClientService(
	address *micro.ServiceAddress, replyTimeout time.Duration, dispatch Handler, log zerolog.Logger,
) *clientService {
	return &clientService{
		address:      address,
		replyTimeout: replyTimeout,
		dispatch:     dispatch,
		log:          log,
		subscribers:  make(map[string]*streamSubscriber),
		replyWaiters: make(map[string]map[string][]chan *iscv1.Event),
		stopped:      make(chan struct{}),
	}
}

// stop ends every open event stream and reply wait, and any started later, as unavailable.
func (s *clientService) stop() {
	s.stopOnce.Do(func() { close(s.stopped) })
}

// dispatchError reports a dispatch error to the client: unavailable during shutdown, invalid otherwise.
func dispatchError(err error) error {
	code := connect.CodeInvalidArgument
	if eris.Is(err, errStopping) {
		code = connect.CodeUnavailable
	}
	return connect.NewError(code, eris.Wrap(err, "failed to enqueue command"))
}

// -------------------------------------------------------------------------------------------------
// Command handlers
// -------------------------------------------------------------------------------------------------

type streamSubscriber struct {
	ctx    context.Context
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse]
	events map[string]struct{}
	mu     sync.Mutex // Held for each write, so close waits for one in progress
	closed bool       // Set by close; send skips afterwards
}

// send writes response to the stream, or does nothing once the stream is closed.
func (s *streamSubscriber) send(response *cardinalv1.StartEventStreamResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	return s.stream.Send(response)
}

// close stops all later sends. StartEventStream calls it before returning: connect-go finishes the stream
// once the handler returns, and a publish that picked this subscriber earlier may still be sending.
func (s *streamSubscriber) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
}

func (s *clientService) SendCommand(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandRequest],
) (*connect.Response[cardinalv1.SendCommandResponse], error) {
	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "context cancelled"))
	default:
	}

	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")

	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID), attrCommandName.String(cmd.GetName()))

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.dispatch(ctx, cmd, PlayerSender(player.ID)); err != nil {
		return nil, dispatchError(err)
	}

	return connect.NewResponse(&cardinalv1.SendCommandResponse{}), nil
}

func (s *clientService) SendCommandWithReply(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandWithReplyRequest],
) (*connect.Response[cardinalv1.SendCommandWithReplyResponse], error) {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")

	span := oteltrace.SpanFromContext(ctx)
	span.SetAttributes(semconv.EnduserID(player.ID), attrCommandName.String(cmd.GetName()),
		attrEventName.String(req.Msg.GetEventName()))

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	// Register the waiter before dispatching: a handler that replies before returning would otherwise
	// publish before anyone waits.
	waiter := s.addReplyWaiter(player.ID, req.Msg.GetEventName())
	defer s.removeReplyWaiter(player.ID, req.Msg.GetEventName(), waiter)

	if err := s.dispatch(ctx, cmd, PlayerSender(player.ID)); err != nil {
		return nil, dispatchError(err)
	}

	// The span's duration is the round trip; this event marks where the enqueue ended and the wait
	// for the reply began. A cancelled wait ends the span with only this event and an error status.
	span.AddEvent("command enqueued")
	// The cap starts after dispatch, so it bounds only the wait, not the handler. Without one, timeout
	// stays nil and never fires.
	var timeout <-chan time.Time
	if s.replyTimeout > 0 {
		timer := time.NewTimer(s.replyTimeout)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "waiting for reply event"))
	case <-s.stopped:
		return nil, connect.NewError(connect.CodeUnavailable, eris.Wrap(errStopping, "waiting for reply event"))
	case <-timeout:
		// The client is still waiting, so no reply within the cap is a server-side bug.
		s.log.Error().
			Str("command", cmd.GetName()).
			Str("event", req.Msg.GetEventName()).
			Str("player", player.ID).
			Dur("waited", s.replyTimeout).
			Msg("no reply event emitted for command")
		return nil, connect.NewError(connect.CodeDeadlineExceeded,
			eris.Errorf("no %s emitted within %s", req.Msg.GetEventName(), s.replyTimeout))
	case event := <-waiter:
		span.AddEvent("reply received")
		return connect.NewResponse(&cardinalv1.SendCommandWithReplyResponse{Event: event}), nil
	}
}

func (s *clientService) addReplyWaiter(playerID, eventName string) chan *iscv1.Event {
	waiter := make(chan *iscv1.Event, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	byPlayer := s.replyWaiters[eventName]
	if byPlayer == nil {
		byPlayer = make(map[string][]chan *iscv1.Event)
		s.replyWaiters[eventName] = byPlayer
	}
	byPlayer[playerID] = append(byPlayer[playerID], waiter)
	return waiter
}

func (s *clientService) removeReplyWaiter(playerID, eventName string, waiter chan *iscv1.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	byPlayer := s.replyWaiters[eventName]
	waiters := byPlayer[playerID]
	for i, current := range waiters {
		if current == waiter {
			byPlayer[playerID] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(byPlayer[playerID]) == 0 {
		delete(byPlayer, playerID)
	}
	if len(byPlayer) == 0 {
		delete(s.replyWaiters, eventName)
	}
}

// -------------------------------------------------------------------------------------------------
// Event streams
// -------------------------------------------------------------------------------------------------

func (s *clientService) StartEventStream(
	ctx context.Context,
	req *connect.Request[cardinalv1.StartEventStreamRequest],
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) error {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated stream context")
	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID),
		attrEventSubscriptions.Int(countSubscriptions(req.Msg.GetSubscriptions())))

	subscriber, err := s.addSubscriber(ctx, player, stream)
	if err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	defer subscriber.close()
	defer s.removeSubscriber(player)

	for _, subscription := range req.Msg.GetSubscriptions() {
		if micro.String(s.address) != micro.String(subscription.GetAddress()) {
			return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	s.subscribeEvents(player, req.Msg.GetSubscriptions())

	if err := subscriber.send(&cardinalv1.StartEventStreamResponse{}); err != nil {
		return connect.NewError(connect.CodeInternal, eris.Wrap(err, "failed to send initial empty event to client"))
	}

	// Send periodic keepalive messages to prevent ALB idle timeouts.
	// Empty responses are safely ignored by the client SDK (ShardEvent.Name is null → HandleEvent skips it).
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); !eris.Is(err, context.Canceled) {
				return connect.NewError(connect.CodeCanceled, eris.Wrap(err, "stream cancelled"))
			}
			return nil
		case <-s.stopped:
			return connect.NewError(connect.CodeUnavailable, errStopping)
		case <-ticker.C:
			if err := subscriber.send(&cardinalv1.StartEventStreamResponse{}); err != nil {
				return err
			}
		}
	}
}

func (s *clientService) SubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.SubscribeEventsRequest],
) (*connect.Response[cardinalv1.SubscribeEventsResponse], error) {
	player, err := s.subscriptionRequest(ctx, req.Msg.GetSubscriptions())
	if err != nil {
		return nil, err
	}
	s.subscribeEvents(player, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.SubscribeEventsResponse{}), nil
}

func (s *clientService) UnsubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.UnsubscribeEventsRequest],
) (*connect.Response[cardinalv1.UnsubscribeEventsResponse], error) {
	player, err := s.subscriptionRequest(ctx, req.Msg.GetSubscriptions())
	if err != nil {
		return nil, err
	}
	s.unsubscribeEvents(player, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.UnsubscribeEventsResponse{}), nil
}

// subscriptionRequest validates a subscribe or unsubscribe request from a player with an open stream.
func (s *clientService) subscriptionRequest(
	ctx context.Context, subscriptions []*cardinalv1.EventSubscription,
) (*Player, error) {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")
	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID),
		attrEventSubscriptions.Int(countSubscriptions(subscriptions)))

	if !s.hasSubscriber(player) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range subscriptions {
		if micro.String(s.address) != micro.String(subscription.GetAddress()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	return player, nil
}

func (s *clientService) addSubscriber(
	ctx context.Context,
	player *Player,
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) (*streamSubscriber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscribers[player.ID]; exists {
		return nil, eris.Errorf("player %s already has an open stream", player.ID)
	}

	subscriber := &streamSubscriber{
		ctx:    ctx,
		stream: stream,
		events: make(map[string]struct{}),
	}
	s.subscribers[player.ID] = subscriber
	return subscriber, nil
}

func (s *clientService) removeSubscriber(player *Player) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscribers, player.ID)
}

func (s *clientService) subscribeEvents(player *Player, subscriptions []*cardinalv1.EventSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[player.ID]
	assert.That(subscriber != nil, "subscriber should exist for authenticated stream")

	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			subscriber.events[eventName] = struct{}{}
		}
	}
}

func (s *clientService) unsubscribeEvents(player *Player, subscriptions []*cardinalv1.EventSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[player.ID]
	assert.That(subscriber != nil, "subscriber should exist for authenticated stream")

	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			delete(subscriber.events, eventName)
		}
	}
}

func (s *clientService) hasSubscriber(player *Player) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.subscribers[player.ID]
	return ok
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

	// Send events to stream subscribers. A failed send is logged and recorded on the span but does not
	// fail the publish: one dead stream must not block delivery to the others.
	sendFailures := 0
	for _, subscriber := range subscribers {
		select {
		case <-subscriber.ctx.Done():
			continue
		default:
		}

		err := subscriber.send(&cardinalv1.StartEventStreamResponse{
			Address: s.address,
			Event:   eventPb,
		})
		if err != nil {
			sendFailures++
			span.RecordError(err)
			s.log.Error().Err(err).Str("event", eventPb.GetName()).Msg("failed to send event to subscriber")
			continue
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
