package physics2d_test

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Benchmarks for Cardinal's snapshot path, measured through a real physics scene.
//
// These live in physics2d_test rather than pkg/cardinal because pkg/cardinal cannot import a plugin
// (import cycle), and the plugin's components are the only in-repo components with generated proto
// codecs. Cardinal's own benchmarks use gob-encoded doubles, which do not represent the production
// serialization cost.
//
// The matrix deliberately runs with Debug off. Debug on forces the full ToProto graph build on every
// tick regardless of SnapshotRate (cardinal.go persistState), which is not how production runs and
// makes per-tick numbers unusable as a snapshot-path baseline.

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// snapshotBenchWorld creates a production-shaped world: debug off, Nop snapshot storage, and the
// given snapshot rate. Rate 1_000_000 means "never snapshot" over a benchmark run.
func snapshotBenchWorld(b *testing.B, rate uint32, bodies int) *cardinal.World {
	b.Helper()
	b.Setenv("LOG_LEVEL", "disabled")
	debug := false
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region:              "local",
		Organization:        "bench",
		Project:             "bench",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        rate,
		Debug:               &debug,
	})
	if err != nil {
		b.Fatal(err)
	}
	cardinal.RegisterPlugin(w, physics.NewPlugin(physics.Config{
		Gravity:  physics.Vec2{X: 0, Y: -10},
		TickRate: 60,
	}))
	cardinal.RegisterSystem(w, fallingBodiesSystem(bodies), cardinal.WithHook(cardinal.Init))
	initCardinalECS(w)
	benchTickN(w, 10) // settle
	return w
}

// fallingBodiesSystem spawns a static floor plus count dynamic circles on tick 0, mirroring
// BenchmarkStep's scene so snapshot numbers are comparable to the physics throughput numbers.
func fallingBodiesSystem(count int) func(state *struct {
	cardinal.BaseSystemState
	Spawn spawnArchetype
}) {
	return func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		_, floor := state.Spawn.Create()
		floor.Tag.Set(harnessTag{Role: "floor"})
		floor.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: -5}})
		floor.V.Set(physics.Velocity2D{})
		floor.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 200, Y: 1},
			Friction:     0.5,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))

		cols := int(math.Ceil(math.Sqrt(float64(count))))
		for i := range count {
			col := i % cols
			rowIdx := i / cols
			_, r := state.Spawn.Create()
			r.Tag.Set(harnessTag{Role: "ball"})
			r.T.Set(physics.Transform2D{Position: physics.Vec2{
				X: float64(col)*2.0 - float64(cols),
				Y: float64(rowIdx)*2.0 + 5.0,
			}})
			r.V.Set(physics.Velocity2D{})
			r.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
				ShapeType:    physics.ShapeTypeCircle,
				Radius:       0.5,
				Density:      1,
				Friction:     0.3,
				Restitution:  0.2,
				CategoryBits: 0xFFFF,
				MaskBits:     0xFFFF,
			}))
		}
	}
}

// worldStateProto reaches Cardinal's embedded *ecs.World and calls ToProto, the same reflection
// escape hatch initCardinalECS uses (physics2d_test cannot import cardinal/internal/ecs).
func worldStateProto(b *testing.B, w *cardinal.World) *cardinalv1.WorldState {
	b.Helper()
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	if !f.IsValid() {
		b.Fatal("cardinal.World: missing embedded ecs world field")
	}
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("ToProto")
	if !m.IsValid() {
		b.Fatal("ecs.World: missing ToProto method")
	}
	out := m.Call(nil)
	if !out[1].IsNil() {
		b.Fatalf("ToProto: %v", out[1].Interface())
	}
	ws, ok := out[0].Interface().(*cardinalv1.WorldState)
	if !ok {
		b.Fatalf("ToProto returned %T, want *cardinalv1.WorldState", out[0].Interface())
	}
	return ws
}

// snapshotBenchBytes returns the marshaled world state Cardinal hands to Storage.Store.
func snapshotBenchBytes(b *testing.B, bodies int) []byte {
	b.Helper()
	w := snapshotBenchWorld(b, 1_000_000, bodies)
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(worldStateProto(b, w))
	if err != nil {
		b.Fatal(err)
	}
	return data
}

// ---------------------------------------------------------------------------
// BenchmarkSnapshotTick — whole-tick cost across the production configuration matrix.
// ---------------------------------------------------------------------------

// Rate 1_000_000 never snapshots (physics-only floor). Rate 50 is what every template ships. Rate 1
// snapshots every tick, isolating the snapshot tick itself — the difference against rate 1_000_000
// is the cost this optimization work targets.
func BenchmarkSnapshotTick(b *testing.B) {
	for _, bodies := range []int{1000, 5000} {
		for _, rate := range []uint32{1_000_000, 50, 1} {
			b.Run(fmt.Sprintf("Bodies_%d/Rate_%d", bodies, rate), func(b *testing.B) {
				w := snapshotBenchWorld(b, rate, bodies)
				ctx := context.Background()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					w.Tick(ctx, time.Unix(int64(100+i), 0))
				}
			})
		}
	}
}

// ---------------------------------------------------------------------------
// BenchmarkSnapshotStore — the storage write path, network excluded.
// ---------------------------------------------------------------------------

// doubleMarshalStorage reproduces exactly what JetStreamStorage.Store and S3Storage.Store do before
// they touch the network: unmarshal the bytes Cardinal just marshaled, rebuild the envelope, marshal
// again. It exists so that redundant work is measurable without a NATS or S3 backend.
type doubleMarshalStorage struct {
	sink []byte
}

var _ snapshot.Storage = (*doubleMarshalStorage)(nil)

func (d *doubleMarshalStorage) Store(_ context.Context, snap *snapshot.Snapshot) error {
	var worldState cardinalv1.WorldState
	if err := proto.Unmarshal(snap.Data, &worldState); err != nil {
		return err
	}
	data, err := proto.Marshal(&cardinalv1.Snapshot{
		TickHeight: snap.TickHeight,
		Timestamp:  timestamppb.New(snap.Timestamp),
		WorldState: &worldState,
		Version:    snap.Version,
	})
	if err != nil {
		return err
	}
	d.sink = data
	return nil
}

func (d *doubleMarshalStorage) Load(_ context.Context) (*snapshot.Snapshot, error) {
	return nil, snapshot.ErrSnapshotNotFound
}

func BenchmarkSnapshotStore(b *testing.B) {
	ctx := context.Background()
	for _, bodies := range []int{1000, 5000} {
		data := snapshotBenchBytes(b, bodies)
		snap := &snapshot.Snapshot{
			TickHeight: 1,
			Timestamp:  time.Unix(0, 0),
			Data:       data,
			Version:    snapshot.CurrentVersion,
		}

		b.Run(fmt.Sprintf("Bodies_%d/Nop", bodies), func(b *testing.B) {
			store := snapshot.NewNopStorage()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := store.Store(ctx, snap); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(data)), "snapshot_bytes")
		})

		b.Run(fmt.Sprintf("Bodies_%d/DoubleMarshal", bodies), func(b *testing.B) {
			store := &doubleMarshalStorage{}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := store.Store(ctx, snap); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(data)), "snapshot_bytes")
		})
	}
}
