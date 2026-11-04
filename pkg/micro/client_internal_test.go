package micro

import (
	"errors"
	"testing"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NATS invokes ErrHandler with a nil subscription for connection-level async
// errors, so this test guards against regressing to a nil-pointer panic.
func TestClient_HandleErrorWithNilSubscription(t *testing.T) {
	t.Parallel()

	client := &Client{log: zerolog.Nop()}

	assert.NotPanics(t, func() {
		client.handleError(nil, nil, errors.New("transient connection error"))
	})
}

func TestNATSConfig_PingDefaults(t *testing.T) {
	cfg, err := env.ParseAs[NATSConfig]()
	require.NoError(t, err)

	assert.Equal(t, nats.DefaultPingInterval, cfg.PingInterval)
	assert.Equal(t, nats.DefaultMaxPingOut, cfg.MaxPingsOut)
	require.NoError(t, cfg.Validate())
}

func TestNATSConfig_PingEnvOverrides(t *testing.T) {
	t.Setenv("NATS_PING_INTERVAL", "15s")
	t.Setenv("NATS_MAX_PINGS_OUT", "3")

	cfg, err := env.ParseAs[NATSConfig]()
	require.NoError(t, err)

	assert.Equal(t, 15*time.Second, cfg.PingInterval)
	assert.Equal(t, 3, cfg.MaxPingsOut)
	require.NoError(t, cfg.Validate())
}

func TestNATSConfig_PingEnvInvalidFormat(t *testing.T) {
	t.Setenv("NATS_PING_INTERVAL", "soon")

	_, err := env.ParseAs[NATSConfig]()
	require.Error(t, err)
}

func TestNATSConfig_ValidatePing(t *testing.T) {
	t.Parallel()

	valid := NATSConfig{URL: "nats://localhost:4222", PingInterval: time.Second, MaxPingsOut: 1}

	tests := []struct {
		name   string
		mutate func(*NATSConfig)
	}{
		{name: "zero ping interval", mutate: func(c *NATSConfig) { c.PingInterval = 0 }},
		{name: "negative ping interval", mutate: func(c *NATSConfig) { c.PingInterval = -time.Second }},
		{name: "zero max pings out", mutate: func(c *NATSConfig) { c.MaxPingsOut = 0 }},
		{name: "negative max pings out", mutate: func(c *NATSConfig) { c.MaxPingsOut = -1 }},
	}

	require.NoError(t, valid.Validate())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tt.mutate(&cfg)
			require.Error(t, cfg.Validate())
		})
	}
}

func TestWithNATSConfig_PingFallbacks(t *testing.T) {
	t.Parallel()

	c := &Client{}
	WithNATSConfig(NATSConfig{URL: "nats://localhost:4222"})(c)
	assert.Equal(t, nats.DefaultPingInterval, c.natsConfig.PingInterval)
	assert.Equal(t, nats.DefaultMaxPingOut, c.natsConfig.MaxPingsOut)

	WithNATSConfig(NATSConfig{URL: "nats://localhost:4222", PingInterval: 5 * time.Second, MaxPingsOut: 4})(c)
	assert.Equal(t, 5*time.Second, c.natsConfig.PingInterval)
	assert.Equal(t, 4, c.natsConfig.MaxPingsOut)
}

func TestNewClient_AppliesPingOptions(t *testing.T) {
	t.Parallel()

	c, err := NewClient(
		WithNATSConfig(NATSConfig{
			Name:         "ping-test",
			URL:          TestNATS.ClientURL(),
			PingInterval: 7 * time.Second,
			MaxPingsOut:  5,
		}),
		WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	assert.Equal(t, 7*time.Second, c.Opts.PingInterval)
	assert.Equal(t, 5, c.Opts.MaxPingsOut)
}
