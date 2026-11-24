package cardinal

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorldShutdownClosesNATSConnections verifies that a world leaves no NATS connection open after
// shutdown, so a host that creates and stops worlds in one process does not accumulate clients.
// Nop storage opens no connection of its own and is the control.
func TestWorldShutdownClosesNATSConnections(t *testing.T) {
	cases := []struct {
		storage snapshot.StorageType
		clients int // Connections a started world holds: the service, plus JetStream storage's own
	}{
		{storage: snapshot.StorageTypeNop, clients: 1},
		{storage: snapshot.StorageTypeJetStream, clients: 2},
	}

	for _, tc := range cases {
		t.Run(tc.storage.String(), func(t *testing.T) {
			srv := startShutdownTestNATS(t, true)
			w := newShutdownTestWorld(t, tc.storage)

			require.NoError(t, w.service.init("127.0.0.1:0"))
			require.Equal(t, tc.clients, srv.NumClients())

			// Store a snapshot so storage has used its connection before shutdown.
			w.init()
			w.Tick(time.Unix(1, 0))

			w.shutdown()

			requireNoNATSClients(t, srv)
		})
	}
}

// TestNewWorldClosesSnapshotConnectionOnStorageError verifies that NewWorld releases the snapshot
// storage connection when storage setup fails after connecting, here because the server has no
// JetStream.
func TestNewWorldClosesSnapshotConnectionOnStorageError(t *testing.T) {
	srv := startShutdownTestNATS(t, false)

	_, err := NewWorld(shutdownTestWorldOptions(snapshot.StorageTypeJetStream))
	require.Error(t, err)

	requireNoNATSClients(t, srv)
}

// startShutdownTestNATS starts a NATS server for one test and points NATS_URL at it. Tests count
// its connections, so they cannot share TestNATS with parallel tests.
func startShutdownTestNATS(t *testing.T, jetStream bool) *server.Server {
	t.Helper()

	srv := test.RunServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // Random available port
		NoLog:     true,
		NoSigs:    true,
		JetStream: jetStream,
		StoreDir:  t.TempDir(),
	})
	t.Cleanup(srv.Shutdown)

	// Snapshot storage reads the NATS config from the environment.
	t.Setenv("NATS_URL", srv.ClientURL())
	t.Setenv("NATS_CREDENTIALS_FILE", "")
	t.Setenv("LOG_LEVEL", "disabled")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "") // Shutdown would otherwise wait on the default collector

	return srv
}

func newShutdownTestWorld(t *testing.T, storage snapshot.StorageType) *World {
	t.Helper()

	w, err := NewWorld(shutdownTestWorldOptions(storage))
	require.NoError(t, err)

	return w
}

func shutdownTestWorldOptions(storage snapshot.StorageType) WorldOptions {
	debug := false

	return WorldOptions{
		Project:             "shutdown",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: storage,
		SnapshotRate:        1,
		Debug:               &debug,
	}
}

// requireNoNATSClients waits for the server to drop every client connection.
func requireNoNATSClients(t *testing.T, srv *server.Server) {
	t.Helper()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Zero(c, srv.NumClients(), "NATS connections still open")
	}, 2*time.Second, 10*time.Millisecond)
}
