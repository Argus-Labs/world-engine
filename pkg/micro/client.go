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
	ErrFailedToConnect   = eris.New("failed to connect to NATS server")
	ErrFailedToSubscribe = eris.New("failed to subscribe to subject")
	ErrFailedToPublish   = eris.New("failed to publish message")
	ErrInvalidConfig     = eris.New("invalid NATS configuration")
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
