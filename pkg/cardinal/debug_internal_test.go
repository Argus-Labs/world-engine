package cardinal

import (
	"context"
	"reflect"
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
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/performance"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

type introspectionSample struct{}

func (introspectionSample) Name() string                 { return "introspection-sample" }
func (introspectionSample) MarshalWire() ([]byte, error) { return nil, nil }
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

type snapshotEntities struct {
	Entities Contains[struct {
		Position  Ref[Position3D]
		Health    Ref[Health2]
		Inventory Ref[Inventory]
	}]
}

func seedSnapshotWorld(t *testing.T, state *snapshotEntities) {
	t.Helper()

	for i := range 5 {
		_, e := state.Entities.Create()
		e.Position.Set(Position3D{X: float64(i), Y: float64(i) * 2, Z: -1})
		e.Health.Set(Health2{Current: 100 - i, Max: 100})
		e.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10 + i})
	}
	doomed, e := state.Entities.Create()
	e.Position.Set(Position3D{X: 42})
	require.True(t, state.Entities.Destroy(doomed))
}

// TestDebugGetStatePublishesEveryTick checks snapshot content and ownership after each tick.
func TestDebugGetStatePublishesEveryTick(t *testing.T) {
	w, state := newDebugStateWorld(t)
	seedSnapshotWorld(t, state)

	for range 12 {
		resp, err := w.debug.GetState(
			context.Background(), connect.NewRequest(&cardinalv1.GetStateRequest{}),
		)
		require.NoError(t, err)
		held := resp.Msg.GetSnapshot()
		frozen, err := proto.MarshalOptions{Deterministic: true}.Marshal(held)
		require.NoError(t, err)

		_, e := state.Entities.Create()
		e.Position.Set(Position3D{X: float64(w.currentTick.height)})

		completed := w.currentTick.height
		w.Tick(time.Now())

		resp, err = w.debug.GetState(
			context.Background(), connect.NewRequest(&cardinalv1.GetStateRequest{}),
		)
		require.NoError(t, err)
		snap := resp.Msg.GetSnapshot()
		assert.Equal(t, completed, snap.GetTickHeight())
		assert.NotEmpty(t, snap.GetWorldState().GetArchetypes())

		after, err := proto.MarshalOptions{Deterministic: true}.Marshal(held)
		require.NoError(t, err)
		assert.Equal(t, frozen, after, "a tick changed a published snapshot")
	}
}

// TestDebugGetStateConcurrentWithTicks checks concurrent reads and writes. Run it with -race.
func TestDebugGetStateConcurrentWithTicks(t *testing.T) {
	w, state := newDebugStateWorld(t)
	seedSnapshotWorld(t, state)

	const readers = 4
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
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
		}()
	}

	for range 100 {
		w.Tick(time.Now())
	}
	close(stop)
	wg.Wait()

	assert.Equal(t, uint64(100), w.currentTick.height)
}

func newDebugStateWorld(t *testing.T) (*World, *snapshotEntities) {
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

	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(reflect.ValueOf(state).Elem(), w))
	w.world.Init()
	return w, state
}

func TestPerformanceBatchConverters(t *testing.T) {
	tickStart := time.Now()
	systemPhaseStartedAt := tickStart.Add(time.Millisecond)
	batch := performance.Batch{
		Ticks: []performance.TickTimeline{{
			TickHeight:           42,
			TickStart:            tickStart,
			SystemPhaseStartedAt: systemPhaseStartedAt,
			SystemPhaseElapsed:   3 * time.Millisecond,
			Profiled:             true,
			Spans: []performance.TickSpan{{
				SystemName: "move",
				StartTime:  systemPhaseStartedAt.Add(time.Millisecond),
				EndTime:    systemPhaseStartedAt.Add(2 * time.Millisecond),
			}},
		}},
	}

	overview := timingBatchToProto(batch)
	require.Len(t, overview.GetTicks(), 1)
	assert.Equal(t, uint64(3*time.Millisecond), overview.GetTicks()[0].GetDurationNs())

	profile := profileBatchToProto(batch)
	require.Len(t, profile.GetTicks(), 1)
	require.Len(t, profile.GetTicks()[0].GetSpans(), 1)
	assert.Equal(t, "move", profile.GetTicks()[0].GetSpans()[0].GetSystem())
	assert.Equal(t, uint64(time.Millisecond), profile.GetTicks()[0].GetSpans()[0].GetStartOffsetNs())
	assert.Equal(t, uint64(3*time.Millisecond), profile.GetTicks()[0].GetTiming().GetDurationNs())
}

func TestProfileBatchToProtoFiltersUnprofiledTicks(t *testing.T) {
	now := time.Now()
	batch := performance.Batch{Ticks: []performance.TickTimeline{
		{TickHeight: 1, TickStart: now},
		{TickHeight: 2, TickStart: now, SystemPhaseStartedAt: now, Profiled: true},
	}}

	overview := timingBatchToProto(batch)
	require.Len(t, overview.GetTicks(), 2, "timing subscribers keep the whole batch")

	profile := profileBatchToProto(batch)
	require.Len(t, profile.GetTicks(), 1)
	assert.Equal(t, uint64(2), profile.GetTicks()[0].GetTiming().GetTickHeight())
	assert.Empty(t, profile.GetTicks()[0].GetSpans(), "a profiled tick with no systems remains visible")
}
