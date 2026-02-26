// DST (Deterministic Simulation Testing) provides a game-logic-agnostic fuzzer and structural
// state checker for Cardinal. It generates random commands by introspecting registered command
// types (via reflection), injects engine operations (tick, restart, snapshot/restore) with
// randomized weights, and validates structural ECS invariants after every tick. Game logic
// correctness is irrelevant — only engine correctness matters.
//
// Usage from a game shard's test directory:
//
//	func TestDST(t *testing.T) {
//	    cardinal.RunDST(t, func(w *cardinal.World) {
//	        cardinal.RegisterSystem(w, system.MySystem)
//	        // ... register all systems
//	    })
//	}
package cardinal

import (
	"context"
	"flag"
	"maps"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var numTicks = flag.Int("dst.ticks", 1000, "number of ticks to run in DST")

// DSTSetupFunc registers systems, components, and commands on a World.
// It is called once during fixture creation, before the first tick.
type DSTSetupFunc func(world *World)

// RunDST executes a deterministic simulation test. The setup function registers game-specific
// systems; the harness handles everything else: randomized engine config, command generation,
// ticking, restart/restore operations, and structural invariant checking.
func RunDST(t *testing.T, setup DSTSetupFunc) {
	t.Helper()

	prng := testutils.NewRand(t)
	cfg := newDSTConfig(prng)
	fix := newDSTFixture(t, cfg, setup)

	// Add the world's commmands as operations in the dst config.
	cmdNames := fix.world.commands.Names()
	cmdOps := make([]string, 0, len(cmdNames))
	for _, name := range cmdNames {
		cmdOps = append(cmdOps, opCommandPrefix+name)
	}
	cfg.addCommandOps(prng, cmdOps)
	cfg.log(t)

	fix.logWorldState(t, "before")

	tick := 0
	for tick < cfg.Ticks {
		op := testutils.RandWeightedOp(prng, cfg.OpWeights)

		switch {
		case op == opTick:
			timestamp := time.Unix(int64(tick), 0)
			require.NoError(t, fix.world.Tick(context.Background(), timestamp))

			// Assert structural ECS invariants after every tick.
			ecs.CheckWorld(t, fix.world.world)

			tick++

		case strings.HasPrefix(op, opCommandPrefix):
			cmdName := strings.TrimPrefix(op, opCommandPrefix)
			cmd := fix.randCommand(prng, cmdName)
			require.NoError(t, fix.world.commands.Enqueue(cmd))

		case op == opRestart:
			fix.world.reset()
			fix.world.world.Init()
			// ecs.World.Tick returns early on the first tick after reset (only runs init systems).
			// Consume the init tick so subsequent ticks run normally.
			require.NoError(t, fix.world.Tick(context.Background(), time.Time{}))

		case op == opSnapshotRestore:
			fix.world.reset()
			require.NoError(t, fix.world.restore(context.Background()))

			// Verify snapshot roundtrip fidelity: restored state re-serializes to identical bytes.
			fix.verifySnapshotRoundtrip(t)
		}
	}

	fix.logWorldState(t, "after")
}

// Operations.
const (
	opTick            = "tick"
	opCommandPrefix   = "command:"
	opRestart         = "restart"
	opSnapshotRestore = "restore"
)

// engineOps are the non-command operations that may be randomly enabled.
var engineOps = []string{
	opRestart,
	opSnapshotRestore,
}

// -------------------------------------------------------------------------------------------------
// Config
// -------------------------------------------------------------------------------------------------

// dstConfig holds all configurable parameters for a DST run.
type dstConfig struct {
	Ticks        int
	OpWeights    testutils.OpWeights
	SnapshotRate uint32
}

func newDSTConfig(rng *rand.Rand) dstConfig {
	opWeights := testutils.RandOpWeights(rng, engineOps)
	// Tick must always be enabled so the simulation makes progress.
	opWeights[opTick] = uint64(1 + rng.IntN(100)) //nolint:gosec // not gonna happen
	return dstConfig{
		Ticks:        *numTicks,
		OpWeights:    opWeights,
		SnapshotRate: uint32(1 + rng.IntN(25)),
	}
}

// addCommandOps adds per-command-type ops to the weights.
func (c *dstConfig) addCommandOps(rng *rand.Rand, cmdOps []string) {
	if len(cmdOps) == 0 {
		return
	}
	cmdWeights := testutils.RandOpWeights(rng, cmdOps)
	maps.Copy(c.OpWeights, cmdWeights)
}

func (c *dstConfig) log(t *testing.T) {
	t.Helper()
	t.Logf("DST config:")
	t.Logf("  ticks:         %d", c.Ticks)
	t.Logf("  op_weights:    %v", c.OpWeights)
	t.Logf("  snapshot_rate: %d", c.SnapshotRate)
}

// -------------------------------------------------------------------------------------------------
// Fixture
// -------------------------------------------------------------------------------------------------

type dstFixture struct {
	world    *World
	storage  *memSnapshotStorage
	cmdTypes map[string]reflect.Type // command name -> concrete payload type
}

func newDSTFixture(t *testing.T, cfg dstConfig, setup DSTSetupFunc) *dstFixture {
	t.Helper()

	// Suppress world logs during DST to reduce noise.
	t.Setenv("LOG_LEVEL", "disabled")

	debug := false
	w, err := NewWorld(WorldOptions{
		Region:              "dst",
		Organization:        "dst",
		Project:             "dst",
		ShardID:             "0",
		TickRate:            1,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        cfg.SnapshotRate,
		Debug:               &debug,
	})
	require.NoError(t, err)

	// Register the user's systems (components, commands, events are auto-registered).
	setup(w)

	// Replace NATS event handlers with local handlers that assert structural invariants.
	w.events.RegisterHandler(event.KindDefault, func(evt event.Event) error {
		assert.Equal(t, event.KindDefault, evt.Kind, "nats: received non-default event kind")
		assert.NotNil(t, evt.Payload, "nats: received nil payload")
		return nil
	})
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

	// Replace snapshot storage with in-memory storage.
	storage := &memSnapshotStorage{t: t}
	w.snapshotStorage = storage

	// Initialize ECS schedulers and consume the init tick.
	w.world.Init()
	require.NoError(t, w.Tick(context.Background(), time.Unix(0, 0)))

	// Cache concrete payload types for random command generation.
	cmdTypes := make(map[string]reflect.Type)
	for _, name := range w.commands.Names() {
		cmdTypes[name] = reflect.TypeOf(w.commands.Zero(name))
	}

	return &dstFixture{
		world:    w,
		storage:  storage,
		cmdTypes: cmdTypes,
	}
}

func (f *dstFixture) logWorldState(t *testing.T, label string) {
	t.Helper()
	ws, err := f.world.world.ToProto()
	if err != nil {
		t.Logf("world state (%s): failed to serialize: %v", label, err)
		return
	}
	t.Logf("world state (%s):", label)
	t.Logf("  next_entity_id: %d", ws.GetNextId())
	t.Logf("  free_ids:       %v", ws.GetFreeIds())
	t.Logf("  archetypes:     %d", len(ws.GetArchetypes()))
	for _, arch := range ws.GetArchetypes() {
		compNames := make([]string, 0, len(arch.GetColumns()))
		for _, col := range arch.GetColumns() {
			compNames = append(compNames, col.GetComponentName())
		}
		t.Logf("    archetype %d: entities=%d components=%v",
			arch.GetId(), len(arch.GetEntities()), compNames)
	}
}

func (f *dstFixture) randCommand(rng *rand.Rand, name string) *iscv1.Command {
	val := reflect.New(f.cmdTypes[name]).Elem()
	fillRandom(rng, val) // Recursive so not inlined
	payload, err := schema.Serialize(val.Interface().(command.Payload))
	if err != nil {
		panic(err)
	}
	return &iscv1.Command{
		Name:    name,
		Address: f.world.address,
		Persona: &iscv1.Persona{Id: testutils.RandString(rng, 8)},
		Payload: payload,
	}
}

func (f *dstFixture) verifySnapshotRoundtrip(t *testing.T) {
	t.Helper()
	if f.storage.snap == nil {
		return // No snapshot stored yet, nothing to verify.
	}

	// Serialize the restored state and compare with what was stored.
	worldState, err := f.world.world.ToProto()
	require.NoError(t, err)
	restoredBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(worldState)
	require.NoError(t, err)

	assert.Equal(t, f.storage.snap.Data, restoredBytes,
		"snapshot roundtrip: restored state differs from stored snapshot")
}

// fillRandom recursively fills a reflect.Value with random data based on its type.
func fillRandom(prng *rand.Rand, v reflect.Value) {
	switch v.Kind() { //nolint:exhaustive // only handle types used in command payloads
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(prng.Int64())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(prng.Uint64())
	case reflect.Float32, reflect.Float64:
		v.SetFloat(prng.Float64() * 1000)
	case reflect.Bool:
		v.SetBool(prng.IntN(2) == 0)
	case reflect.String:
		const chars = "abcdefghijklmnopqrstuvwxyz"
		n := 1 + prng.IntN(12)
		b := make([]byte, n)
		for i := range b {
			b[i] = chars[prng.IntN(len(chars))]
		}
		v.SetString(string(b))
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Field(i).CanSet() {
				fillRandom(prng, v.Field(i))
			}
		}
	case reflect.Slice:
		n := prng.IntN(5)
		slice := reflect.MakeSlice(v.Type(), n, n)
		for i := range n {
			fillRandom(prng, slice.Index(i))
		}
		v.Set(slice)
	case reflect.Array:
		for i := range v.Len() {
			fillRandom(prng, v.Index(i))
		}
	}
}

// -------------------------------------------------------------------------------------------------
// In-memory snapshot storage
// -------------------------------------------------------------------------------------------------

type memSnapshotStorage struct {
	t    *testing.T
	snap *snapshot.Snapshot
}

var _ snapshot.Storage = (*memSnapshotStorage)(nil)

func (m *memSnapshotStorage) Store(_ context.Context, s *snapshot.Snapshot) error {
	// Invariant: data must be non-empty (serialized ECS world always produces bytes).
	assert.NotEmpty(m.t, s.Data, "snapshot: Store called with empty data")
	// Invariant: data must be valid protobuf (must unmarshal into WorldState).
	var ws cardinalv1.WorldState
	assert.NoError(m.t, proto.Unmarshal(s.Data, &ws), "snapshot: Store data is not valid WorldState protobuf")

	cp := *s
	cp.Data = make([]byte, len(s.Data))
	copy(cp.Data, s.Data)
	m.snap = &cp
	return nil
}

func (m *memSnapshotStorage) Load(_ context.Context) (*snapshot.Snapshot, error) {
	if m.snap == nil {
		return nil, snapshot.ErrSnapshotNotFound
	}

	// Return a defensive copy so callers cannot corrupt stored state.
	cp := *m.snap
	cp.Data = make([]byte, len(m.snap.Data))
	copy(cp.Data, m.snap.Data)

	return &cp, nil
}
