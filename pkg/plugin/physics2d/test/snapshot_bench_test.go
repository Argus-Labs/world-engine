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

// snapshotBenchWarmupTicks is how far the scene is driven before any measurement, so that the
// timed loop only ever sees the steady state (see BenchmarkSnapshotTick). Per-tick cost was
// observed to stop drifting by tick ~400 at 5000 bodies; 600 leaves margin.
const snapshotBenchWarmupTicks = 600

// snapshotBenchWorld creates a production-shaped world: debug off, Nop snapshot storage, and the
// given snapshot rate. Rate 1_000_000 means "never snapshot" over a benchmark run. warmup is the
// number of ticks run before returning; pass snapshotBenchWarmupTicks whenever the world is about
// to be timed.
func snapshotBenchWorld(b *testing.B, rate uint32, bodies, warmup int) *cardinal.World {
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
	// Zero gravity, together with restingBodiesSystem's spacing, is what makes one tick equal to
	// the next: nothing accelerates, nothing collides, nothing changes state.
	cardinal.RegisterPlugin(w, physics.NewPlugin(physics.Config{
		Gravity:  physics.Vec2{},
		TickRate: 60,
	}))
	cardinal.RegisterSystem(w, restingBodiesSystem(bodies), cardinal.WithHook(cardinal.Init))
	initCardinalECS(w)
	benchTickN(w, warmup)
	return w
}

// restingBodiesSystem spawns a static floor plus count dynamic circles on tick 0, in the same grid
// and with the same components and shapes as BenchmarkStep's scene, but at rest: the circles carry
// GravityScale 0 in a zero-gravity world, and the grid pitch of 2.0 against a radius of 0.5 leaves
// a 1.0 gap, so no collider ever touches another. The entity count, archetype layout and component
// payloads — everything the snapshot path costs money on — are unchanged by that; what changes is
// that the scene stops evolving, which is what makes the benchmark reproducible.
func restingBodiesSystem(count int) func(state *struct {
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
			r.PB.Set(newRigidNoGravity(physics.BodyTypeDynamic, physics.ColliderShape{
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

// snapshotBenchEnvelope returns the snapshot envelope Cardinal hands to Storage.Store. Only the
// shape of the graph matters here — the envelope is built once, outside any timed loop — so the
// world is not warmed up.
func snapshotBenchEnvelope(b *testing.B, bodies int) *cardinalv1.Snapshot {
	b.Helper()
	w := snapshotBenchWorld(b, 1_000_000, bodies, 1)
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
//
// Comparability is the reason the scene is built the way it is. b.N ticks of the SAME world are
// timed, so unless every tick does the same work, ns/op is a function of b.N: a run at one
// -benchtime cannot be compared with a run at another, and neither can a before/after pair. An
// evolving physics scene does not give that. The scene this benchmark used to run — the bodies of
// BenchmarkStep, falling onto a floor and piling up — averaged (machine below, 1000 bodies, no
// snapshots) 759 us/tick over ticks 0-200, 1139 us over ticks 200-400, and then drifted down to
// 946 us by tick 3000: a 50% spread with nothing changed but how long the loop ran.
//
// So restingBodiesSystem builds a scene that cannot evolve, and snapshotBenchWarmupTicks drives it
// past the point where per-tick cost settles before the timer starts. Every timed tick then
// serializes the same world and steps the same physics. This deliberately is not a physics
// throughput benchmark — BenchmarkStep is, and it keeps the falling scene.
//
// Re-verify after touching the scene, the warm-up or the tick path, by running two -benchtime
// settings that differ by 4x and comparing ns/op column by column:
//
//	go test ./pkg/plugin/physics2d/test/ -run '^$' -bench 'BenchmarkSnapshotTick' -benchtime=300x
//	go test ./pkg/plugin/physics2d/test/ -run '^$' -bench 'BenchmarkSnapshotTick' -benchtime=1200x
//
// Measured 2026-08-02 on darwin/arm64, Apple M5 Max, go1.26.5. ns/op at 300x vs 1200x, worst case
// of the six sub-benchmarks 2.0% apart:
//
//	sub-benchmark              300x       1200x     delta
//	Bodies_1000/Rate_1000000    299.1 us   300.8 us  +0.6%
//	Bodies_1000/Rate_50         314.3 us   319.3 us  +1.6%
//	Bodies_1000/Rate_1         1069.4 us  1048.2 us  -2.0%
//	Bodies_5000/Rate_1000000   1503.5 us  1517.9 us  +1.0%
//	Bodies_5000/Rate_50        1606.7 us  1599.8 us  -0.4%
//	Bodies_5000/Rate_1         5047.3 us  5039.7 us  -0.2%
func BenchmarkSnapshotTick(b *testing.B) {
	for _, bodies := range []int{1000, 5000} {
		for _, rate := range []uint32{1_000_000, 50, 1} {
			b.Run(fmt.Sprintf("Bodies_%d/Rate_%d", bodies, rate), func(b *testing.B) {
				w := snapshotBenchWorld(b, rate, bodies, snapshotBenchWarmupTicks)
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					w.Tick(time.Unix(int64(100+i), 0))
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
//
// What the current path saves, measured 2026-08-02 on darwin/arm64, Apple M5 Max, go1.26.5:
// five runs at -benchtime=200x, per-run LegacyDoubleMarshal minus SingleMarshal, median taken
// across the five. Unlike a whole tick, each iteration here re-serializes one fixed envelope, so
// the loop is comparable at any -benchtime; the repeats are only against machine noise.
//
//	bodies  snapshot bytes  legacy (median)  single (median)  saved per snapshot
//	  1000         157_765         199.0 us          47.7 us    151 us (range 148-158)
//	  5000         877_089         999.4 us         222.1 us    777 us (range 742-820)
//
// Allocation deltas do not vary run to run: at 5000 bodies 20105 -> 1 allocations and
// 4_748_910 -> 884_737 B, i.e. 20104 allocations and 3.86 MB of garbage removed per snapshot.
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
