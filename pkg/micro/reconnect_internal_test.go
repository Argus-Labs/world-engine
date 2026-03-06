package micro

import (
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// reconnectDelay unit tests
// -------------------------------------------------------------------------------------------------

func TestReconnectDelay(t *testing.T) {
	t.Parallel()

	t.Run("first attempt uses first backoff value", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 500*time.Millisecond, reconnectDelay(0))
	})

	t.Run("last attempt uses last backoff value", func(t *testing.T) {
		t.Parallel()
		lastIdx := len(reconnectBackoff) - 1
		assert.Equal(t, 20000*time.Millisecond, reconnectDelay(lastIdx))
	})

	t.Run("beyond max attempt clamps to last value", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 20000*time.Millisecond, reconnectDelay(9999))
	})

	t.Run("negative attempt does not panic", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, 500*time.Millisecond, reconnectDelay(-1))
	})

	t.Run("delay increases over attempts", func(t *testing.T) {
		t.Parallel()
		assert.Greater(t, reconnectDelay(len(reconnectBackoff)-1), reconnectDelay(0))
	})
}

// -------------------------------------------------------------------------------------------------
// Reconnection integration test
// -------------------------------------------------------------------------------------------------
// Verifies that a client with our reconnection config (MaxReconnects=-1, CustomReconnectDelay)
// recovers after a NATS server restart and can resume request-reply.
// -------------------------------------------------------------------------------------------------

func TestClient_ReconnectsAfterServerRestart(t *testing.T) {
	t.Parallel()

	// Start a dedicated NATS server for this test (separate from the shared TestNATS).
	opts := &server.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		MaxControlLine:        4096,
		DisableShortFirstPing: true,
		StoreDir:              t.TempDir(),
	}
	srv := test.RunServer(opts)
	srvURL := srv.ClientURL()

	// Connect a client.
	client, err := NewClient(
		WithNATSConfig(NATSConfig{Name: "reconnect-test", URL: srvURL}),
		WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Set up a subscription and verify it works before the restart.
	sub, err := client.Subscribe("test.ping", func(msg *nats.Msg) {
		msg.Respond([]byte("pong"))
	})
	require.NoError(t, err)
	t.Cleanup(func() { sub.Unsubscribe() })
	require.NoError(t, client.Flush())

	msg, err := client.Conn.Request("test.ping", []byte("ping"), 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "pong", string(msg.Data))

	// Remember the port so we can restart on the same address.
	addr := srv.Addr()

	// Shut down the NATS server.
	srv.Shutdown()
	srv.WaitForShutdown()

	// Restart the NATS server on the same port.
	opts.Port = addr.(*net.TCPAddr).Port
	srv = test.RunServer(opts)
	t.Cleanup(func() { srv.Shutdown() })

	// Wait for the client to reconnect.
	require.Eventually(t, func() bool {
		return client.IsReconnecting() || client.IsConnected()
	}, 5*time.Second, 100*time.Millisecond, "client should be reconnecting or connected")

	require.Eventually(t, func() bool {
		return client.IsConnected()
	}, 10*time.Second, 100*time.Millisecond, "client should have reconnected")

	// Verify request-reply works again after reconnection.
	var reconnectedMsg *nats.Msg
	require.Eventually(t, func() bool {
		var reqErr error
		reconnectedMsg, reqErr = client.Conn.Request("test.ping", []byte("ping"), 2*time.Second)
		return reqErr == nil
	}, 5*time.Second, 200*time.Millisecond, "request-reply should work after reconnect")

	assert.Equal(t, "pong", string(reconnectedMsg.Data))
}
