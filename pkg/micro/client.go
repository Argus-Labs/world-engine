package micro

import (
	"context"
	"math/rand/v2"
	"time"

	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/caarlos0/env/v11"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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
		return nil, eris.Wrap(err, "failed to parse NATS config")
	}

	// Apply options that may override environment variables.
	for _, opt := range opts {
		opt(c)
	}

	if err := c.natsConfig.Validate(); err != nil {
		return nil, eris.Wrap(err, "invalid NATS config")
	}

	// Init NATS options with validated configuration.
	// Reconnection strategy mirrors the NATS CLI: infinite retries with exponential backoff + jitter.
	natsOpts := []nats.Option{
		nats.Name(c.natsConfig.Name),
		nats.MaxReconnects(-1),
		nats.IgnoreAuthErrorAbort(),
		nats.CustomReconnectDelay(reconnectDelay),
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
		return nil, eris.Wrap(err, "failed to connect to NATS server")
	}
	c.Conn = conn

	c.log.Info().
		Str("url", c.ConnectedUrl()).
		Str("name", c.natsConfig.Name).
		Msg("Connected to NATS server")

	return c, nil
}

// Request sends a request to a subject and waits for a response (request-reply pattern).
// The timeout should be set in ctx.
func (c *Client) Request(
	ctx context.Context,
	address *ServiceAddress,
	endpoint string,
	payload proto.Message,
) (*microv1.Response, error) {
	var anyPayload *anypb.Any
	var err error

	if payload != nil {
		anyPayload, err = anypb.New(payload)
		if err != nil {
			return nil, eris.Wrap(err, "failed to create Any payload")
		}
	}

	req := &microv1.Request{
		ServiceAddress: address,
		Payload:        anyPayload,
	}

	reqBytes, err := proto.Marshal(req)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal request")
	}

	msg, err := c.RequestWithContext(ctx, Endpoint(address, endpoint), reqBytes)
	if err != nil {
		return nil, eris.Wrap(err, "failed to send request")
	}

	var res microv1.Response
	if err := proto.Unmarshal(msg.Data, &res); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal response")
	}

	// Check for application-level errors in the response status.
	status := res.GetStatus()
	if status.GetCode() != 0 {
		return nil, eris.New(status.GetMessage())
	}

	return &res, nil
}

// RequestAndSubscribe sends a message to the send endpoint and waits for a message on the receive endpoint.
// This is useful when the response comes on a different subject than the request (e.g., events).
// The timeout should be set in ctx.
func (c *Client) RequestAndSubscribe(
	ctx context.Context,
	sendAddress *ServiceAddress,
	sendEndpoint string,
	receiveAddress *ServiceAddress,
	receiveEndpoint string,
	payload proto.Message,
) (*nats.Msg, error) {
	receiveSubject := Endpoint(receiveAddress, receiveEndpoint)

	// Subscribe BEFORE sending request to prevent race condition where response arrives
	// before we're listening.
	sub, err := c.SubscribeSync(receiveSubject)
	if err != nil {
		return nil, eris.Wrap(err, "failed to subscribe to receive subject")
	}
	defer func() {
		if err := sub.Unsubscribe(); err != nil {
			c.log.Warn().Err(err).Str("subject", receiveSubject).Msg("Failed to unsubscribe")
		}
	}()

	// Send request. If it fails, return early without waiting for response.
	_, err = c.Request(ctx, sendAddress, sendEndpoint, payload)
	if err != nil {
		return nil, eris.Wrap(err, "send failed")
	}

	// Wait for message on the subscription.
	msg, err := sub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, eris.Wrap(err, "failed to receive message")
	}

	return msg, nil
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

// reconnectBackoff defines the exponential backoff delays in milliseconds.
// Copied from the NATS CLI's DefaultBackoff: https://github.com/nats-io/natscli/blob/main/internal/util/backoff.go
// Starts at 500ms and ramps up to 20s over 44 steps.
//
//nolint:gochecknoglobals // package-level lookup table is appropriate here.
var reconnectBackoff = []int{
	500, 750, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 4500, 5000,
	5500, 5750, 6000, 6500, 7000, 7500, 8000, 8500, 9000, 9500, 10000,
	10500, 10750, 11000, 11500, 12000, 12500, 13000, 13500, 14000, 14500, 15000,
	15500, 15750, 16000, 16500, 17000, 17500, 18000, 18500, 19000, 19500, 20000,
}

// reconnectDelay returns a jittered backoff duration for the given reconnect attempt.
// The delay is randomized in the range [0.5*base .. 1.5*base) to prevent thundering herd.
// Logic copied from NATS CLI: https://github.com/nats-io/natscli/blob/main/internal/util/backoff.go
// Note: NATS increments the attempt counter before calling this function, so the first
// real call is reconnectDelay(1). Index 0 is never used in practice.
func reconnectDelay(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts >= len(reconnectBackoff) {
		attempts = len(reconnectBackoff) - 1
	}
	return time.Duration(jitter(reconnectBackoff[attempts])) * time.Millisecond
}

// jitter returns a random integer uniformly distributed in the range [0.5*millis .. 1.5*millis).
func jitter(millis int) int {
	if millis == 0 {
		return 0
	}
	return millis/2 + rand.IntN(millis) //nolint:gosec // jitter doesn't need crypto rand.
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

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
