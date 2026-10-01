package micro

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

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

// TestClient_HandleDisconnectLogsConfiguredURL guards the bug where
// handleDisconnect logged nc.ConnectedUrl(), which nats.go guarantees to
// return "" inside a disconnect callback (the connection status has already
// transitioned away from CONNECTED). The handler must instead log the
// configured NATS URL (c.natsConfig.URL).
//
// We reproduce the disconnect-time condition deterministically: connect a raw
// nats.Conn then close it (status -> CLOSED), which makes ConnectedUrl()
// return "" — exactly as it would during a real disconnect callback.
func TestClient_HandleDisconnectLogsConfiguredURL(t *testing.T) {
	t.Parallel()

	// Connect a raw nats.Conn (no handlers) to the shared test server and close
	// it. After close the status is CLOSED, so ConnectedUrl() returns "".
	nc, err := nats.Connect(TestNATS.ClientURL())
	require.NoError(t, err)
	nc.Close()

	// Precondition: this is the exact condition under which the bug manifested —
	// ConnectedUrl() is empty once the connection is no longer CONNECTED.
	require.Empty(t, nc.ConnectedUrl(), "precondition: ConnectedUrl() must be empty when not CONNECTED")

	const configuredURL = "nats://configured-disconnect-test:4222"

	cases := []struct {
		name    string
		discErr error
		level   string
		msg     string
	}{
		{"with error", errors.New("connection reset"), "error", "Disconnected from NATS with error"},
		{"without error", nil, "warn", "Disconnected from NATS (no error)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			client := &Client{
				Conn:       nc,
				log:        zerolog.New(&buf),
				natsConfig: NATSConfig{URL: configuredURL},
			}

			client.handleDisconnect(nc, tc.discErr)

			// Each zerolog call emits one JSON object terminated by a newline;
			// pick the first non-empty line.
			var entry map[string]any
			for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte{'\n'}) {
				if len(line) == 0 {
					continue
				}
				require.NoError(t, json.Unmarshal(line, &entry))
				break
			}
			require.NotNil(t, entry, "expected a log entry, got empty buffer")

			// The fix: nats_url must be the configured URL, never the empty
			// string that ConnectedUrl() yields during a disconnect.
			assert.Equal(t, configuredURL, entry["nats_url"],
				"nats_url must be the configured URL, not ConnectedUrl()")
			assert.Equal(t, tc.level, entry["level"])
			assert.Contains(t, entry["message"], tc.msg)
			// reconnect_attempts must still be present (regression guard).
			assert.Contains(t, entry, "reconnect_attempts")
		})
	}
}
