package cardinal

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
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
		fmt.Fprintf(os.Stderr, "failed to remove temp dir: %v\n", err)
	}
	os.Exit(code)
}

func RandServiceAddress(prng *rand.Rand) *micro.ServiceAddress {
	return micro.GetAddress(
		"p-"+strconv.FormatInt(prng.Int64(), 10),
		"s-"+strconv.FormatInt(prng.Int64(), 10),
	)
}
