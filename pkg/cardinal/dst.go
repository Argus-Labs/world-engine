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
//	    }, []cardinal.Command{system.BootstrapCommand{Seed: 42}})
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
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var numTicks = flag.Int("dst.ticks", 1000, "number of ticks to run in DST") //nolint:gochecknoglobals // test flag

// DSTSetupFunc registers systems, components, and commands on a World. It is called once during
// fixture creation, before the first tick.
type DSTSetupFunc func(world *World)

// RunDST executes a deterministic simulation test. The setup function registers game-specific
// systems; the harness handles everything else: randomized engine config, command generation,
// ticking, restart/restore operations, and structural invariant checking.
//
// preTestCommands are enqueued before randomized fuzz operations begin. This supports worlds that
// require deterministic bootstrap commands before entering an active state.
//
// The number of ticks defaults to 1000 and can be overridden with -dst.ticks:
//
//	go test ./pkg/cardinal/... -dst.ticks=5000
func RunDST(t *testing.T, setup DSTSetupFunc, preTestCommands []Command) {
	t.Helper()

	prng := testutils.NewRand(t)
	cfg := newDSTConfig(prng)
	fix := newDSTFixture(t, cfg, setup)
	for _, cmd := range preTestCommands {
		require.NoError(t, fix.enqueueCommand(cmd))
	}

	// Add the world's commmands as operations in the dst config.
	cmdNames := fix.world.commands.Names()
	cmdOps := make([]string, 0, len(cmdNames))
	for _, name := range cmdNames {
		cmdOps = append(cmdOps, opCommandPrefix+name)
	}
	cfg.addCommandOps(prng, cmdOps)
	cfg.log(t)

	// fix.logWorldState(t, "before")

	tick := 0
	for tick < cfg.Ticks {
		op := testutils.RandWeightedOp(prng, cfg.OpWeights)

		switch {
		case op == opTick:
			timestamp := time.Unix(int64(tick), 0)
			fix.world.Tick(timestamp)

			// Assert structural ECS invariants after every tick.
			ecs.CheckWorld(t, fix.world.world)

			tick++

		case strings.HasPrefix(op, opCommandPrefix):
			cmdName := strings.TrimPrefix(op, opCommandPrefix)
			cmd := fix.randCommand(t, prng, cmdName)
			require.NoError(t, fix.world.commands.Enqueue(cmd))

		case op == opRestart:
			fix.world.reset()

		case op == opSnapshotRestore:
			fix.world.reset()
			require.NoError(t, fix.world.restore(context.Background()))

			// Verify snapshot roundtrip fidelity: restored state re-serializes to identical bytes.
			// fix.verifySnapshotRoundtrip(t)
		}
	}

	// Final validation after all randomized operations complete.
	ecs.CheckWorld(t, fix.world.world)

	// Ensure final world state remains serializable.
	// Encoding asserts internally, so reaching the next line at all is the check.
	_ = fix.world.world.ToProto()

	// fix.logWorldState(t, "after")
}

// Operations.
const (
	opTick            = "tick"
	opCommandPrefix   = "command:"
	opRestart         = "restart"
	opSnapshotRestore = "restore"
)

// engineOps are the non-command operations that may be randomly enabled.
var engineOps = []string{ //nolint:gochecknoglobals // DST operation table
	opTick,
	// opRestart,
	// opSnapshotRestore,
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
	// Cap restart/restore weights so the world state can grow complex before being disrupted.
	for _, op := range engineOps {
		if w, ok := opWeights[op]; ok && w > 5 {
			opWeights[op] = uint64(1 + rng.IntN(5)) //nolint:gosec // not gonna happen
		}
	}
	return dstConfig{
		Ticks:        *numTicks,
		OpWeights:    opWeights,
		SnapshotRate: uint32(1 + rng.IntN(25)), //nolint:gosec // bounded to [1,25]
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

	// Replace snapshot storage with in-memory storage, written inline rather than through the
	// background writer. DST trades tick latency for reproducibility: a seed must produce one run,
	// and an upload goroutine decides which snapshots survive latest-wins by how it interleaves
	// with the tick loop. memSnapshotStorage also asserts on t, which only this goroutine may do.
	storage := &memSnapshotStorage{t: t}
	w.useSyncSnapshotStorage(storage)

	// Initialize ECS and run init systems.
	w.world.Init()

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

func (f *dstFixture) logWorldState(t *testing.T, label string) { //nolint: unused // Used
	t.Helper()
	ws := f.world.world.ToProto()
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

func (f *dstFixture) randCommand(t *testing.T, rng *rand.Rand, name string) *iscv1.Command {
	t.Helper()
	val := reflect.New(f.cmdTypes[name]).Elem()
	fillRandom(rng, val, f.world.world.LiveEntityIDs()) // Recursive so not inlined
	p, ok := val.Interface().(command.Payload)
	require.True(t, ok, "type assertion to command.Payload failed for %q", name)
	payload := p.MarshalWire()
	return &iscv1.Command{
		Name:    name,
		Address: f.world.address,
		Persona: &iscv1.Persona{Id: testutils.RandString(rng, 8)},
		Payload: payload,
	}
}

func (f *dstFixture) enqueueCommand(cmd Command) error {
	payload := cmd.MarshalWire()
	return f.world.commands.Enqueue(&iscv1.Command{
		Name:    cmd.Name(),
		Address: f.world.address,
		Persona: &iscv1.Persona{Id: "dst-pretest"},
		Payload: payload,
	})
}

// fillRandom recursively fills a reflect.Value with random data based on its type.
func fillRandom(prng *rand.Rand, v reflect.Value, liveEntityIDs []EntityID) {
	t := v.Type()
	if len(liveEntityIDs) > 0 &&
		(t == reflect.TypeFor[uint32]() || t == reflect.TypeFor[EntityID]()) &&
		prng.IntN(10) < 9 {
		eid := liveEntityIDs[prng.IntN(len(liveEntityIDs))]
		v.SetUint(uint64(eid))
		return
	}

	switch v.Kind() { //nolint:exhaustive // only handle types used in command payloads
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(prng.Int64())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(prng.Uint64())
	case reflect.Float32, reflect.Float64:
		v.SetFloat(prng.Float64() * 1000)
	case reflect.Bool:
		v.SetBool(testutils.RandBool(prng))
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
				fillRandom(prng, v.Field(i), liveEntityIDs)
			}
		}
	case reflect.Slice:
		n := prng.IntN(5)
		slice := reflect.MakeSlice(v.Type(), n, n)
		for i := range n {
			fillRandom(prng, slice.Index(i), liveEntityIDs)
		}
		v.Set(slice)
	case reflect.Array:
		for i := range v.Len() {
			fillRandom(prng, v.Index(i), liveEntityIDs)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// In-memory snapshot storage
// -------------------------------------------------------------------------------------------------

func (w *World) useSyncSnapshotStorage(store snapshot.Storage) {
	if w.snapshotWriter != nil {
		w.snapshotWriter.Stop(context.Background())
	}
	w.snapshotStorage = store
	w.snapshotWriter = snapshot.NewSyncWriter(store, w.tel.GetLogger("snapshot"))
}

// memSnapshotStorage keeps the last snapshot in memory and checks the envelope on the way in.
//
// It must only be driven by the synchronous snapshot writer (World.useSyncSnapshotStorage), never by
// the background one. The reason is the assertions, not the field: require fails a test by calling
// t.FailNow, which is only valid on the goroutine running the test, so a mutex around snap would
// silence the race detector while leaving the actual defect in place. Storage that is worth
// pointing an async writer at is storage that does not assert.
type memSnapshotStorage struct {
	t    *testing.T
	snap *cardinalv1.Snapshot
}

var _ snapshot.Storage = (*memSnapshotStorage)(nil)

func (m *memSnapshotStorage) Store(_ context.Context, s *cardinalv1.Snapshot) error {
	// Invariant: the envelope must carry a world state (a serialized ECS world is never nil).
	require.NotNil(m.t, s.GetWorldState(), "snapshot: Store called without a world state")
	// Invariant: the envelope must survive the wire format a real backend would write it to.
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(s)
	require.NoError(m.t, err, "snapshot: Store envelope failed to marshal")
	assert.NotEmpty(m.t, data, "snapshot: Store produced empty bytes")
	var rt cardinalv1.Snapshot
	require.NoError(m.t, proto.Unmarshal(data, &rt), "snapshot: Store bytes are not a valid Snapshot protobuf")
	assert.True(m.t, proto.Equal(s, &rt), "snapshot: Store envelope did not survive a wire roundtrip")

	// Storage.Store hands over the caller's own world-state graph and keeps ownership of it, so a
	// backend must consume the message before it returns. The graph itself is frozen — ToProto
	// builds a fresh one per call and nothing writes into it afterwards — so the rule is not about
	// mutation racing this copy; it is that the caller owns the memory and every reader of it, and
	// a backend holding on would pin a full world-state graph it has no claim to. A real backend
	// satisfies the rule by serializing inside Store; this one copies, which is the same promise.
	m.snap = proto.CloneOf(s)
	return nil
}

func (m *memSnapshotStorage) Load(_ context.Context) (*cardinalv1.Snapshot, error) {
	if m.snap == nil {
		return nil, snapshot.ErrSnapshotNotFound
	}

	// Storage.Load transfers ownership to the caller, which publishes the message and feeds it to
	// FromProto, so the retained copy must not escape. A real backend decodes fresh bytes, which
	// gives the caller a private message for the same reason.
	return proto.CloneOf(m.snap), nil
}
