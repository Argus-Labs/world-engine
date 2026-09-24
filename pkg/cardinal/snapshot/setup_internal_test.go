package snapshot

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// TestNATS is the in-process JetStream-enabled NATS server shared by every test in this
// package. It mirrors the setup used by pkg/micro and pkg/cardinal, so the snapshot JetStream
// backend is exercised against the same nats-server v2.12.6 pinned in go.mod.
var TestNATS *server.Server

func TestMain(m *testing.M) {
	tempDir := filepath.Join(os.TempDir(), "nats-snapshot-test-"+strconv.Itoa(os.Getpid()))

	// Uses modified values of NATS's own default test server config.
	opts := &server.Options{
		Host:                  "127.0.0.1",
		Port:                  -1, // Random available port
		NoLog:                 true,
		NoSigs:                true,
		MaxControlLine:        4096,
		DisableShortFirstPing: true,
		JetStream:             true,
		StoreDir:              tempDir,
	}

	TestNATS = test.RunServer(opts)

	code := m.Run()

	TestNATS.Shutdown()
	if err := os.RemoveAll(tempDir); err != nil {
		log.Printf("failed to remove temp dir: %v", err)
	}
	os.Exit(code)
}

// randServiceAddress returns a fresh, collision-free ServiceAddress so each test owns its own
// ObjectStore bucket (the bucket name is derived from the address).
func randServiceAddress(t *testing.T) *micro.ServiceAddress {
	t.Helper()
	return micro.GetAddress(
		"r-"+strconv.FormatInt(rand.Int64(), 10),
		micro.RealmInternal,
		"o-"+strconv.FormatInt(rand.Int64(), 10),
		"p-"+strconv.FormatInt(rand.Int64(), 10),
		"s-"+strconv.FormatInt(rand.Int64(), 10),
	)
}

// requireObjectStoreMaxBytes reads the server-side MaxBytes for the given bucket's backing
// stream. It uses the same read path as the fix (js.Stream -> Info -> Config.MaxBytes) to assert
// the bind path did not mutate server state.
func requireObjectStoreMaxBytes(t *testing.T, js jetstream.JetStream, bucketName string) int64 {
	t.Helper()
	stream, err := js.Stream(t.Context(), fmt.Sprintf(objectStoreStreamNameTmpl, bucketName))
	require.NoError(t, err, "failed to look up stream for bucket %s", bucketName)
	info, err := stream.Info(t.Context())
	require.NoError(t, err, "failed to read stream info for bucket %s", bucketName)
	return info.Config.MaxBytes
}
