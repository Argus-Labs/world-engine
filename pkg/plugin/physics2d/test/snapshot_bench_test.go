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

// snapshotBenchEnvelope returns the snapshot envelope Cardinal hands to Storage.Store.
func snapshotBenchEnvelope(b *testing.B, bodies int) *cardinalv1.Snapshot {
	b.Helper()
	w := snapshotBenchWorld(b, 1_000_000, bodies)
	return &cardinalv1.Snapshot{
		TickHeight: 1,
		Timestamp:  timestamppb.New(time.Unix(0, 0)),
		WorldState: worldStateProto(b, w),
		Version:    snapshot.CurrentVersion,
	}
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

// Each sub-benchmark measures every serialization step a snapshot tick performs before any network
// I/O, so the shapes are directly comparable:
//
//	LegacyDoubleMarshal — the pre-optimization path: cardinal marshals the WorldState, the backend
//	                      unmarshals it, rebuilds the envelope and marshals it again.
//	SingleMarshal       — the current path: cardinal hands the envelope over, the backend marshals
//	                      it exactly once (what JetStreamStorage and S3Storage now do).
//	Nop                 — the default storage type; with no caller-side marshal it is free.
type legacyDoubleMarshalStorage struct {
	sink []byte
}

func (d *legacyDoubleMarshalStorage) Store(_ context.Context, snap *cardinalv1.Snapshot) error {
	// The caller used to marshal the world state and pass the bytes down.
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(snap.GetWorldState())
	if err != nil {
		return err
	}
	// The backend then threw those bytes away and rebuilt the envelope from scratch.
	var worldState cardinalv1.WorldState
	if err := proto.Unmarshal(data, &worldState); err != nil {
		return err
	}
	out, err := proto.Marshal(&cardinalv1.Snapshot{
		TickHeight: snap.GetTickHeight(),
		Timestamp:  snap.GetTimestamp(),
		WorldState: &worldState,
		Version:    snap.GetVersion(),
	})
	if err != nil {
		return err
	}
	d.sink = out
	return nil
}

// singleMarshalStorage reproduces what JetStreamStorage.Store and S3Storage.Store do before they
// touch the network: marshal the envelope once.
type singleMarshalStorage struct {
	sink []byte
}

func (s *singleMarshalStorage) Store(_ context.Context, snap *cardinalv1.Snapshot) error {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(snap)
	if err != nil {
		return err
	}
	s.sink = data
	return nil
}

func BenchmarkSnapshotStore(b *testing.B) {
	ctx := context.Background()
	for _, bodies := range []int{1000, 5000} {
		snap := snapshotBenchEnvelope(b, bodies)
		size, err := proto.MarshalOptions{Deterministic: true}.Marshal(snap)
		if err != nil {
			b.Fatal(err)
		}
		snapshotBytes := float64(len(size))

		stores := []struct {
			name  string
			store func(context.Context, *cardinalv1.Snapshot) error
		}{
			{"Nop", snapshot.NewNopStorage().Store},
			{"SingleMarshal", (&singleMarshalStorage{}).Store},
			{"LegacyDoubleMarshal", (&legacyDoubleMarshalStorage{}).Store},
		}
		for _, tc := range stores {
			b.Run(fmt.Sprintf("Bodies_%d/%s", bodies, tc.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if err := tc.store(ctx, snap); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(snapshotBytes, "snapshot_bytes")
			})
		}
	}
}
