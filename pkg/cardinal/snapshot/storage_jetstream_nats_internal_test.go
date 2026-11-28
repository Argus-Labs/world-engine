package snapshot

import (
	"path/filepath"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJetStreamStorageUsesExplicitNATSConfig runs with NATS_URL pointing at one server and an
// explicit NATSConfig pointing at another. Each server holds its own snapshot in the same bucket.
func TestJetStreamStorageUsesExplicitNATSConfig(t *testing.T) {
	explicitServer := startJetStreamServer(t)
	envServer := startJetStreamServer(t)
	address := micro.GetAddress("project", "shard")

	t.Setenv("NATS_NAME", "env-client")
	t.Setenv("NATS_CREDENTIALS_FILE", "")
	t.Setenv("CARDINAL_SNAPSHOT_STORAGE_MAX_BYTES", "")

	t.Setenv("NATS_URL", explicitServer.ClientURL())
	storeEnvSnapshot(t, address, "explicit server snapshot")
	t.Setenv("NATS_URL", envServer.ClientURL())
	storeEnvSnapshot(t, address, "env server snapshot")

	t.Run("explicit config wins over env", func(t *testing.T) {
		// Connecting fails if this env credentials file leaks into the explicit config.
		t.Setenv("NATS_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "missing.creds"))

		cfg := &micro.NATSConfig{Name: "explicit-client", URL: explicitServer.ClientURL()}
		want := *cfg

		storage, err := NewJetStreamStorage(JetStreamStorageOptions{
			Address:    address,
			Logger:     zerolog.Nop(),
			NATSConfig: cfg,
		})
		require.NoError(t, err)

		data, err := storage.Load(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "explicit server snapshot", string(data))
		assert.Equal(t, want, *cfg, "NewJetStreamStorage changed the caller's NATSConfig")
	})

	t.Run("env config without explicit config", func(t *testing.T) {
		storage, err := NewJetStreamStorage(JetStreamStorageOptions{
			Address: address,
			Logger:  zerolog.Nop(),
		})
		require.NoError(t, err)

		data, err := storage.Load(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "env server snapshot", string(data))
	})
}

// startJetStreamServer starts a JetStream-enabled NATS server that shuts down with the test.
func startJetStreamServer(t *testing.T) *server.Server {
	t.Helper()

	srv := test.RunServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random available port
		NoLog:     true,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	return srv
}

// storeEnvSnapshot stores data through storage configured only by the NATS_* environment.
func storeEnvSnapshot(t *testing.T, address *micro.ServiceAddress, data string) {
	t.Helper()

	storage, err := NewJetStreamStorage(JetStreamStorageOptions{
		Address: address,
		Logger:  zerolog.Nop(),
	})
	require.NoError(t, err)
	require.NoError(t, storage.Store(t.Context(), 0, []byte(data)))
}
