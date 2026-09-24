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
	w.RegisterPlugin(physics.NewPlugin(physics.Config{
		Gravity:  physics.Vec2{},
		TickRate: 60,
	}))
	w.RegisterSystem(restingBodiesSystem(bodies), cardinal.WithHook(cardinal.Init))
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
func restingBodiesSystem(count int) func(state *spawnState) {
	return func(state *spawnState) {
		if state.Tick() != 0 {
			return
		}
		floor := state.Spawn.Create()
		floor.Set(harnessTag{Role: "floor"})
		floor.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: -5}})
		floor.Set(physics.Velocity2D{})
		floor.Set(newRigid(physics.BodyTypeStatic,
			physics.Box(200, 1).Material(0.5, 0, 0).Filter(0xFFFF, 0xFFFF)))

		cols := int(math.Ceil(math.Sqrt(float64(count))))
		for i := range count {
			col := i % cols
			rowIdx := i / cols
			r := state.Spawn.Create()
			r.Set(harnessTag{Role: "ball"})
			r.Set(physics.Transform2D{Position: physics.Vec2{
				X: float64(col)*2.0 - float64(cols),
				Y: float64(rowIdx)*2.0 + 5.0,
			}})
			r.Set(physics.Velocity2D{})
			r.Set(newRigidNoGravity(physics.BodyTypeDynamic,
				physics.Circle(0.5).Material(0.3, 0.2, 1).Filter(0xFFFF, 0xFFFF)))
		}
	}
}

// worldStateProto reaches Cardinal's embedded *ecs.World and calls EncodeState, the same
// reflection escape hatch initCardinalECS uses (physics2d_test cannot import cardinal/internal/ecs),
// then decodes the bytes into the message the legacy benchmarks need.
func worldStateProto(b *testing.B, w *cardinal.World) *cardinalv1.WorldState {
	b.Helper()
	v := reflect.ValueOf(w).Elem()
	f := v.FieldByName("world")
	if !f.IsValid() {
		b.Fatal("cardinal.World: missing embedded ecs world field")
	}
	inner := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
	m := inner.MethodByName("EncodeState")
	if !m.IsValid() {
		b.Fatal("ecs.World: missing EncodeState method")
	}
	out := m.Call([]reflect.Value{reflect.ValueOf([]byte(nil))})
	data, ok := out[0].Interface().([]byte)
	if !ok {
		b.Fatalf("EncodeState returned %T, want []byte", out[0].Interface())
	}
	ws := &cardinalv1.WorldState{}
	if err := proto.Unmarshal(data, ws); err != nil {
		b.Fatalf("decode world state: %v", err)
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
// The table below was measured 2026-08-02 on darwin/arm64, Apple M5 Max, go1.26.5, when a body
// serialized a fat collider struct inline (every kind's fields, a fixed 8-vertex array). A body
// now serializes one compact Shape per collider, about a quarter smaller on the wire (see the
// byte counts on BenchmarkSnapshotStore). So read these as the record of the comparability
// check on that model, not as what a tick costs today. Anything measured now belongs beside
// numbers from the same machine and the same scene.
//
//	sub-benchmark              300x       1200x     delta
//	Bodies_1000/Rate_1000000    299.1 us   300.8 us  +0.6%
//	Bodies_1000/Rate_50         314.3 us   319.3 us  +1.6%
//	Bodies_1000/Rate_1         1069.4 us  1048.2 us  -2.0%
//	Bodies_5000/Rate_1000000   1503.5 us  1517.9 us  +1.0%
//	Bodies_5000/Rate_50        1606.7 us  1599.8 us  -0.4%
//	Bodies_5000/Rate_1         5047.3 us  5039.7 us  -0.2%
//
// Re-run on the shape-entity scene 2026-09-21 (linux/amd64, i9-11900K under WSL2, go1.27.1): the
// spread across those two -benchtime settings stayed within 10%, but two runs at the SAME 300x
// differed by 12% on Bodies_1000/Rate_1, so that box cannot resolve a 2% property. The check needs
// a quiet machine; it was not re-established here.
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
//	SingleMarshal       — one marshal of the whole envelope, the intermediate step: the backend was
//	                      handed a message and serialized it exactly once.
//	Nop                 — the default storage type, handed bytes and doing nothing with them.
//
// None of the three is the current engine path. Storage now takes []byte and marshals nothing at
// all; the one serialization left is cardinal's own hand-rolled snapshot.Encode, which never builds
// a proto graph and so is not proto.Marshal of anything. These rows measure what a BACKEND does
// with what it is given, which is why SingleMarshal is the ceiling the byte interface removed
// rather than a description of today. For the engine-side cost see BenchmarkSnapshotTick.
//
// What the current path saves. Unlike a whole tick, each iteration here re-serializes one fixed
// envelope, so the loop is comparable at any -benchtime; repeats are only against machine noise.
//
// Snapshot bytes and allocations are properties of the data model, not the machine, so they are
// current (measured 2026-09-21 at -benchtime=200x):
//
//	bodies  snapshot bytes  legacy allocs  single allocs  garbage removed per snapshot
//	  1000          78_007           9058              1                       453 KB
//	  5000         390_286          45063              1                      2.31 MB
//
// The shape encoding changed between measurements: the inline fat collider gave 157_765 bytes at
// 1000 bodies and 877_089 at 5000, shapes as shared entities gave 78_007 and 390_286, and the
// compact inline Shape gives 117_838 and 590_117 (linux/amd64, 2026-09-23).
//
// The microsecond savings that used to sit here (151 us at 1000 bodies, 777 us at 5000, measured
// 2026-08-02 on darwin/arm64, Apple M5 Max, go1.26.5) were taken on the old model and against the
// old byte counts, so they are not comparable with a run today. Re-measure them on a quiet machine
// before quoting a saving.
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
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(snap)
		if err != nil {
			b.Fatal(err)
		}
		snapshotBytes := float64(len(encoded))

		nop := snapshot.NewNopStorage()
		stores := []struct {
			name  string
			store func(context.Context, *cardinalv1.Snapshot) error
		}{
			// Handed bytes the engine already encoded, exactly as production does, so this row is
			// the free baseline it claims to be. Marshaling here instead would just re-measure
			// SingleMarshal, and with proto.Marshal — which is not how the engine encodes.
			{"Nop", func(ctx context.Context, s *cardinalv1.Snapshot) error {
				return nop.Store(ctx, s.GetTickHeight(), encoded)
			}},
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
