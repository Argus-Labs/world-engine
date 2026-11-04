package micro

import (
	"errors"
	"sync"
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
	t.Parallel()

	// Empty environment, so exported NATS_* vars in the shell cannot leak in.
	cfg, err := env.ParseAsWithOptions[NATSConfig](env.Options{Environment: map[string]string{}})
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
	t.Parallel()

	tests := []struct {
		name  string
		env   map[string]string
		field string
	}{
		{name: "malformed ping interval", env: map[string]string{"NATS_PING_INTERVAL": "soon"}, field: "PingInterval"},
		{name: "malformed max pings out", env: map[string]string{"NATS_MAX_PINGS_OUT": "abc"}, field: "MaxPingsOut"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Explicit environment, so each case isolates one field.
			_, err := env.ParseAsWithOptions[NATSConfig](env.Options{Environment: tt.env})
			require.ErrorContains(t, err, tt.field)
		})
	}
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

	const url = "nats://localhost:4222"
	tests := []struct {
		name         string
		in           NATSConfig
		wantInterval time.Duration
		wantMaxOut   int
	}{
		{"both zero", NATSConfig{URL: url}, nats.DefaultPingInterval, nats.DefaultMaxPingOut},
		{"zero interval", NATSConfig{URL: url, MaxPingsOut: 4}, nats.DefaultPingInterval, 4},
		{
			"zero max pings out",
			NATSConfig{URL: url, PingInterval: 5 * time.Second},
			5 * time.Second, nats.DefaultMaxPingOut,
		},
		{"both set", NATSConfig{URL: url, PingInterval: 5 * time.Second, MaxPingsOut: 4}, 5 * time.Second, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &Client{}
			WithNATSConfig(tt.in)(c)
			assert.Equal(t, tt.wantInterval, c.natsConfig.PingInterval)
			assert.Equal(t, tt.wantMaxOut, c.natsConfig.MaxPingsOut)
		})
	}
}

// One option reused across concurrent client setups must not share mutable state (run with -race).
func TestWithNATSConfig_ConcurrentReuse(t *testing.T) {
	t.Parallel()

	opt := WithNATSConfig(NATSConfig{URL: "nats://localhost:4222"})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			c := &Client{}
			opt(c)
			assert.Equal(t, nats.DefaultPingInterval, c.natsConfig.PingInterval)
		})
	}
	wg.Wait()
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
