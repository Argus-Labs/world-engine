package micro

import (
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	TestNATS *server.Server
)

func TestMain(m *testing.M) {
	tempDir := filepath.Join(os.TempDir(), "nats-test-shared-"+strconv.Itoa(os.Getpid()))

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

func NewTestClient2(t *testing.T) *Client {
	t.Helper()

	assert.NotNil(t, TestNATS, "test NATS server is not running")
	c, err := NewClient(
		WithNATSConfig(NATSConfig{Name: "test-client", URL: TestNATS.ClientURL()}),
		WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		c.Close()
	})

	return c
}

func RandServiceAddress(t *testing.T, rng *rand.Rand) *ServiceAddress {
	t.Helper()

	return GetAddress(
		"r-"+strconv.FormatInt(rng.Int64(), 10),
		RealmInternal,
		"o-"+strconv.FormatInt(rng.Int64(), 10),
		"p-"+strconv.FormatInt(rng.Int64(), 10),
		"s-"+strconv.FormatInt(rng.Int64(), 10),
	)
}
