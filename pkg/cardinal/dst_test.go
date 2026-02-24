package cardinal

import (
	"context"
	"flag"
	"math/rand/v2"
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
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

var dstNumTicks = flag.Int("dst.ticks", 1000, "number of ticks to run in DST")

func TestDST(t *testing.T) {
	rng := testutils.NewRand(t)
	cfg := newDSTConfig(rng)
	cfg.log(t)

	fix := newDSTFixture(t, rng, cfg)

	for tick := range cfg.Ticks {
		op := testutils.RandWeightedOp(rng, cfg.OpWeights)

		switch op {
		case dstOpJoinGame:
			dstDoJoinGame(t, fix, rng)
		case dstOpAttack:
			dstDoAttack(t, fix, rng)
		case dstOpRestart:
			dstDoRestart(t, fix)
		case dstOpSnapshotRestore:
			dstDoSnapshotRestore(t, fix)
		}

		// Reset processed command tracking before the tick.
		dstTracker.reset()

		timestamp := time.Unix(int64(tick), 0)
		require.NoError(t, fix.world.Tick(context.Background(), timestamp))

		// Assert every enqueued command was processed by its system.
		require.ElementsMatch(t, fix.nats.pending, dstTracker.processed,
			"tick %d: enqueued commands do not match processed commands", tick)

		// Assert every event emitted by systems was received by the event handler.
		require.ElementsMatch(t, dstTracker.events, fix.nats.events,
			"tick %d: emitted events do not match received events", tick)

		// Assert every inter-shard command emitted by systems was received by the ISC handler.
		require.ElementsMatch(t, dstTracker.iscCommands, fix.nats.iscEvents,
			"tick %d: emitted ISC commands do not match received ISC commands", tick)

		fix.nats.clear()
	}
}

// dstConfig holds all configurable parameters for a DST run.
type dstConfig struct {
	// Simulation
	Ticks     int                 // Total number of ticks to simulate
	OpWeights testutils.OpWeights // Weighted operation selection

	// Fault injection: snapshot storage
	StoreFaultRate float64 // Probability [0,1] that snapshot Store fails
	LoadFaultRate  float64 // Probability [0,1] that snapshot Load fails
}

func newDSTConfig(rng *rand.Rand) dstConfig {
	return dstConfig{
		Ticks:          *dstNumTicks,
		OpWeights:      testutils.RandOpWeights(rng, dstOps),
		StoreFaultRate: 0, // TODO: randomize when storage fault injection is implemented
		LoadFaultRate:  0, // TODO: randomize when storage fault injection is implemented
	}
}

func (c *dstConfig) log(t *testing.T) {
	t.Helper()
	t.Logf("DST config:")
	t.Logf("  ticks:              %d", c.Ticks)
	t.Logf("  op_weights:         %v", c.OpWeights)
	t.Logf("  store_fault_rate:   %.2f", c.StoreFaultRate)
	t.Logf("  load_fault_rate:    %.2f", c.LoadFaultRate)
}

// DST operations.
const (
	dstOpJoinGame        = "join_game"
	dstOpAttack          = "attack"
	dstOpRestart         = "restart"
	dstOpSnapshotRestore = "snapshot_restore"
)

var dstOps = []string{
	dstOpJoinGame,
	dstOpAttack,
	dstOpRestart,
	dstOpSnapshotRestore,
}

func dstDoJoinGame(t *testing.T, fix *dstFixture, rng *rand.Rand) {
	t.Helper()
	cmd := dstJoinGame{
		Nickname:     testutils.RandString(rng, 6),
		HP:           1 + rng.Int(),
		ShieldPoints: 1 + rng.Int(),
	}
	persona := testutils.RandString(rng, 8)
	require.NoError(t, fix.nats.enqueueCommand(cmd, persona))
}

func dstDoAttack(t *testing.T, fix *dstFixture, rng *rand.Rand) {
	t.Helper()
	// Pick a random entity ID from the world. If no entities exist, skip.
	entities := fix.allEntityIDs()
	if len(entities) == 0 {
		return
	}
	target := entities[rng.IntN(len(entities))]
	cmd := dstAttack{
		TargetID: target,
		Damage:   1 + rng.IntN(50),
	}
	persona := testutils.RandString(rng, 8)
	require.NoError(t, fix.nats.enqueueCommand(cmd, persona))
}

func dstDoRestart(t *testing.T, fix *dstFixture) {
	t.Helper()
	fix.world.reset()
	fix.world.world.Init()
}

func dstDoSnapshotRestore(t *testing.T, fix *dstFixture) {
	t.Helper()
	ctx := context.Background()

	// Reset and restore from existing snapshot.
	fix.world.reset()
	require.NoError(t, fix.world.restore(ctx))
}

// -------------------------------------------------------------------------------------------------
// DST Fixture
// -------------------------------------------------------------------------------------------------

type dstFixture struct {
	world *World
	nats  *dstFakeNATS
}

// allEntityIDs returns all entity IDs currently alive in the world using MatchAll search.
func (f *dstFixture) allEntityIDs() []EntityID {
	results, err := f.world.world.NewSearch(ecs.SearchParam{Match: ecs.MatchAll})
	if err != nil {
		return nil
	}
	ids := make([]EntityID, 0, len(results))
	for _, r := range results {
		if id, ok := r["_id"].(uint32); ok {
			ids = append(ids, EntityID(id))
		}
	}
	return ids
}

func newDSTFixture(t *testing.T, _ *rand.Rand, _ dstConfig) *dstFixture {
	t.Helper()

	// Suppress world logs during DST to reduce noise.
	t.Setenv("LOG_LEVEL", "disabled")

	// Step 1: Create the World via NewWorld with nop snapshot storage.
	debug := false
	w, err := NewWorld(WorldOptions{
		Region:              "dst",
		Organization:        "dst",
		Project:             "dst",
		ShardID:             "0",
		TickRate:            1,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        10,
		Debug:               &debug,
	})
	require.NoError(t, err)

	// Register shard systems (components, commands, events, etc. are auto-registered).
	dstRegisterShardSystems(w)

	// Replace NATS with fake that captures events and allows direct command injection.
	fakeNATS := newDSTFakeNATS(t, w)

	// Step 2: Replace nop snapshot storage with in-memory storage for DST.
	w.snapshotStorage = &memSnapshotStorage{t: t}

	// Initialize the ECS schedulers so Tick can run systems.
	w.world.Init()

	// Verify the world can tick without errors.
	require.NoError(t, w.Tick(context.Background(), time.Unix(0, 0)))

	return &dstFixture{
		world: w,
		nats:  fakeNATS,
	}
}

// -------------------------------------------------------------------------------------------------
// In-memory snapshot storage (DST-only)
// -------------------------------------------------------------------------------------------------

// memSnapshotStorage is an in-memory snapshot.Storage used exclusively for DST.
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

// -------------------------------------------------------------------------------------------------
// Fake NATS (DST-only)
// -------------------------------------------------------------------------------------------------

// dstFakeNATS replaces real NATS in DST. It enqueues commands directly into the world's command
// manager and captures events emitted by systems instead of publishing them over the network.
type dstFakeNATS struct {
	t         *testing.T
	world     *World
	pending   []CommandContext[Command] // Commands enqueued this tick (expected to be processed)
	events    []event.Event             // Default events captured during the last tick
	iscEvents []event.Event             // Inter-shard commands captured during the last tick
}

func newDSTFakeNATS(t *testing.T, world *World) *dstFakeNATS {
	f := &dstFakeNATS{
		t:         t,
		world:     world,
		pending:   make([]CommandContext[Command], 0),
		events:    make([]event.Event, 0),
		iscEvents: make([]event.Event, 0),
	}

	// Replace NATS event handlers with local capture handlers.
	world.events.RegisterHandler(event.KindDefault, f.handleDefaultEvent)
	world.events.RegisterHandler(event.KindInterShardCommand, f.handleInterShardCommand)

	return f
}

// enqueueCommand serializes a command payload and enqueues it into the world's command manager,
// bypassing NATS entirely. All commands are valid — boundary validation (protovalidate) is not
// exercised here because it belongs to the real NATS handler.
func (f *dstFakeNATS) enqueueCommand(cmd command.Payload, persona string) error {
	assert.NotEmpty(f.t, cmd.Name(), "nats: enqueueCommand called with empty command name")
	assert.NotEmpty(f.t, persona, "nats: enqueueCommand called with empty persona")

	data, err := schema.Serialize(cmd)
	if err != nil {
		return eris.Wrap(err, "failed to serialize command payload")
	}
	assert.NotEmpty(f.t, data, "nats: enqueueCommand serialized payload is empty")

	iscCmd := &iscv1.Command{
		Name:    cmd.Name(),
		Address: f.world.address,
		Persona: &iscv1.Persona{Id: persona},
		Payload: data,
	}
	err = f.world.commands.Enqueue(iscCmd)
	if err == nil {
		f.pending = append(f.pending, CommandContext[Command]{
			Payload: cmd,
			Persona: iscCmd.GetPersona().GetId(),
		})
	}
	return err
}

// clear resets captured events. Should be called before each tick.
func (f *dstFakeNATS) clear() {
	f.pending = f.pending[:0]
	f.events = f.events[:0]
	f.iscEvents = f.iscEvents[:0]
}

// handleDefaultEvent captures default events emitted by systems.
func (f *dstFakeNATS) handleDefaultEvent(evt event.Event) error {
	assert.Equal(f.t, event.KindDefault, evt.Kind, "nats: handleDefaultEvent received non-default event kind")
	assert.NotNil(f.t, evt.Payload, "nats: handleDefaultEvent received nil payload")
	f.events = append(f.events, evt)
	return nil
}

// handleInterShardCommand captures inter-shard commands emitted by systems.
func (f *dstFakeNATS) handleInterShardCommand(evt event.Event) error {
	assert.Equal(f.t, event.KindInterShardCommand, evt.Kind, "nats: handleInterShardCommand received wrong event kind")
	isc, ok := evt.Payload.(command.Command)
	if !assert.True(f.t, ok, "nats: handleInterShardCommand payload is %T, want command.Command", evt.Payload) {
		return eris.Errorf("invalid inter-shard command payload: %T", evt.Payload)
	}
	assert.NotEmpty(f.t, isc.Name, "nats: inter-shard command has empty name")
	assert.NotNil(f.t, isc.Address, "nats: inter-shard command has nil address")
	f.iscEvents = append(f.iscEvents, event.Event{Kind: event.KindInterShardCommand, Payload: isc.Payload})
	return nil
}
