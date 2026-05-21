package cardinal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
)

// service2 will host the direct client-facing Cardinal service.
type service2 struct {
	world        *World
	server       *http.Server
	log          zerolog.Logger
	authMode     AuthMode
	argusAuthURL string
	subscribers  map[string]*streamSubscriber
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

func (s *service2) addSubscriber(
	ctx context.Context,
	user *User,
	stream *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
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

// -------------------------------------------------------------------------------------------------
// Handlers
// -------------------------------------------------------------------------------------------------

type streamSubscriber struct {
	ctx    context.Context
	stream *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse]
	events map[string]struct{}
	mu     sync.Mutex
}

func (s *service2) Stream(
	ctx context.Context,
	stream *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
) error {
	user := UserFromContext(ctx)
	assert.That(user != nil, "user should exist in authenticated stream context")

	if err := s.addSubscriber(ctx, user, stream); err != nil {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	defer s.removeSubscriber(user)

	for {
		msg, err := stream.Receive()
		if eris.Is(err, io.EOF) {
			s.log.Debug().Str("user_id", user.ID).Msg("service2 stream received EOF")
			return nil
		}
		if err != nil {
			return eris.Wrap(err, "failed to receive stream request")
		}

		payload := msg.GetPayload()
		s.log.Debug().
			Str("user_id", user.ID).
			Str("message_type", fmt.Sprintf("%T", payload)).
			Stringer("message", msg).
			Msg("service2 stream received message")

		switch payload := payload.(type) {
		case *cardinalv1.StreamRequest_Command:
			if err := s.handleStreamCommand(ctx, user, stream, payload.Command); err != nil {
				return eris.Wrap(err, "failed to handle stream command")
			}
		case *cardinalv1.StreamRequest_SubscribeEvents:
			if err := s.handleStreamSubscribeEvents(ctx, user, stream, payload.SubscribeEvents); err != nil {
				return eris.Wrap(err, "failed to handle stream subscribe events")
			}
		case *cardinalv1.StreamRequest_UnsubscribeEvents:
			if err := s.handleStreamUnsubscribeEvents(ctx, user, stream, payload.UnsubscribeEvents); err != nil {
				return eris.Wrap(err, "failed to handle stream unsubscribe events")
			}
		case *cardinalv1.StreamRequest_Heartbeat:
			if err := s.handleStreamHeartbeat(ctx, user, stream, payload.Heartbeat); err != nil {
				return eris.Wrap(err, "failed to handle stream heartbeat")
			}
		default:
			err := s.sendStreamError(stream, connect.CodeInvalidArgument, "stream request payload is required")
			if err != nil {
				return eris.Wrap(err, "failed to send stream error")
			}
		}
	}
}

func (s *service2) handleStreamCommand(
	_ context.Context,
	user *User,
	stream *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
	request *cardinalv1.CommandRequest,
) error {
	assert.That(request != nil, "command request should have been validated")

	cmd := request.GetCommand()
	assert.That(cmd != nil, "command should have been validated")
	assert.That(cmd.GetPersona() != nil, "command persona should have been validated")

	cmd.Persona.Id = user.ID

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return s.sendStreamError(stream, connect.CodeInvalidArgument, "command address doesn't match shard address")
	}

	if err := s.world.commands.Enqueue(cmd); err != nil {
		return s.sendStreamError(stream, connect.CodeInvalidArgument, eris.Wrap(err, "failed to enqueue command").Error())
	}

	return nil
}

func (s *service2) handleStreamSubscribeEvents(
	_ context.Context,
	user *User,
	_ *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
	request *cardinalv1.SubscribeEventsRequest,
) error {
	assert.That(request != nil, "subscribe events request should have been validated")
	assert.That(len(request.GetEventNames()) > 0, "event names should have been validated")

	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[user.ID]
	assert.That(subscriber != nil, "subscriber should exist for authenticated stream")
	for _, eventName := range request.GetEventNames() {
		subscriber.events[eventName] = struct{}{}
	}

	return nil
}

func (s *service2) handleStreamUnsubscribeEvents(
	_ context.Context,
	user *User,
	_ *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
	request *cardinalv1.UnsubscribeEventsRequest,
) error {
	assert.That(request != nil, "unsubscribe events request should have been validated")
	assert.That(len(request.GetEventNames()) > 0, "event names should have been validated")

	s.mu.Lock()
	defer s.mu.Unlock()

	subscriber := s.subscribers[user.ID]
	assert.That(subscriber != nil, "subscriber should exist for authenticated stream")
	for _, eventName := range request.GetEventNames() {
		delete(subscriber.events, eventName)
	}

	return nil
}

func (s *service2) handleStreamHeartbeat(
	_ context.Context,
	_ *User,
	stream *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
	_ *cardinalv1.Heartbeat,
) error {
	err := stream.Send(&cardinalv1.StreamResponse{
		Payload: &cardinalv1.StreamResponse_Heartbeat{
			Heartbeat: &cardinalv1.Heartbeat{},
		},
	})
	if err != nil {
		return eris.Wrap(err, "failed to send heartbeat response")
	}
	return nil
}

func (s *service2) sendStreamError(
	stream *connect.BidiStream[cardinalv1.StreamRequest, cardinalv1.StreamResponse],
	code connect.Code,
	message string,
) error {
	err := stream.Send(&cardinalv1.StreamResponse{
		Payload: &cardinalv1.StreamResponse_Error{
			Error: &cardinalv1.StreamError{
				Status: &statuspb.Status{
					Code:    int32(code),
					Message: message,
				},
			},
		},
	})
	if err != nil {
		return eris.Wrap(err, "failed to send stream error response")
	}
	return nil
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
		if _, ok := subscriber.events[eventPb.GetName()]; ok {
			subscribers = append(subscribers, subscriber)
			continue
		}
		if _, ok := subscriber.events["*"]; ok {
			subscribers = append(subscribers, subscriber)
		}
	}
	s.mu.RUnlock()

	for _, subscriber := range subscribers {
		select {
		case <-subscriber.ctx.Done():
			continue
		default:
		}

		subscriber.mu.Lock()
		err := subscriber.stream.Send(&cardinalv1.StreamResponse{
			Payload: &cardinalv1.StreamResponse_Event{
				Event: &cardinalv1.EventMessage{Event: eventPb},
			},
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

	// TODO: Remove this check once persona ID is removed from the JWT.
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
