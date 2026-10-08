package cardinal

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry/trace"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	otelcodes "go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// service hosts the direct client-facing Cardinal service.
type service struct {
	world        *World
	server       *http.Server
	log          zerolog.Logger
	authMode     AuthMode
	argusAuthURL string
	client       *micro.Client
	microService *micro.Service
	interShard   *interShard
	commands     map[string]struct{}
	subscribers  map[string]*streamSubscriber
	replyWaiters map[string]map[string][]chan *iscv1.Event // event name -> player ID -> waiters
	mu           sync.RWMutex
}

var _ cardinalv1connect.CardinalServiceHandler = (*service)(nil)

// newService creates a new direct client-facing Cardinal service.
func newService(world *World, authMode AuthMode, argusAuthURL string) *service {
	return &service{
		world:        world,
		log:          world.tel.GetLogger("service"),
		authMode:     authMode,
		argusAuthURL: argusAuthURL,
		commands:     make(map[string]struct{}),
		subscribers:  make(map[string]*streamSubscriber),
		replyWaiters: make(map[string]map[string][]chan *iscv1.Event),
	}
}

// h2cProtocols enables HTTP/1.1 and unencrypted HTTP/2 (h2c), matching the
// previous h2c.NewHandler(mux, &http2.Server{}) behavior without the deprecated
// golang.org/x/net/http2/h2c package.
func h2cProtocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}

func (s *service) init(address string) error {
	clientOpts := []micro.ClientOption{micro.WithLogger(s.world.tel.GetLogger("service"))}
	if cfg := s.world.options.NATSConfig; cfg != nil {
		clientOpts = append(clientOpts, micro.WithNATSConfig(*cfg))
	}
	client, err := micro.NewClient(clientOpts...)
	if err != nil {
		return eris.Wrap(err, "failed to initialize micro client")
	}
	s.client = client
	microService, err := micro.NewService(client, s.world.address, &s.world.tel)
	if err != nil {
		return eris.Wrap(err, "failed to create micro service")
	}
	s.microService = microService
	s.interShard = newInterShard(s.world.address, client, &s.world.commands, s.log)

	// Keep these for now cuz ISC requires a bit more work than client connections. Will need another
	// refactor after the current clients are migrated to connect directly to the shards.
	if err = s.microService.AddEndpoint("ping", s.handlePing); err != nil {
		return eris.Wrap(err, "failed to register ping handler")
	}
	if err := s.interShard.start(s.microService, s.commands); err != nil {
		return err
	}

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return eris.Wrap(err, "failed to create otel interceptor")
	}
	validateInterceptor := validate.NewInterceptor()

	mux := http.NewServeMux()

	var authenticate func(context.Context, *http.Request) (any, error)
	switch s.authMode {
	case AuthModeArgus:
		authenticator, err := NewArgusAuthenticator(
			s.argusAuthURL, s.world.options.Organization, s.world.options.Project,
		)
		if err != nil {
			return eris.Wrap(err, "failed to create argus authenticator")
		}
		authenticate = authenticator.Authenticate
	case AuthModeDev:
		authenticate = authenticatorDev{}.authenticate
	case AuthModeUndefined:
		fallthrough
	default:
		return eris.Errorf("invalid service auth mode: %s", s.authMode)
	}
	authMiddleware := authn.NewMiddleware(authenticate)

	cardinalPath, cardinalHandler := cardinalv1connect.NewCardinalServiceHandler(
		s,
		connect.WithInterceptors(
			otelInterceptor,
			validateInterceptor,
		),
	)
	mux.Handle(cardinalPath, authMiddleware.Wrap(cardinalHandler))

	if err := s.mountDebugService(mux, otelInterceptor, validateInterceptor); err != nil {
		return err
	}

	s.server = &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		Protocols:         h2cProtocols(),
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", address)
	if err != nil {
		return eris.Wrap(err, "failed to listen for service server")
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && !eris.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("service server error")
		}
	}()

	return nil
}

func (s *service) mountDebugService(mux *http.ServeMux, interceptors ...connect.Interceptor) error {
	if s.world.debug == nil {
		return nil
	}
	if err := s.world.debug.finalizeCatalog(); err != nil {
		return eris.Wrap(err, "failed to finalize introspection catalog")
	}
	debugPath, debugHandler := cardinalv1connect.NewDebugServiceHandler(
		s.world.debug,
		connect.WithInterceptors(interceptors...),
	)
	mux.Handle(debugPath, debugHandler)
	s.log.Info().Msg("DebugService mounted on client-facing port (dev)")
	return nil
}

func (s *service) shutdown(ctx context.Context) error {
	// Before server.Shutdown: an open event stream holds it until ctx expires, and its error returns early.
	if s.interShard != nil {
		s.interShard.stop(ctx)
	}
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return eris.Wrap(err, "failed to shutdown service server")
		}
	}
	if s.microService != nil {
		if err := s.microService.Close(); err != nil {
			return eris.Wrap(err, "failed to close micro service")
		}
	}
	if s.client != nil {
		s.client.Close()
	}

	return nil
}

func (s *service) registerCommandHandler(name string) {
	s.commands[name] = struct{}{}
}

// -------------------------------------------------------------------------------------------------
// Command handlers
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

func (s *service) SendCommand(
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

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.world.commands.Enqueue(ctx, cmd, command.PlayerSender(player.ID)); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command"))
	}

	return connect.NewResponse(&cardinalv1.SendCommandResponse{}), nil
}

func (s *service) SendCommandWithReply(
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

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	// Register the reply waiter before enqueuing the command. Enqueueing first opens a window
	// where a tick can drain the command, emit the reply, and find no waiter — dropping the
	// reply and deadlocking the client until its context times out.
	waiter := s.addReplyWaiter(player.ID, req.Msg.GetEventName())
	defer s.removeReplyWaiter(player.ID, req.Msg.GetEventName(), waiter)

	if err := s.world.commands.Enqueue(ctx, cmd, command.PlayerSender(player.ID)); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command"))
	}

	span.AddEvent("command enqueued")

	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "waiting for reply event"))
	case event := <-waiter:
		span.AddEvent("reply received")
		return connect.NewResponse(&cardinalv1.SendCommandWithReplyResponse{Event: event}), nil
	}
}

func (s *service) addReplyWaiter(playerID, eventName string) chan *iscv1.Event {
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

func (s *service) removeReplyWaiter(playerID, eventName string, waiter chan *iscv1.Event) {
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

func (s *service) StartEventStream(
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
	defer s.removeSubscriber(player)

	for _, subscription := range req.Msg.GetSubscriptions() {
		if micro.String(s.world.address) != micro.String(subscription.GetAddress()) {
			return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	if err := s.subscribeEvents(player, req.Msg.GetSubscriptions()); err != nil {
		return err
	}

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
		case <-ticker.C:
			if err := subscriber.send(&cardinalv1.StartEventStreamResponse{}); err != nil {
				return err
			}
		}
	}
}

func (s *service) SubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.SubscribeEventsRequest],
) (*connect.Response[cardinalv1.SubscribeEventsResponse], error) {
	player, err := s.subscriptionRequest(ctx, req.Msg.GetSubscriptions())
	if err != nil {
		return nil, err
	}
	if err := s.subscribeEvents(player, req.Msg.GetSubscriptions()); err != nil {
		return nil, err
	}

	return connect.NewResponse(&cardinalv1.SubscribeEventsResponse{}), nil
}

func (s *service) UnsubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.UnsubscribeEventsRequest],
) (*connect.Response[cardinalv1.UnsubscribeEventsResponse], error) {
	player, err := s.subscriptionRequest(ctx, req.Msg.GetSubscriptions())
	if err != nil {
		return nil, err
	}
	if err := s.unsubscribeEvents(player, req.Msg.GetSubscriptions()); err != nil {
		return nil, err
	}

	return connect.NewResponse(&cardinalv1.UnsubscribeEventsResponse{}), nil
}

// subscriptionRequest validates a subscribe or unsubscribe request from a player. It performs the
// state-independent checks (authentication and shard-address matching); the subscriber existence
// check is deferred to subscribeEvents/unsubscribeEvents, which perform it under the same write
// lock as the mutation so a concurrent removeSubscriber (from a closing stream) cannot turn a
// passing check into a nil-subscriber panic.
func (s *service) subscriptionRequest(
	ctx context.Context, subscriptions []*cardinalv1.EventSubscription,
) (*Player, error) {
	player := PlayerFromContext(ctx)
	assert.That(player != nil, "player should exist in authenticated request context")
	oteltrace.SpanFromContext(ctx).SetAttributes(semconv.EnduserID(player.ID),
		attrEventSubscriptions.Int(countSubscriptions(subscriptions)))

	for _, subscription := range subscriptions {
		if micro.String(s.world.address) != micro.String(subscription.GetAddress()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	return player, nil
}

func (s *service) addSubscriber(
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

func (s *service) removeSubscriber(player *Player) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscribers, player.ID)
}

func (s *service) subscribeEvents(player *Player, subscriptions []*cardinalv1.EventSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[player.ID]
	if subscriber == nil {
		return connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			subscriber.events[eventName] = struct{}{}
		}
	}
	return nil
}

func (s *service) unsubscribeEvents(player *Player, subscriptions []*cardinalv1.EventSubscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[player.ID]
	if subscriber == nil {
		return connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range subscriptions {
		for _, eventName := range subscription.GetEvents() {
			delete(subscriber.events, eventName)
		}
	}
	return nil
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
func (s *service) publishDefaultEvent(ctx context.Context, evt event.Event) error {
	payload, ok := evt.Payload.(event.Payload)
	if !ok {
		return eris.Errorf("invalid event payload type: %T", evt.Payload)
	}

	// Ended by a direct defer so an encoding panic below is recorded on this span, which is the
	// one that names the event. Nothing after this point returns an error.
	_, span := trace.New(ctx, spanEventPublish, oteltrace.WithAttributes(
		attrEventName.String(payload.Name()), attrEventRecipient.String(evt.Recipient)))
	defer span.End()

	payloadPb := schema.Marshal(payload)

	eventPb := &iscv1.Event{
		Name:    payload.Name(),
		Payload: payloadPb,
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
			s.log.Debug().
				Str("recipient", evt.Recipient).
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
	if evt.Recipient != "" {
		waiters = append(waiters, s.replyWaiters[eventPb.GetName()][evt.Recipient]...)
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
	// fail the dispatch: one dead stream must not block delivery to the others.
	sendFailures := 0
	for _, subscriber := range subscribers {
		select {
		case <-subscriber.ctx.Done():
			continue
		default:
		}

		err := subscriber.send(&cardinalv1.StartEventStreamResponse{
			Address: s.world.address,
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

	return nil
}

func matchesEvent(subscription string, eventName string) bool {
	return subscription == eventName ||
		subscription == "*" ||
		subscription == ">" ||
		(strings.HasSuffix(subscription, ".>") && strings.HasPrefix(eventName, strings.TrimSuffix(subscription, ">")))
}

// -------------------------------------------------------------------------------------------------
// ISC
// -------------------------------------------------------------------------------------------------

func (s *service) handlePing(_ context.Context, req *micro.Request) *micro.Response {
	return micro.NewSuccessResponse(req, nil)
}

// drainInterShardCommands is a no-op when the service never connected to NATS, as in the DST harness.
func (s *service) drainInterShardCommands() {
	if s.interShard != nil {
		s.interShard.drain()
	}
}

func (s *service) publishInterShardCommand(ctx context.Context, evt event.Event) error {
	isc, ok := evt.Payload.(command.Command)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", evt.Payload)
	}
	assert.That(isc.Address != nil, "inter shard command has nil address")
	assert.That(s.interShard != nil, "inter shard command published before the service started")

	s.interShard.enqueue(ctx, &iscv1.InterShardCommand{
		Command: &iscv1.Command{
			Name:    isc.Payload.Name(),
			Address: isc.Address,
			Payload: schema.Marshal(isc.Payload),
		},
		Sender: s.world.address,
	})
	return nil
}

// -------------------------------------------------------------------------------------------------
// Authentication
// -------------------------------------------------------------------------------------------------

// Player is the authenticated gameplay identity supplied to Cardinal handlers.
type Player struct {
	ID string
}

// AuthMode selects the authentication mode for the client-facing ConnectRPC service.
type AuthMode uint8

const (
	AuthModeUndefined AuthMode = iota
	AuthModeArgus
	AuthModeDev
)

const (
	argusAuthModeString     = "ARGUS"
	devAuthModeString       = "DEV"
	undefinedAuthModeString = "UNDEFINED"
)

func (a AuthMode) String() string {
	switch a {
	case AuthModeUndefined:
		return undefinedAuthModeString
	case AuthModeArgus:
		return argusAuthModeString
	case AuthModeDev:
		return devAuthModeString
	default:
		return undefinedAuthModeString
	}
}

func (a AuthMode) IsValid() bool {
	return a == AuthModeArgus || a == AuthModeDev
}

func ParseAuthMode(s string) (AuthMode, error) {
	switch strings.ToUpper(s) {
	case argusAuthModeString:
		return AuthModeArgus, nil
	case devAuthModeString:
		return AuthModeDev, nil
	default:
		return AuthModeUndefined, eris.Errorf("invalid auth mode: %s", s)
	}
}

func PlayerFromContext(ctx context.Context) *Player {
	info := authn.GetInfo(ctx)
	if info == nil {
		return nil
	}
	player, ok := info.(*Player)
	if !ok {
		return nil
	}
	return player
}

// -------------------------------------------------------------------------------------------------
// Argus Auth
// -------------------------------------------------------------------------------------------------

// ArgusAuthenticator authenticates game tokens issued by Argus Auth for one organization/project.
type ArgusAuthenticator struct {
	audience string
	keyfunc  keyfunc.Keyfunc
}

// NewArgusAuthenticator fetches Argus Auth's signing keys and returns an authenticator that
// accepts only EdDSA game tokens with aud equal to organization/project, an unexpired exp, and a
// non-empty sub. Pass its Authenticate method to authn.NewMiddleware.
func NewArgusAuthenticator(argusAuthURL, organization, project string) (*ArgusAuthenticator, error) {
	if argusAuthURL == "" {
		return nil, eris.New("argus auth URL cannot be empty")
	}
	// Argus Auth issues aud as exactly "organization/project", so a '/' in either part could never
	// match and every token would be rejected.
	if organization == "" || project == "" || strings.Contains(organization+project, "/") {
		return nil, eris.Errorf(
			"organization %q and project %q must be non-empty and must not contain '/' in ARGUS auth mode",
			organization, project,
		)
	}

	jwksURL := argusAuthURL + "/auth/jwks"
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create JWKS request")
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, eris.Wrap(err, "failed to fetch JWKS")
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return nil, eris.Errorf("HTTP error: %d - %s", response.StatusCode, response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, eris.Wrap(err, "failed to read response body")
	}

	keyfn, err := keyfunc.NewJWKSetJSON(json.RawMessage(body))
	if err != nil {
		return nil, eris.Wrap(err, "failed to create keyfunc")
	}

	return &ArgusAuthenticator{
		audience: organization + "/" + project,
		keyfunc:  keyfn,
	}, nil
}

// Authenticate returns the *Player named by the request's bearer token. It satisfies
// authn.AuthFunc; every rejection is a connect.CodeUnauthenticated error.
func (a *ArgusAuthenticator) Authenticate(_ context.Context, req *http.Request) (any, error) {
	jwtString, ok := authn.BearerToken(req)
	if !ok {
		return nil, authn.Errorf("Authorization header must be in format: 'Bearer <JWT>'")
	}

	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		jwtString,
		claims,
		a.keyfunc.Keyfunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithAudience(a.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, authn.Errorf("invalid JWT: %v", err)
	}
	if !token.Valid {
		return nil, authn.Errorf("JWT token is invalid")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, authn.Errorf("JWT subject is required")
	}

	return &Player{ID: claims.Subject}, nil
}

// -------------------------------------------------------------------------------------------------
// Dev Auth
// -------------------------------------------------------------------------------------------------

// devPlayerIDHeader names the caller's player ID in DEV auth mode.
const devPlayerIDHeader = "X-Player-Id"

type authenticatorDev struct{}

func (a authenticatorDev) authenticate(_ context.Context, req *http.Request) (any, error) {
	playerID := strings.TrimSpace(req.Header.Get(devPlayerIDHeader))
	if playerID == "" {
		return nil, authn.Errorf("%s header is required", devPlayerIDHeader)
	}

	return &Player{ID: playerID}, nil
}
