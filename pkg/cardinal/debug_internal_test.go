package cardinal

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

type introspectionSample struct{}

func (introspectionSample) Name() string                 { return "introspection-sample" }
func (c introspectionSample) SizeWire() int              { return len(c.MarshalWire()) }
func (c introspectionSample) AppendWire(b []byte) []byte { return append(b, c.MarshalWire()...) }

func (introspectionSample) MarshalWire() []byte { return nil }
func (introspectionSample) ProtoDescriptor() protoreflect.MessageDescriptor {
	return (&cardinalv1.TypeSchema{}).ProtoReflect().Descriptor()
}
func (introspectionSample) UnmarshalWire([]byte) (any, error) {
	return introspectionSample{}, nil
}

func newIntrospectionTestModule() *debugModule {
	return &debugModule{
		world:   &World{world: ecs.NewWorld()},
		catalog: introspect.NewCatalog(),
	}
}

func TestIntrospectAdvertisesSharedProtobufMetadata(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	for _, kind := range []introspect.Kind{introspect.Command, introspect.Component, introspect.Event} {
		require.NoError(t, d.register(kind, introspectionSample{}))
	}
	require.NoError(t, d.finalizeCatalog())

	response, err := d.Introspect(context.Background(), (*connect.Request[cardinalv1.IntrospectRequest])(nil))
	require.NoError(t, err)

	for _, schemas := range [][]*cardinalv1.TypeSchema{
		response.Msg.GetCommands(),
		response.Msg.GetComponents(),
		response.Msg.GetEvents(),
	} {
		require.Len(t, schemas, 1)
		schema := schemas[0]
		assert.Equal(
			t,
			introspectionSample{}.ProtoDescriptor().FullName(),
			protoreflect.FullName(schema.GetProtoMessageName()),
		)
	}

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(response.Msg.GetProtoDescriptorSet(), &set))
	require.NotNil(t, findMessageDescriptor(&set, "TypeSchema"))
}

func TestDebugRegistrationIsNilSafe(t *testing.T) {
	t.Parallel()

	var d *debugModule
	require.NoError(t, d.register(introspect.Command, introspectionSample{}))
}

func findMessageDescriptor(set *descriptorpb.FileDescriptorSet, name string) *descriptorpb.DescriptorProto {
	for _, file := range set.GetFile() {
		for _, message := range file.GetMessageType() {
			if message.GetName() == name {
				return message
			}
		}
	}
	return nil
}

type snapshotArchetype struct {
	Position  Position3D
	Health    Health2
	Inventory Inventory
}

func seedSnapshotWorld(t *testing.T, w *World) {
	t.Helper()

	for i := range 5 {
		e := w.Create[snapshotArchetype]()
		e.Set(Position3D{X: float64(i), Y: float64(i) * 2, Z: -1})
		e.Set(Health2{Current: 100 - i, Max: 100})
		e.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10 + i})
	}
	e := w.Create[snapshotArchetype]()
	e.Set(Position3D{X: 42})
	require.True(t, e.Destroy())
}

// TestDebugGetStatePublishesEveryTick checks snapshot content and ownership after each tick.
func TestDebugGetStatePublishesEveryTick(t *testing.T) {
	w := newDebugStateWorld(t)
	seedSnapshotWorld(t, w)

	for range 12 {
		resp, err := w.debug.GetState(
			context.Background(), connect.NewRequest(&cardinalv1.GetStateRequest{}),
		)
		require.NoError(t, err)
		held := resp.Msg.GetSnapshot()
		frozen, err := proto.MarshalOptions{Deterministic: true}.Marshal(held)
		require.NoError(t, err)

		e := w.Create[snapshotArchetype]()
		e.Set(Position3D{X: float64(w.currentTick.height)})

		completed := w.currentTick.height
		w.Tick(time.Now())

		resp, err = w.debug.GetState(
			context.Background(), connect.NewRequest(&cardinalv1.GetStateRequest{}),
		)
		require.NoError(t, err)
		snap := resp.Msg.GetSnapshot()
		assert.Equal(t, completed, snap.GetTickHeight())
		assert.NotEmpty(t, snap.GetWorldState().GetEntities())

		after, err := proto.MarshalOptions{Deterministic: true}.Marshal(held)
		require.NoError(t, err)
		assert.Equal(t, frozen, after, "a tick changed a published snapshot")
	}
}

// TestDebugGetStateConcurrentWithTicks checks concurrent reads and writes. Run it with -race.
func TestDebugGetStateConcurrentWithTicks(t *testing.T) {
	w := newDebugStateWorld(t)
	seedSnapshotWorld(t, w)

	const readers = 4
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range readers {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp, err := w.debug.GetState(
					context.Background(), connect.NewRequest(&cardinalv1.GetStateRequest{}),
				)
				if err != nil {
					t.Errorf("GetState failed: %v", err)
					return
				}
				snap := resp.Msg.GetSnapshot()
				if _, err := proto.Marshal(snap); err != nil {
					t.Errorf("failed to serialize the published snapshot: %v", err)
					return
				}
			}
		})
	}

	for range 100 {
		w.Tick(time.Now())
	}
	close(stop)
	wg.Wait()

	assert.Equal(t, uint64(100), w.currentTick.height)
}

func newDebugStateWorld(t *testing.T) *World {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "debug-state",
		Organization:        "debug-state",
		Project:             "debug-state",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)
	w.RegisterComponent[Position3D]()
	w.RegisterComponent[Health2]()
	w.RegisterComponent[Inventory]()
	w.world.Init()
	return w
}

// TestResetClearsSystemEventsThroughShippedPath verifies that the shipped cardinal
// reset() path — the function the resetChan branch in run() dispatches to when debug
// mode is enabled — does not leak init-emitted system events from a prior Init into
// the first post-reset Tick. The receiver runs inside the tick before Tick's deferred
// clear, so it would observe both the stale and fresh batches without the ecs-layer
// Reset clearing the buffer.
func TestResetClearsSystemEventsThroughShippedPath(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "reset-events",
		Organization:        "reset-events",
		Project:             "reset-events",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)

	_, err = w.world.RegisterSystemEvent[testutils.SimpleSystemEvent]()
	require.NoError(t, err)

	var observed []testutils.SimpleSystemEvent
	const emitVal = 7

	require.NoError(t, w.world.RegisterSystem("emit-on-init", ecs.Init, func() {
		require.NoError(t, w.world.EmitSystemEvent(testutils.SimpleSystemEvent{Value: emitVal}))
	}))
	require.NoError(t, w.world.RegisterSystem("receive-on-update", ecs.Update, func() {
		evs, err := w.world.GetSystemEvents[testutils.SimpleSystemEvent]()
		require.NoError(t, err)
		observed = append(observed, evs...)
	}))

	w.world.Init()          // first Init: emits one event
	w.reset()               // shipped reset: Reset()+Init() emits again
	w.Tick(time.Unix(0, 0)) // first post-reset tick: receiver runs

	require.Len(t, observed, 1,
		"receiver should see only the fresh post-reset event; got stale+fresh: %v", observed)
	require.Equal(t, testutils.SimpleSystemEvent{Value: emitVal}, observed[0])
}

// broadcastOnInitSystem is an Init-hook system that broadcasts a client-facing event. The
// cardinal Broadcast/SendTo API has no guard against use inside an Init system, so a game may
// emit a "world initialized" event here that clients expect on the first tick.
type broadcastOnInitSystem struct {
	value int
}

func (s *broadcastOnInitSystem) Run(w *World) {
	w.Broadcast(testutils.SimpleEvent{Value: s.value})
}

// TestResetPreservesInitEmittedClientEvents verifies that the shipped cardinal reset() path
// preserves init-emitted client-facing events for delivery on the next tick, matching
// cold-start behavior. It is the client-event (event.Manager) analogue of
// TestResetClearsSystemEventsThroughShippedPath (which covers ECS system events). reset() must
// clear stale pre-reset events before re-running init, so the next tick delivers exactly the
// fresh init-emitted event — neither the stale one from the first Init nor zero (the bug, where
// w.events.Clear() ran after w.init() and dropped the fresh event).
func TestResetPreservesInitEmittedClientEvents(t *testing.T) {
	t.Setenv("LOG_LEVEL", "disabled")

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "reset-client-events",
		Organization:        "reset-client-events",
		Project:             "reset-client-events",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug)

	w.RegisterEvent[testutils.SimpleEvent]()

	const emitVal = 42
	require.NotPanics(t, func() {
		w.RegisterSystem(&broadcastOnInitSystem{value: emitVal}, WithHook(Init))
	})

	var observed []testutils.SimpleEvent
	w.events.RegisterHandler(event.KindDefault, func(_ context.Context, e event.Event) error {
		if payload, ok := e.Payload.(testutils.SimpleEvent); ok {
			observed = append(observed, payload)
		}
		return nil
	})

	// First Init emits one event into the event manager channel (the "stale" pre-reset event).
	w.world.Init()

	// Shipped reset: Reset() -> Clear() -> init(). Init re-emits; the clears ran before init so
	// the fresh event survives and the stale one does not.
	w.reset()

	// First post-reset tick: dispatchEvents delivers only the surviving fresh event.
	w.Tick(time.Unix(0, 0))

	require.Len(t, observed, 1,
		"init-emitted client event should survive reset and be delivered on the next tick, "+
			"matching cold-start behavior; got %v", observed)
	require.Equal(t, emitVal, observed[0].Value)
}
