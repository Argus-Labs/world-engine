package micro

import (
	"context"
	"time"

	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/caarlos0/env/v11"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// Common errors that can occur during NATS operations.
var (
	ErrFailedToConnect         = eris.New("failed to connect to NATS server")
	ErrFailedToSubscribe       = eris.New("failed to subscribe to subject")
	ErrFailedToPublish         = eris.New("failed to publish message")
	ErrInvalidConfig           = eris.New("invalid NATS configuration")
	ErrFailedToCreatePayload   = eris.New("failed to create Any payload")
	ErrFailedToMarshal         = eris.New("failed to marshal request")
	ErrFailedToAutoUnsubscribe = eris.New("failed to set auto-unsubscribe")
	ErrCommandRejected         = eris.New("command rejected")
	ErrFailedToReceiveEvent    = eris.New("failed to receive event response")
	ErrFailedToUnmarshal       = eris.New("failed to unmarshal event response")
)

// Default timeout constants for RequestSync.
const (
	defaultRequestTimeout  = 2 * time.Second  // Timeout for initial request validation
	defaultResponseTimeout = 10 * time.Second // Timeout for event response
)

// Client represents a NATS client with enhanced logging and error handling.
type Client struct {
	*nats.Conn
	log        zerolog.Logger
	natsConfig NATSConfig
}

// NATSConfig holds the configuration for the NATS client.
type NATSConfig struct {
	Name            string `env:"NATS_NAME" envDefault:"isc"`
	URL             string `env:"NATS_URL" envDefault:"nats://nats:4222"`
	CredentialsFile string `env:"NATS_CREDENTIALS_FILE"`
}

// Validate validates the NATS configuration and returns an error if invalid.
func (cfg NATSConfig) Validate() error {
	if cfg.URL == "" {
		return eris.New("NATS URL is required")
	}
	// CredentialsFile, NKey fields are all optional.
	// If none are provided, will connect without authentication (for testing).
	return nil
}

// NewClient creates a new NATS client with the given configuration.
// It handles connection setup, error handling, and logging.
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{
		Conn:       nil,
		natsConfig: NATSConfig{},
	}

	// Parse the NATS config from environment variables.
	var err error
	c.natsConfig, err = env.ParseAs[NATSConfig]()
	if err != nil {
		return nil, eris.Wrap(ErrInvalidConfig, err.Error())
	}

	// Apply options that may override environment variables.
	for _, opt := range opts {
		opt(c)
	}

	if err := c.natsConfig.Validate(); err != nil {
		return nil, eris.Wrap(ErrInvalidConfig, err.Error())
	}

	// Init NATS options with validated configuration.
	natsOpts := []nats.Option{
		nats.Name(c.natsConfig.Name),
		nats.MaxReconnects(10),
		nats.ReconnectWait(time.Second * 5),
		nats.DisconnectErrHandler(c.handleDisconnect),
		nats.ReconnectHandler(c.handleReconnect),
		nats.ClosedHandler(c.handleClosed),
		nats.ErrorHandler(c.handleError),
	}

	// Add credentials authentication if credentials file is provided.
	if c.natsConfig.CredentialsFile != "" {
		natsOpts = append(natsOpts, nats.UserCredentials(c.natsConfig.CredentialsFile))
	}
	// Else we're unauthenticated.

	// Create the NATS connection.
	conn, err := nats.Connect(c.natsConfig.URL, natsOpts...)
	if err != nil {
		return nil, eris.Wrap(ErrFailedToConnect, err.Error())
	}
	c.Conn = conn

	c.log.Info().
		Str("url", c.ConnectedUrl()).
		Str("name", c.natsConfig.Name).
		Msg("Connected to NATS server")

	return c, nil
}

// NewTestClient creates a NATS client specifically for testing without authentication.
// This should only be used in test environments with unauthenticated NATS servers.
func NewTestClient(natsURL string) (*Client, error) {
	c := &Client{
		natsConfig: NATSConfig{
			Name: "test-client",
			URL:  natsURL,
		},
		log: zerolog.Nop(),
	}

	// Create basic NATS options without authentication
	natsOpts := []nats.Option{
		nats.Name(c.natsConfig.Name),
		nats.MaxReconnects(10),
		nats.ReconnectWait(time.Second * 5),
		nats.DisconnectErrHandler(c.handleDisconnect),
		nats.ReconnectHandler(c.handleReconnect),
		nats.ClosedHandler(c.handleClosed),
		nats.ErrorHandler(c.handleError),
	}

	// Connect without authentication (for test NATS servers).
	conn, err := nats.Connect(c.natsConfig.URL, natsOpts...)
	if err != nil {
		return nil, eris.Wrap(ErrFailedToConnect, err.Error())
	}
	c.Conn = conn

	c.log.Info().
		Str("url", c.ConnectedUrl()).
		Str("name", c.natsConfig.Name).
		Msg("Connected to test NATS server")

	return c, nil
}

// Request sends a request to a specific endpoint and returns the response. Use this method when
// you want a request-reply pattern.
func (c *Client) Request(
	ctx context.Context,
	address *ServiceAddress,
	endpoint string,
	payload proto.Message,
) (*microv1.Response, error) {
	anyPayload, err := anypb.New(payload)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create Any payload")
	}

	req := &microv1.Request{
		ServiceAddress: address,
		Payload:        anyPayload,
	}

	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal request")
	}

	// Use NATS's built-in context support
	msg, err := c.RequestWithContext(ctx, Endpoint(address, endpoint), reqBytes)
	if err != nil {
		return nil, eris.Wrap(err, "failed to send request")
	}

	var res microv1.Response
	if err := proto.Unmarshal(msg.Data, &res); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal response")
	}

	// micro handlers return errors in the response, so we'll check for it here return it as an error
	// instead of having the users do it themselves.
	status := res.GetStatus()
	if status.GetCode() != 0 { // 0 is the only success code
		return nil, eris.New(status.GetMessage())
	}

	// We return the response only if it's successful.
	return &res, nil
}

// RequestSync sends a request and waits for a response on a separate event subject.
// Default timeouts: 2s for request, 10s for response. Use WithRequestTimeout/WithResponseTimeout to customize.
//
// Example:
//
//	response, err := client.RequestSync(ctx, address, endpoint, payload, eventSubject,
//	    micro.WithRequestTimeout(5*time.Second))
func (c *Client) RequestSync(
	ctx context.Context,
	address *ServiceAddress,
	endpoint string,
	payload proto.Message,
	eventSubject string,
	opts ...RequestSyncOption,
) (*microv1.Response, error) {
	// Apply default timeouts
	cfg := &requestSyncConfig{
		requestTimeout:  defaultRequestTimeout,
		responseTimeout: defaultResponseTimeout,
	}

	// Apply custom options
	for _, opt := range opts {
		opt(cfg)
	}

	return c.requestSyncWithTimeouts(
		ctx, address, endpoint, payload, eventSubject,
		cfg.requestTimeout, cfg.responseTimeout,
	)
}

// requestSyncWithTimeouts is the internal implementation of RequestSync.
func (c *Client) requestSyncWithTimeouts(
	ctx context.Context,
	address *ServiceAddress,
	endpoint string,
	payload proto.Message,
	eventSubject string,
	requestTimeout time.Duration,
	responseTimeout time.Duration,
) (*microv1.Response, error) {
	// Step 1: Prepare the request payload
	anyPayload, err := anypb.New(payload)
	if err != nil {
		return nil, eris.Wrap(ErrFailedToCreatePayload, err.Error())
	}

	req := &microv1.Request{
		ServiceAddress: address,
		Payload:        anyPayload,
	}

	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return nil, eris.Wrap(ErrFailedToMarshal, err.Error())
	}

	// Subscribe to event subject BEFORE sending request to prevent race condition.
	// Using SubscribeSync (not Subscribe) to allow NextMsgWithContext for timeout control.
	sub, err := c.SubscribeSync(eventSubject)
	if err != nil {
		return nil, eris.Wrap(ErrFailedToSubscribe, err.Error())
	}

	// Ensure cleanup in all paths (errors, timeouts, and success)
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			c.log.Warn().Err(err).Str("subject", eventSubject).Msg("Failed to unsubscribe")
		}
	}()

	// Auto-cleanup after one message for the success path.
	if err := sub.AutoUnsubscribe(1); err != nil {
		return nil, eris.Wrap(ErrFailedToAutoUnsubscribe, err.Error())
	}

	// Send command with immediate validation timeout.
	// Using RequestWithContext (not Publish) because handler calls msg.Respond() for validation.
	requestCtx, requestCancel := context.WithTimeout(ctx, requestTimeout)
	defer requestCancel()

	_, err = c.RequestWithContext(requestCtx, Endpoint(address, endpoint), reqBytes)
	if err != nil {
		return nil, eris.Wrap(ErrCommandRejected, err.Error())
	}

	// Wait for event response with separate timeout.
	responseCtx, responseCancel := context.WithTimeout(ctx, responseTimeout)
	defer responseCancel()

	msg, err := sub.NextMsgWithContext(responseCtx)
	if err != nil {
		return nil, eris.Wrap(ErrFailedToReceiveEvent, err.Error())
	}

	// Unmarshal the response
	var res microv1.Response
	if err := proto.Unmarshal(msg.Data, &res); err != nil {
		return nil, eris.Wrap(ErrFailedToUnmarshal, err.Error())
	}

	// micro handlers return errors in the response, so we'll check for it here
	status := res.GetStatus()
	if status.GetCode() != 0 { // 0 is the only success code
		return nil, eris.New(status.GetMessage())
	}

	return &res, nil
}

// Close gracefully closes the NATS connection and logs the event.
func (c *Client) Close() {
	if c.Conn != nil {
		c.Conn.Close()
		c.log.Info().Msg("NATS connection closed")
	}
}

// handleDisconnect handles NATS disconnection events.
func (c *Client) handleDisconnect(nc *nats.Conn, err error) {
	log := c.log.With().
		Str("nats_url", nc.ConnectedUrl()).
		Uint64("reconnect_attempts", nc.Reconnects).
		Logger()

	if err != nil {
		log.Error().Err(err).Msg("Disconnected from NATS with error")
	} else {
		log.Warn().Msg("Disconnected from NATS (no error)")
	}
}

// handleReconnect handles NATS reconnection events.
func (c *Client) handleReconnect(nc *nats.Conn) {
	c.log.Info().
		Str("nats_url", nc.ConnectedUrl()).
		Uint64("reconnect_attempts", nc.Reconnects).
		Msg("Reconnected to NATS")
}

// handleClosed handles NATS connection closure events.
func (c *Client) handleClosed(nc *nats.Conn) {
	log := c.log.With().
		Uint64("reconnect_attempts", nc.Reconnects).
		Logger()

	if err := nc.LastError(); err != nil {
		log.Warn().Err(err).Msg("NATS connection closed with error")
	} else {
		log.Info().Msg("NATS connection closed")
	}
}

// handleError handles NATS subscription errors.
func (c *Client) handleError(_ *nats.Conn, sub *nats.Subscription, err error) {
	c.log.Error().
		Err(err).
		Str("subject", sub.Subject).
		Msg("NATS subscription error occurred")
}

// ----------------------------------------------------------------------------
// Options
// ----------------------------------------------------------------------------

// ClientOption defines a function that can modify a Client.
type ClientOption func(*Client)

// WithLogger returns a ClientOption that sets the logger.
func WithLogger(log zerolog.Logger) ClientOption {
	return func(c *Client) {
		c.log = log
	}
}

// WithNATSConfig returns a ClientOption that sets the NATS configuration.
func WithNATSConfig(cfg NATSConfig) ClientOption {
	return func(c *Client) {
		c.natsConfig = cfg
	}
}

// ----------------------------------------------------------------------------
// RequestSync Options
// ----------------------------------------------------------------------------

// RequestSyncOption defines a function that can modify RequestSync behavior.
type RequestSyncOption func(*requestSyncConfig)

// requestSyncConfig holds configuration for a RequestSync call.
type requestSyncConfig struct {
	requestTimeout  time.Duration
	responseTimeout time.Duration
}

// WithRequestTimeout returns a RequestSyncOption that sets the request timeout.
// This timeout is used for the initial request validation phase.
func WithRequestTimeout(d time.Duration) RequestSyncOption {
	return func(cfg *requestSyncConfig) {
		cfg.requestTimeout = d
	}
}

// WithResponseTimeout returns a RequestSyncOption that sets the response timeout.
// This timeout is used for waiting for the event response.
func WithResponseTimeout(d time.Duration) RequestSyncOption {
	return func(cfg *requestSyncConfig) {
		cfg.responseTimeout = d
	}
}
