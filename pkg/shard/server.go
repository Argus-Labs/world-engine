// Package shard serves the client-facing CardinalService protocol.
//
// The server owns every wire detail a client can observe: where a command's persona comes from,
// when a subscription takes effect, which events a subscription matches, and how a reply is
// awaited. What it does not own is what happens to a command once accepted — that is the runtime's
// job, reached through CommandSink. A runtime that queues commands for a simulation tick and one
// that executes each command in a transaction both plug in here and are indistinguishable to a
// client, because neither gets to reimplement the protocol.
//
// Wiring is two halves. Inbound, the runtime supplies a CommandSink and mounts the server's
// handler:
//
//	server, err := shard.NewServer(shard.Config{Address: addr, Commands: sink, Logger: log})
//	path, handler, err := server.Handler()
//	mux.Handle(path, authMiddleware.Wrap(handler))
//
// Outbound, the runtime publishes events as they become visible to clients:
//
//	server.PublishEvent(shard.Event{Name: "player_died", Payload: bz})
//
// PublishEvent never runs on its own, so a runtime that must not reveal an outcome before its
// state is durable decides when to call it.
package shard

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/auth"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// keepaliveInterval bounds how long an idle event stream stays silent. Load balancers drop idle
// connections, so the server sends an empty response to hold the stream open. The client SDK
// ignores a response with no event.
const keepaliveInterval = 30 * time.Second

// CommandSink accepts commands the server has accepted on a client's behalf. The server has
// already authenticated the caller, stamped the command's persona from that identity, and proven
// the command is addressed to this shard.
//
// Submit decides what "accepted" means for the runtime: queueing the command for a later tick, or
// running it now inside a transaction. Returning an error rejects the client's request.
type CommandSink interface {
	Submit(ctx context.Context, cmd *iscv1.Command) error
}

// Event is a marshaled event on its way to clients. Recipient addresses it to a single user; empty
// broadcasts it to every subscriber whose subscriptions match Name.
type Event struct {
	Name      string
	Payload   []byte
	Recipient string
}

// EventBus is the outbound half of the protocol, implemented by *Server. A runtime holds one of
// these to publish without depending on the server's concrete type.
type EventBus interface {
	PublishEvent(evt Event) error
}

// Config configures a Server.
type Config struct {
	// Address is this shard's own address. Commands and subscriptions naming a different shard are
	// rejected.
	Address *micro.ServiceAddress

	// Commands receives accepted commands. Required.
	Commands CommandSink

	// Logger records delivery failures that are not worth failing a request over.
	Logger zerolog.Logger
}

// Server implements the client-facing CardinalService.
type Server struct {
	address      *micro.ServiceAddress
	commands     CommandSink
	log          zerolog.Logger
	subscribers  map[string]*streamSubscriber
	replyWaiters map[string][]chan *iscv1.Event
	mu           sync.RWMutex
}

var (
	_ cardinalv1connect.CardinalServiceHandler = (*Server)(nil)
	_ EventBus                                 = (*Server)(nil)
)

// NewServer creates a client-facing server for the shard at cfg.Address.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Address == nil {
		return nil, eris.New("shard address is required")
	}
	if cfg.Commands == nil {
		return nil, eris.New("command sink is required")
	}

	return &Server{
		address:      cfg.Address,
		commands:     cfg.Commands,
		log:          cfg.Logger,
		subscribers:  make(map[string]*streamSubscriber),
		replyWaiters: make(map[string][]chan *iscv1.Event),
	}, nil
}

// Handler returns the mount path and HTTP handler for the CardinalService, with the tracing and
// proto-validation interceptors already applied. Callers mount it behind authentication:
//
//	path, handler, err := server.Handler()
//	mux.Handle(path, authMiddleware.Wrap(handler))
//
// The interceptors are attached here rather than left to callers because proto validation is part
// of the protocol: a runtime that mounted the handler bare would accept malformed commands the
// other runtimes reject.
func (s *Server) Handler() (string, http.Handler, error) {
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return "", nil, eris.Wrap(err, "failed to create otel interceptor")
	}

	path, handler := cardinalv1connect.NewCardinalServiceHandler(
		s,
		connect.WithInterceptors(otelInterceptor, validate.NewInterceptor()),
	)
	return path, handler, nil
}

// -------------------------------------------------------------------------------------------------
// Commands
// -------------------------------------------------------------------------------------------------

// accept authenticates the caller, stamps the command's persona from that identity, and checks the
// command is addressed to this shard. The persona is taken from the authenticated user and never
// read from the request body, so a client cannot act as another player by asking to.
func (s *Server) accept(ctx context.Context, cmd *iscv1.Command) error {
	user := auth.UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	assert.That(cmd != nil, "command should have been validated")
	assert.That(cmd.GetPersona() != nil, "command persona should have been validated")

	cmd.Persona.Id = user.ID

	if micro.String(s.address) != micro.String(cmd.GetAddress()) {
		return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.commands.Submit(ctx, cmd); err != nil {
		return connect.NewError(connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command"))
	}

	return nil
}

func (s *Server) SendCommand(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandRequest],
) (*connect.Response[cardinalv1.SendCommandResponse], error) {
	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "context cancelled"))
	default:
	}

	if err := s.accept(ctx, req.Msg.GetCommand()); err != nil {
		return nil, err
	}

	return connect.NewResponse(&cardinalv1.SendCommandResponse{}), nil
}

func (s *Server) SendCommandWithReply(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandWithReplyRequest],
) (*connect.Response[cardinalv1.SendCommandWithReplyResponse], error) {
	// The waiter must be registered before the command is submitted. A runtime that executes
	// commands synchronously publishes the reply event during Submit, and a waiter registered
	// afterwards would miss it and block until the client gives up.
	waiter := s.addReplyWaiter(req.Msg.GetEventName())
	defer s.removeReplyWaiter(req.Msg.GetEventName(), waiter)

	if err := s.accept(ctx, req.Msg.GetCommand()); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "waiting for reply event"))
	case event := <-waiter:
		return connect.NewResponse(&cardinalv1.SendCommandWithReplyResponse{Event: event}), nil
	}
}

func (s *Server) addReplyWaiter(eventName string) chan *iscv1.Event {
	waiter := make(chan *iscv1.Event, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.replyWaiters[eventName] = append(s.replyWaiters[eventName], waiter)
	return waiter
}

func (s *Server) removeReplyWaiter(eventName string, waiter chan *iscv1.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	waiters := s.replyWaiters[eventName]
	for i, current := range waiters {
		if current == waiter {
			s.replyWaiters[eventName] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(s.replyWaiters[eventName]) == 0 {
		delete(s.replyWaiters, eventName)
	}
}

// -------------------------------------------------------------------------------------------------
// Event streams
// -------------------------------------------------------------------------------------------------

type streamSubscriber struct {
	ctx    context.Context
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse]
	events map[string]struct{}
	mu     sync.Mutex
}

func (s *streamSubscriber) send(response *cardinalv1.StartEventStreamResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.stream.Send(response)
}

func (s *Server) StartEventStream(
	ctx context.Context,
	req *connect.Request[cardinalv1.StartEventStreamRequest],
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) error {
	user := auth.UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated stream context")

	subscriber, err := s.addSubscriber(ctx, user, stream)
	if err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	defer s.removeSubscriber(user)

	if err := s.checkSubscriptionAddresses(req.Msg.GetSubscriptions()); err != nil {
		return err
	}
	s.subscribeEvents(user, req.Msg.GetSubscriptions())

	if err := subscriber.send(&cardinalv1.StartEventStreamResponse{}); err != nil {
		return connect.NewError(connect.CodeInternal, eris.Wrap(err, "failed to send initial empty event to client"))
	}

	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); !eris.Is(err, context.Canceled) {
				return connect.NewError(connect.CodeCanceled, eris.Wrap(err, "stream cancelled"))
			}
			return nil
		case <-ticker.C:
			if err := subscriber.send(&cardinalv1.StartEventStreamResponse{}); err != nil {
				return err
			}
		}
	}
}

func (s *Server) SubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.SubscribeEventsRequest],
) (*connect.Response[cardinalv1.SubscribeEventsResponse], error) {
	user := auth.UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	if !s.hasSubscriber(user) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	if err := s.checkSubscriptionAddresses(req.Msg.GetSubscriptions()); err != nil {
		return nil, err
	}
	s.subscribeEvents(user, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.SubscribeEventsResponse{}), nil
}

func (s *Server) UnsubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.UnsubscribeEventsRequest],
) (*connect.Response[cardinalv1.UnsubscribeEventsResponse], error) {
	user := auth.UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	if !s.hasSubscriber(user) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	if err := s.checkSubscriptionAddresses(req.Msg.GetSubscriptions()); err != nil {
		return nil, err
	}
	s.unsubscribeEvents(user, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.UnsubscribeEventsResponse{}), nil
}

// checkSubscriptionAddresses rejects subscriptions naming a different shard.
func (s *Server) checkSubscriptionAddresses(subscriptions []*cardinalv1.EventSubscription) error {
	for _, subscription := range subscriptions {
		if micro.String(s.address) != micro.String(subscription.GetAddress()) {
			return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	return nil
}

func (s *Server) addSubscriber(
	ctx context.Context,
	user *auth.User,
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) (*streamSubscriber, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscribers[user.ID]; exists {
		return nil, eris.Errorf("user %s already has an open stream", user.ID)
	}

	subscriber := &streamSubscriber{
		ctx:    ctx,
		stream: stream,
		events: make(map[string]struct{}),
	}
	s.subscribers[user.ID] = subscriber
	return subscriber, nil
}

func (s *Server) removeSubscriber(user *auth.User) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscribers, user.ID)
}

func (s *Server) subscribeEvents(user *auth.User, subscriptions []*cardinalv1.EventSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[user.ID]
	assert.That(subscriber != nil, "subscriber should exist for authenticated stream")

	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			subscriber.events[eventName] = struct{}{}
		}
	}
}

func (s *Server) unsubscribeEvents(user *auth.User, subscriptions []*cardinalv1.EventSubscription) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[user.ID]
	assert.That(subscriber != nil, "subscriber should exist for authenticated stream")

	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			delete(subscriber.events, eventName)
		}
	}
}

func (s *Server) hasSubscriber(user *auth.User) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.subscribers[user.ID]
	return ok
}

// -------------------------------------------------------------------------------------------------
// Event publishing
// -------------------------------------------------------------------------------------------------

// PublishEvent delivers evt to everyone waiting for it: any SendCommandWithReply call awaiting an
// event of this name, and every stream subscriber whose subscriptions match it.
//
// A subscriber whose stream is closed or failing is skipped and logged rather than failing the
// call, since one broken client must not stop an event reaching the rest.
//
//nolint:gocognit // Put everything here so you can understand the logic in one place.
func (s *Server) PublishEvent(evt Event) error {
	eventPb := &iscv1.Event{
		Name:    evt.Name,
		Payload: evt.Payload,
	}

	s.mu.RLock()
	var subscribers []*streamSubscriber
	//nolint:nestif // It's fine
	if evt.Recipient != "" {
		if subscriber, exists := s.subscribers[evt.Recipient]; exists {
			for subscription := range subscriber.events {
				if matchesEvent(subscription, eventPb.GetName()) {
					subscribers = []*streamSubscriber{subscriber}
					break
				}
			}
		} else {
			s.log.Debug().Str("recipient", evt.Recipient).Str("event", eventPb.GetName()).Msg("recipient has no open stream")
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
	waiters := append([]chan *iscv1.Event(nil), s.replyWaiters[eventPb.GetName()]...)
	s.mu.RUnlock()

	// Send events for SendCommandWithReply channels.
	for _, waiter := range waiters {
		select {
		case waiter <- eventPb:
		default:
		}
	}

	// Send events to stream subscribers.
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
			s.log.Error().Err(err).Str("event", eventPb.GetName()).Msg("failed to send event to subscriber")
			continue
		}
	}

	return nil
}

// matchesEvent reports whether a subscription covers an event name. "*" and ">" match everything,
// a "prefix.>" subscription matches any event under that prefix, and anything else must match
// exactly.
func matchesEvent(subscription string, eventName string) bool {
	return subscription == eventName ||
		subscription == "*" ||
		subscription == ">" ||
		(strings.HasSuffix(subscription, ".>") && strings.HasPrefix(eventName, strings.TrimSuffix(subscription, ">")))
}
