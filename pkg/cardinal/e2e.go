// E2E test helper with real (in-memory) NATS. Similar to the DST harness in dst.go, but
// runs the full World.run loop (ticking, snapshotting, restoring) while a separate goroutine
// sends randomized commands through real NATS subjects.
package cardinal

import (
	"context"
	"errors"
	"flag"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//nolint:gochecknoglobals // test flags registered via flag package
var (
	e2eRun      = flag.Bool("e2e.run", false, "enable e2e tests")
	e2eDuration = flag.Duration("e2e.duration", 30*time.Second, "e2e run duration")
)

// E2ESetupFunc builds and returns the world used by the E2E harness.
// The harness injects a test NATS URL via environment variables before invoking setup.
type E2ESetupFunc func() *World

const e2eCommandTimeout = 2 * time.Second

// RunE2E executes an end-to-end test. The setup function creates the world the same way production
// does; the harness transparently injects the in-memory test NATS URL. The harness starts the
// world's run loop in a background goroutine and sends randomized commands over NATS from the test
// goroutine.
//
// E2E tests are skipped by default. Use the -e2e.run flag to enable them:
//
//	go test ./pkg/cardinal/... -e2e.run
//
// The run duration defaults to 30s and can be overridden with -e2e.duration:
//
//	go test ./pkg/cardinal/... -e2e.run -e2e.duration=2m
func RunE2E(t *testing.T, setup E2ESetupFunc) {
	t.Helper()
	if !*e2eRun {
		t.Skip("skipping e2e test; use -e2e.run to enable")
	}

	prng := testutils.NewRand(t)
	cfg := newE2EConfig()
	fix := newE2EFixture(t, setup)

	// Build the weighted command ops for random selection.
	cmdNames := fix.world.commands.Names()
	cfg.addCommandOps(prng, cmdNames)
	cfg.log(t)

	// Start the world's run loop in a background goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() {
		runErr <- fix.world.run(ctx)
	}()

	var (
		stopWorldOnce sync.Once
		stopWorldErr  error
	)
	stopWorld := func() error {
		stopWorldOnce.Do(func() {
			cancel()
			stopWorldErr = <-runErr
		})
		return stopWorldErr
	}
	t.Cleanup(func() {
		err := stopWorld()
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("world.run returned unexpected error during cleanup: %v", err)
		}
	})

	// Send randomized commands for a wall-clock duration. Unlike DST, E2E runs the full world.run
	// loop in the background, so this harness does not directly drive or count ticks; runtime jitter
	// (scheduler/NATS/GC) can make elapsed time and tick count diverge.
	deadline := time.After(cfg.Duration)
sendLoop:
	for {
		select {
		case <-deadline:
			err := stopWorld()
			// context.Canceled is the expected exit.
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("world.run returned unexpected error: %v", err)
			}
			break sendLoop
		default:
			if len(cfg.CommandWeights) == 0 {
				// No commands registered; just wait for deadline.
				time.Sleep(10 * time.Millisecond)
				continue
			}
			name := testutils.RandWeightedOp(prng, cfg.CommandWeights)
			cmd := fix.randCommand(t, prng, name)
			fix.sendCommand(t, cmd)
		}
	}

	// Final validation after the world has fully stopped.
	ecs.CheckWorld(t, fix.world.world)
	_, err := fix.world.world.ToProto()
	require.NoError(t, err)
}

// -------------------------------------------------------------------------------------------------
// Config
// -------------------------------------------------------------------------------------------------

// e2eConfig holds all configurable parameters for an e2e test run.
type e2eConfig struct {
	Duration       time.Duration
	CommandWeights testutils.OpWeights
}

func newE2EConfig() e2eConfig {
	return e2eConfig{
		Duration:       *e2eDuration,
		CommandWeights: make(testutils.OpWeights),
	}
}

// addCommandOps adds per-command-type ops to the weights.
func (c *e2eConfig) addCommandOps(rng *rand.Rand, cmdOps []string) {
	if len(cmdOps) == 0 {
		return
	}
	c.CommandWeights = testutils.RandOpWeights(rng, cmdOps)
}

func (c *e2eConfig) log(t *testing.T) {
	t.Helper()
	t.Logf("E2E config:")
	t.Logf("  duration:      %s", c.Duration)
	t.Logf("  op_weights:    %v", c.CommandWeights)
}

// -------------------------------------------------------------------------------------------------
// Fixture
// -------------------------------------------------------------------------------------------------

type e2eFixture struct {
	world    *World
	client   *micro.Client // Separate NATS client for sending commands/queries
	cmdTypes map[string]reflect.Type
}

func newE2EFixture(t *testing.T, setup E2ESetupFunc) *e2eFixture {
	t.Helper()

	// Suppress world logs during E2E to reduce noise.
	t.Setenv("LOG_LEVEL", "disabled")

	// Start a dedicated in-memory NATS server for this test.
	natsServer, natsCleanup := newE2ENATS(t)
	t.Cleanup(natsCleanup)
	natsURL := natsServer.ClientURL()

	// Force all implicit micro.NewClient calls (service/snapshot) to use test NATS.
	t.Setenv("NATS_URL", natsURL)

	w := setup()
	require.NotNil(t, w, "e2e setup returned nil world")

	if w.options.NATSConfig == nil {
		w.options.NATSConfig = &micro.NATSConfig{Name: "e2e-service", URL: natsURL}
	} else {
		w.options.NATSConfig.URL = natsURL
	}

	// Initialize the world's NATS service (client, micro.Service, endpoints).
	require.NoError(t, w.service.init())

	// Replace inter-shard event handler with local assertions.
	// E2E runs a single world instance, so cross-shard requests would otherwise fail with
	// "no responders" and drown useful signal in log noise.
	w.events.RegisterHandler(event.KindInterShardCommand, func(evt event.Event) error {
		assert.Equal(t, event.KindInterShardCommand, evt.Kind, "nats: received wrong event kind")
		isc, ok := evt.Payload.(command.Command)
		assert.True(t, ok, "nats: ISC payload is %T, want command.Command", evt.Payload)
		if ok {
			assert.NotEmpty(t, isc.Name, "nats: inter-shard command has empty name")
			assert.NotNil(t, isc.Address, "nats: inter-shard command has nil address")
		}
		return nil
	})

	// Create a separate NATS client for sending commands (acts as an external caller).
	client, err := micro.NewClient(
		micro.WithNATSConfig(micro.NATSConfig{Name: "e2e-client", URL: natsURL}),
		micro.WithLogger(zerolog.Nop()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Cache concrete payload types for random command generation.
	cmdTypes := make(map[string]reflect.Type)
	for _, name := range w.commands.Names() {
		cmdTypes[name] = reflect.TypeOf(w.commands.Zero(name))
	}

	return &e2eFixture{
		world:    w,
		client:   client,
		cmdTypes: cmdTypes,
	}
}

func (f *e2eFixture) randCommand(t *testing.T, rng *rand.Rand, name string) *iscv1.Command {
	t.Helper()
	val := reflect.New(f.cmdTypes[name]).Elem()
	fillRandom(rng, val)
	p, ok := val.Interface().(command.Payload)
	require.True(t, ok, "type assertion to command.Payload failed for %q", name)
	payload, err := schema.Serialize(p)
	require.NoError(t, err)
	return &iscv1.Command{
		Name:    name,
		Address: f.world.address,
		Persona: &iscv1.Persona{Id: testutils.RandString(rng, 8)},
		Payload: payload,
	}
}

// sendCommand sends a command to the world's service over NATS.
func (f *e2eFixture) sendCommand(t *testing.T, cmd *iscv1.Command) {
	t.Helper()
	// 2s absorbs normal scheduling/reconnect jitter while still failing fast on deadlocks.
	ctx, cancel := context.WithTimeout(context.Background(), e2eCommandTimeout)
	defer cancel()
	_, err := f.client.Request(ctx, f.world.address, "command."+cmd.GetName(), cmd)
	require.NoError(t, err)
}

// -------------------------------------------------------------------------------------------------
// NATS
// -------------------------------------------------------------------------------------------------

// newE2ENATS starts a dedicated in-memory NATS server with JetStream enabled.
// The returned cleanup function shuts down the server and removes its temp storage.
func newE2ENATS(t *testing.T) (*server.Server, func()) {
	t.Helper()
	tempDir := filepath.Join(os.TempDir(), "nats-e2e-"+strconv.Itoa(os.Getpid())+"-"+t.Name())
	srv := test.RunServer(&server.Options{
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 true,
		NoSigs:                true,
		MaxControlLine:        4096,
		DisableShortFirstPing: true,
		JetStream:             true,
		StoreDir:              tempDir,
	})
	return srv, func() {
		srv.Shutdown()
		_ = os.RemoveAll(tempDir)
	}
}
