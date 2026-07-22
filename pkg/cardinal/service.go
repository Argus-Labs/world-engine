package cardinal

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"buf.build/go/protovalidate"
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
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
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
	commands     map[string]struct{}
	subscribers  map[string]*streamSubscriber
	replyWaiters map[string][]chan *iscv1.Event
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
		replyWaiters: make(map[string][]chan *iscv1.Event),
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

	// Keep these for now cuz ISC requires a bit more work than client connections. Will need another
	// refactor after the current clients are migrated to connect directly to the shards.
	if err = s.microService.AddEndpoint("ping", s.handlePing); err != nil {
		return eris.Wrap(err, "failed to register ping handler")
	}
	for cmd := range s.commands {
		if err := s.microService.AddGroup("command").AddEndpoint(cmd, s.handleInterShardCommand); err != nil {
			return eris.Wrapf(err, "failed to register %s command handler", cmd)
		}
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
		authenticator, err := newAuthenticatorArgus(s.argusAuthURL)
		if err != nil {
			return eris.Wrap(err, "failed to create argus authenticator")
		}
		authenticate = authenticator.authenticate
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

	if s.world.debug != nil {
		debugPath, debugHandler := cardinalv1connect.NewDebugServiceHandler(
			s.world.debug,
			connect.WithInterceptors(otelInterceptor, validateInterceptor),
		)
		mux.Handle(debugPath, debugHandler)
		s.log.Info().Msg("DebugService mounted on client-facing port (dev)")
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

func (s *service) shutdown(ctx context.Context) error {
	assert.That(s.server != nil, "Don't call shutdown before you init server")
	assert.That(s.client != nil, "Don't call shutdown before you init server")

	if err := s.server.Shutdown(ctx); err != nil {
		return eris.Wrap(err, "failed to shutdown service server")
	}
	if s.microService != nil {
		if err := s.microService.Close(); err != nil {
			return eris.Wrap(err, "failed to close micro service")
		}
	}
	s.client.Close()

	return nil
}

func (s *service) registerCommandHandler(name string) {
	s.commands[name] = struct{}{}
}

// -------------------------------------------------------------------------------------------------
// Command handlers
// -------------------------------------------------------------------------------------------------

// TODO: eventually, we'll probably have more user fields in the command metadata, possibly a User
// struct field instead of a single persona ID.

type streamSubscriber struct {
	ctx    context.Context
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse]
	events map[string]struct{}
	mu     sync.Mutex
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

	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")
	assert.That(cmd.GetPersona() != nil, "command persona should have been validated")

	cmd.Persona.Id = user.ID

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.world.commands.Enqueue(cmd); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command"))
	}

	return connect.NewResponse(&cardinalv1.SendCommandResponse{}), nil
}

func (s *service) SendCommandWithReply(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandWithReplyRequest],
) (*connect.Response[cardinalv1.SendCommandWithReplyResponse], error) {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")
	assert.That(cmd.GetPersona() != nil, "command persona should have been validated")

	cmd.Persona.Id = user.ID

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.world.commands.Enqueue(cmd); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command"))
	}

	waiter := s.addReplyWaiter(req.Msg.GetEventName())
	defer s.removeReplyWaiter(req.Msg.GetEventName(), waiter)

	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "waiting for reply event"))
	case event := <-waiter:
		return connect.NewResponse(&cardinalv1.SendCommandWithReplyResponse{Event: event}), nil
	}
}

func (s *service) addReplyWaiter(eventName string) chan *iscv1.Event {
	waiter := make(chan *iscv1.Event, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.replyWaiters[eventName] = append(s.replyWaiters[eventName], waiter)
	return waiter
}

func (s *service) removeReplyWaiter(eventName string, waiter chan *iscv1.Event) {
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

func (s *service) StartEventStream(
	ctx context.Context,
	req *connect.Request[cardinalv1.StartEventStreamRequest],
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) error {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated stream context")

	if err := s.addSubscriber(ctx, user, stream); err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	defer s.removeSubscriber(user)

	for _, subscription := range req.Msg.GetSubscriptions() {
		if micro.String(s.world.address) != micro.String(subscription.GetAddress()) {
			return connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	s.subscribeEvents(user, req.Msg.GetSubscriptions())

	if err := stream.Send(&cardinalv1.StartEventStreamResponse{}); err != nil {
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
			if err := stream.Send(&cardinalv1.StartEventStreamResponse{}); err != nil {
				return err
			}
		}
	}
}

func (s *service) SubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.SubscribeEventsRequest],
) (*connect.Response[cardinalv1.SubscribeEventsResponse], error) {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	if !s.hasSubscriber(user) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range req.Msg.GetSubscriptions() {
		if micro.String(s.world.address) != micro.String(subscription.GetAddress()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	s.subscribeEvents(user, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.SubscribeEventsResponse{}), nil
}

func (s *service) UnsubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.UnsubscribeEventsRequest],
) (*connect.Response[cardinalv1.UnsubscribeEventsResponse], error) {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	if !s.hasSubscriber(user) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range req.Msg.GetSubscriptions() {
		if micro.String(s.world.address) != micro.String(subscription.GetAddress()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	s.unsubscribeEvents(user, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.UnsubscribeEventsResponse{}), nil
}

func (s *service) addSubscriber(
	ctx context.Context,
	user *User,
	stream *connect.ServerStream[cardinalv1.StartEventStreamResponse],
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.subscribers[user.ID]; exists {
		return eris.Errorf("user %s already has an open stream", user.ID)
	}

	s.subscribers[user.ID] = &streamSubscriber{
		ctx:    ctx,
		stream: stream,
		events: make(map[string]struct{}),
	}
	return nil
}

func (s *service) removeSubscriber(user *User) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscribers, user.ID)
}

func (s *service) subscribeEvents(user *User, subscriptions []*cardinalv1.EventSubscription) {
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

func (s *service) unsubscribeEvents(user *User, subscriptions []*cardinalv1.EventSubscription) {
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

func (s *service) hasSubscriber(user *User) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.subscribers[user.ID]
	return ok
}

// -------------------------------------------------------------------------------------------------
// Event publishers
// -------------------------------------------------------------------------------------------------

// TODO: move away from this centralized approach to a actor model for easier(?) synchronization.

//nolint:gocognit // Put everything here so you can understand the logic in one place.
func (s *service) publishDefaultEvent(evt event.Event) error {
	payload, ok := evt.Payload.(event.Payload)
	if !ok {
		return eris.Errorf("invalid event payload type: %T", evt.Payload)
	}

	payloadPb, err := schema.Serialize(payload)
	if err != nil {
		return eris.Wrap(err, "failed to marshal event payload")
	}

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

		subscriber.mu.Lock()
		err := subscriber.stream.Send(&cardinalv1.StartEventStreamResponse{
			Address: s.world.address,
			Event:   eventPb,
		})
		subscriber.mu.Unlock()
		if err != nil {
			s.log.Error().Err(err).Str("event", eventPb.GetName()).Msg("failed to send event to subscriber")
			continue
		}
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

func (s *service) handleInterShardCommand(ctx context.Context, req *micro.Request) *micro.Response {
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

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return micro.NewErrorResponse(req, eris.New("command address doesn't match shard address"), codes.InvalidArgument)
	}

	if err := s.world.commands.Enqueue(cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), codes.InvalidArgument)
	}

	return micro.NewSuccessResponse(req, nil)
}

func (s *service) publishInterShardCommand(evt event.Event) error {
	isc, ok := evt.Payload.(command.Command)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", evt.Payload)
	}
	assert.That(isc.Address != nil, "inter shard command has nil address")

	payload, err := command.Marshal(isc.Payload)
	if err != nil {
		return eris.Wrap(err, "failed to marshal command payload")
	}

	commandPb := &iscv1.Command{
		Name:    isc.Payload.Name(),
		Address: isc.Address,
		Persona: &iscv1.Persona{Id: isc.Persona},
		Payload: payload,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = s.client.Request(ctx, isc.Address, "command."+isc.Payload.Name(), commandPb)
	if err != nil {
		return eris.Wrapf(err, "failed to send inter-shard command %s to shard", isc.Payload.Name())
	}

	return nil
}

// -------------------------------------------------------------------------------------------------
// Authentication
// -------------------------------------------------------------------------------------------------

type User struct {
	jwt.RegisteredClaims

	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
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

func UserFromContext(ctx context.Context) *User {
	info := authn.GetInfo(ctx)
	if info == nil {
		return nil
	}
	user, ok := info.(*User)
	if !ok {
		return nil
	}
	return user
}

// -------------------------------------------------------------------------------------------------
// Argus Auth
// -------------------------------------------------------------------------------------------------

type authenticatorArgus struct {
	keyfunc keyfunc.Keyfunc
}

func newAuthenticatorArgus(argusAuthURL string) (*authenticatorArgus, error) {
	assert.That(argusAuthURL != "", "Should've validated the URL")

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

	return &authenticatorArgus{keyfunc: keyfn}, nil
}

func (a *authenticatorArgus) authenticate(_ context.Context, req *http.Request) (any, error) {
	jwtString, ok := authn.BearerToken(req)
	if !ok {
		return nil, authn.Errorf("Authorization header must be in format: 'Bearer <JWT>'")
	}

	user := &User{}
	token, err := jwt.ParseWithClaims(jwtString, user, a.keyfunc.Keyfunc)
	if err != nil {
		return nil, eris.Wrap(err, "JWT parse error")
	}
	if !token.Valid {
		return nil, eris.New("JWT token is invalid")
	}

	// TODO: Remove this comment once persona ID is removed from the JWT.
	// if u.PersonaID == "" {
	// 	return nil, authn.Errorf("JWT token is missing persona ID")
	// }

	return user, nil
}

// -------------------------------------------------------------------------------------------------
// Dev Auth
// -------------------------------------------------------------------------------------------------

type authenticatorDev struct{}

func (a authenticatorDev) authenticate(_ context.Context, req *http.Request) (any, error) {
	email := strings.TrimSpace(req.Header.Get("X-Email"))
	if email == "" {
		return nil, authn.Errorf("X-Email header is required")
	}

	return &User{ID: email, Email: email}, nil
}
