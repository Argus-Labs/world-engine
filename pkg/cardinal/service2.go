package cardinal

import (
	"context"
	"encoding/json"
	"io"
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
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/shamaton/msgpack/v3"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// service2 will host the direct client-facing Cardinal service.
type service2 struct {
	world        *World
	server       *http.Server
	log          zerolog.Logger
	authMode     AuthMode
	argusAuthURL string
	subscribers  map[string]*streamSubscriber
	replyWaiters map[string][]chan *iscv1.Event
	mu           sync.RWMutex
}

var _ cardinalv1connect.CardinalServiceHandler = (*service2)(nil)

// newService2 creates a new direct client-facing Cardinal service.
func newService2(world *World, authMode AuthMode, argusAuthURL string) *service2 {
	return &service2{
		world:        world,
		log:          world.tel.GetLogger("service"),
		authMode:     authMode,
		argusAuthURL: argusAuthURL,
		subscribers:  make(map[string]*streamSubscriber),
		replyWaiters: make(map[string][]chan *iscv1.Event),
	}
}

func (s *service2) init(address string) error {
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
		authenticator := newAuthenticatorDev()
		authenticate = authenticator.authenticate
		mux.Handle("/dev-auth-sign-in", authenticator.signInHandler())
	case AuthModePassthrough:
		authenticate = authenticatorPassthrough{}.authenticate
	default:
		return eris.Errorf("invalid service2 auth mode: %s", s.authMode)
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

	s.server = &http.Server{
		Addr:              address,
		Handler:           h2c.NewHandler(mux, &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && !eris.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("service2 server error")
		}
	}()

	return nil
}

func (s *service2) shutdown(ctx context.Context) error {
	assert.That(s.server != nil, "Don't call shutdown before you init server")

	if err := s.server.Shutdown(ctx); err != nil {
		return eris.Wrap(err, "failed to shutdown service2 server")
	}
	return nil
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

func (s *service2) SendCommand(
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

	if !s.isValidAddress(cmd.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	if err := s.world.commands.Enqueue(cmd); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command"))
	}

	return connect.NewResponse(&cardinalv1.SendCommandResponse{}), nil
}

func (s *service2) SendCommandWithReply(
	ctx context.Context,
	req *connect.Request[cardinalv1.SendCommandWithReplyRequest],
) (*connect.Response[cardinalv1.SendCommandWithReplyResponse], error) {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	cmd := req.Msg.GetCommand()
	assert.That(cmd != nil, "command should have been validated")
	assert.That(cmd.GetPersona() != nil, "command persona should have been validated")

	cmd.Persona.Id = user.ID

	if !s.isValidAddress(cmd.GetAddress()) {
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

func (s *service2) addReplyWaiter(eventName string) chan *iscv1.Event {
	waiter := make(chan *iscv1.Event, 1)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.replyWaiters[eventName] = append(s.replyWaiters[eventName], waiter)
	return waiter
}

func (s *service2) removeReplyWaiter(eventName string, waiter chan *iscv1.Event) {
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
// Query handler
// -------------------------------------------------------------------------------------------------

func (s *service2) Query(
	ctx context.Context,
	req *connect.Request[cardinalv1.QueryRequest],
) (*connect.Response[cardinalv1.QueryResponse], error) {
	select {
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, eris.Wrap(ctx.Err(), "context cancelled"))
	default:
	}

	if !s.isValidAddress(req.Msg.GetAddress()) {
		return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
	}

	queryPb := req.Msg.GetQuery()
	if err := protovalidate.Validate(queryPb); err != nil {
		return nil, connect.NewError(
			connect.CodeInternal,
			eris.Wrap(eris.Wrap(err, "failed to validate query"), "failed to parse request payload"),
		)
	}

	results, err := s.world.world.NewSearch(ecs.SearchParam{
		Find:   queryPb.GetFind(),
		Match:  ecs.SearchMatch(iscv1MatchToString(queryPb.GetMatch())),
		Where:  queryPb.GetWhere(),
		Limit:  queryPb.GetLimit(),
		Offset: queryPb.GetOffset(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, eris.Wrap(err, "failed to search entities"))
	}

	entities := make([][]byte, 0, len(results))
	for _, result := range results {
		data, err := msgpack.Marshal(result)
		if err != nil {
			return nil, connect.NewError(
				connect.CodeInternal,
				eris.Wrap(eris.Wrap(err, "failed to marshal entity to msgpack"), "failed to serialize results"),
			)
		}
		entities = append(entities, data)
	}

	return connect.NewResponse(
		&cardinalv1.QueryResponse{Results: &iscv1.QueryResult{Entities: entities}},
	), nil
}

func iscv1MatchToString(m iscv1.Query_Match) string {
	switch m {
	case iscv1.Query_MATCH_EXACT:
		return "exact"
	case iscv1.Query_MATCH_CONTAINS:
		return "contains"
	case iscv1.Query_MATCH_ALL:
		return "all"
	case iscv1.Query_MATCH_UNSPECIFIED:
		fallthrough
	default:
		return "" // This will be validated again in ecs.NewSearch
	}
}

// -------------------------------------------------------------------------------------------------
// Event streams
// -------------------------------------------------------------------------------------------------

func (s *service2) StartEventStream(
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
		if !s.isValidAddress(subscription.GetAddress()) {
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

func (s *service2) SubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.SubscribeEventsRequest],
) (*connect.Response[cardinalv1.SubscribeEventsResponse], error) {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	if !s.hasSubscriber(user) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range req.Msg.GetSubscriptions() {
		if !s.isValidAddress(subscription.GetAddress()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	s.subscribeEvents(user, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.SubscribeEventsResponse{}), nil
}

func (s *service2) UnsubscribeEvents(
	ctx context.Context,
	req *connect.Request[cardinalv1.UnsubscribeEventsRequest],
) (*connect.Response[cardinalv1.UnsubscribeEventsResponse], error) {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated request context")

	if !s.hasSubscriber(user) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("client has no established stream"))
	}

	for _, subscription := range req.Msg.GetSubscriptions() {
		if !s.isValidAddress(subscription.GetAddress()) {
			return nil, connect.NewError(connect.CodeInvalidArgument, eris.New("address doesn't match shard address"))
		}
	}
	s.unsubscribeEvents(user, req.Msg.GetSubscriptions())

	return connect.NewResponse(&cardinalv1.UnsubscribeEventsResponse{}), nil
}

func (s *service2) addSubscriber(
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

func (s *service2) removeSubscriber(user *User) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.subscribers, user.ID)
}

func (s *service2) subscribeEvents(user *User, subscriptions []*cardinalv1.EventSubscription) {
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

func (s *service2) unsubscribeEvents(user *User, subscriptions []*cardinalv1.EventSubscription) {
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

func (s *service2) hasSubscriber(user *User) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.subscribers[user.ID]
	return ok
}

// -------------------------------------------------------------------------------------------------
// Event publishers
// -------------------------------------------------------------------------------------------------

// TODO: move away from this centralized approach to a actor model for easier(?) synchronization.

func (s *service2) publishDefaultEvent(evt event.Event) error {
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
	subscribers := make([]*streamSubscriber, 0, len(s.subscribers))
	for _, subscriber := range s.subscribers {
		for subscription := range subscriber.events {
			if matchesEvent(subscription, eventPb.GetName()) {
				subscribers = append(subscribers, subscriber)
				break
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
			return eris.Wrap(err, "failed to send event to subscriber")
		}
	}

	return nil
}

func (s *service2) publishInterShardCommand(evt event.Event) error {
	if _, ok := evt.Payload.(command.Command); !ok {
		return eris.Errorf("invalid inter shard command %v", evt.Payload)
	}
	panic("unimplemented")
}

func matchesEvent(subscription string, eventName string) bool {
	return subscription == eventName ||
		subscription == "*" ||
		subscription == ">" ||
		(strings.HasSuffix(subscription, ".>") && strings.HasPrefix(eventName, strings.TrimSuffix(subscription, ">")))
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
	AuthModePassthrough
)

const (
	argusAuthModeString       = "ARGUS"
	devAuthModeString         = "DEV"
	passthroughAuthModeString = "PASSTHROUGH"
	undefinedAuthModeString   = "UNDEFINED"
)

func (a AuthMode) String() string {
	switch a {
	case AuthModeUndefined:
		return undefinedAuthModeString
	case AuthModeArgus:
		return argusAuthModeString
	case AuthModeDev:
		return devAuthModeString
	case AuthModePassthrough:
		return passthroughAuthModeString
	default:
		return undefinedAuthModeString
	}
}

func (a AuthMode) IsValid() bool {
	return a == AuthModeArgus || a == AuthModeDev || a == AuthModePassthrough
}

func ParseAuthMode(s string) (AuthMode, error) {
	switch strings.ToUpper(s) {
	case argusAuthModeString:
		return AuthModeArgus, nil
	case devAuthModeString:
		return AuthModeDev, nil
	case passthroughAuthModeString:
		return AuthModePassthrough, nil
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

type authenticatorDev struct {
	sessions map[string]string
	mu       sync.Mutex
}

func newAuthenticatorDev() *authenticatorDev {
	return &authenticatorDev{
		sessions: make(map[string]string),
	}
}

func (a *authenticatorDev) authenticate(_ context.Context, req *http.Request) (any, error) {
	email := strings.TrimSpace(req.Header.Get("X-Email"))
	if email == "" {
		return nil, authn.Errorf("X-Email header is required. Have you called sign in?")
	}

	email = strings.TrimSpace(email)
	a.mu.Lock()
	userID, exists := a.sessions[email]
	if !exists {
		a.mu.Unlock()
		return "", eris.Errorf("persona ID not found for email %s", email)
	}
	a.mu.Unlock()

	return &User{ID: userID, Email: email}, nil
}

func (a *authenticatorDev) signInHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var signIn struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(req.Body).Decode(&signIn); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		email := strings.TrimSpace(signIn.Email)
		if email == "" {
			http.Error(w, "email is required", http.StatusBadRequest)
			return
		}

		a.mu.Lock()
		userID, exists := a.sessions[email]
		if !exists {
			userID = uuid.New().String()
			a.sessions[email] = userID
		}
		a.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(struct {
			PersonaID string `json:"personaId"`
			Email     string `json:"email"`
		}{
			PersonaID: userID,
			Email:     email,
		})
	})
}

// -------------------------------------------------------------------------------------------------
// Passthrough
// -------------------------------------------------------------------------------------------------

type authenticatorPassthrough struct{}

func (a authenticatorPassthrough) authenticate(_ context.Context, req *http.Request) (any, error) {
	return &User{ID: "test-user"}, nil
}

// -------------------------------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------------------------------

func (s *service2) isValidAddress(address *micro.ServiceAddress) bool {
	return micro.String(s.world.address) == micro.String(address)
}
