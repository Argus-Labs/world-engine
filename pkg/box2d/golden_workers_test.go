// Worker-count determinism matrix for the internal worker pool
// (worker_pool.go). Every golden suite that steps worlds is re-run with
// WorldDef.WorkerCount in {2, 4, 8} and asserted against the SAME testdata
// files the serial suites committed — no per-worker-count golden files exist,
// and the regen path (BOX2D_UPDATE_GOLDEN) writes from the serial world only
// (see TestGoldenStep and friends). A mismatch here means the pool broke the
// byte-identical-for-every-worker-count guarantee, never that the goldens
// need regenerating.
//
// Run with -race to prove the parallel Step dispatches (broad-phase pair
// find, collide, solver body and color stages, finalize, sensors) are
// race-clean: the pool's channel/WaitGroup edges are exactly the
// happens-before graph the detector verifies.
//
// NewWorld does not clamp the worker count to runtime.GOMAXPROCS(0), so the
// w=8 rows are true 8-way partitions (with 7 live pool workers) even on
// machines with fewer cores — each row genuinely exercises the partition
// width it names.

package box2d_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// goldenWorkerCounts is the worker-count matrix every golden suite re-runs
// under. 1 is covered by the serial suites themselves.
var goldenWorkerCounts = []int{2, 4, 8}

// runGoldenWorkerMatrix loads the committed golden file and requires the
// suite recomputed at each matrix worker count to reproduce it exactly.
func runGoldenWorkerMatrix(t *testing.T, filename string, compute func(workerCount int) []goldenStepScene) {
	t.Helper()

	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "golden file missing — the serial suite owns its creation (BOX2D_UPDATE_GOLDEN)")

	var want []goldenStepScene
	require.NoError(t, json.Unmarshal(data, &want))

	for _, workerCount := range goldenWorkerCounts {
		t.Run(fmt.Sprintf("workers=%d", workerCount), func(t *testing.T) {
			got := compute(workerCount)

			require.Len(t, got, len(want), "scene count")
			for i := range want {
				require.Equal(t, want[i].Name, got[i].Name)
				require.Equal(t, want[i].Hashes, got[i].Hashes,
					"scene %s at WorkerCount=%d diverges from the serial golden %s — worker-pool determinism broken",
					want[i].Name, workerCount, filename)
			}
		})
	}
}

func TestGoldenStepWorkers(t *testing.T) {
	runGoldenWorkerMatrix(t, "golden_step.json", computeGoldenStepWorkers)
}

func TestGoldenContinuousWorkers(t *testing.T) {
	runGoldenWorkerMatrix(t, "golden_continuous.json", computeGoldenContinuousWorkers)
}

func TestGoldenJointWorkers(t *testing.T) {
	runGoldenWorkerMatrix(t, "golden_joint.json", computeGoldenJointWorkers)
}

func TestGoldenJoint2Workers(t *testing.T) {
	runGoldenWorkerMatrix(t, "golden_joint2.json", computeGoldenJoint2Workers)
}

func TestGoldenJoint3Workers(t *testing.T) {
	runGoldenWorkerMatrix(t, "golden_joint3.json", computeGoldenJoint3Workers)
}

// buildWorkerStressScene builds a scene big enough to push EVERY dispatch in
// World.Step past its inline grain threshold at 8 workers, so the parallel
// code paths genuinely execute (the golden scenes are small and mostly run
// inline): a 20-row pyramid (210 boxes — awake bodies > the body grain,
// hundreds of contacts across the graph colors), 80 independent pendulums
// (80 body-disjoint revolute joints landing in one color, with force
// thresholds so the joint-event bookkeeping runs), 40 sensors starting inside
// the pyramid (40/sensorMinRange 16 = 2 engaged workers under capped
// engagement), and 6 bullets fired through the sensors (bullet gather +
// serial bullet stage + continuous sensor hits).
func buildWorkerStressScene(w *box2d.World) []box2d.BodyID {
	return buildWorkerScene(w, 20, 80, 40, 6)
}

// buildWorkerScene is the shared parallel-dispatch scene builder used by the
// worker stress tests and, at a smaller size, as the fuzz harness base scene
// (fuzz_ops_test.go): a rows-row box pyramid on a wide ground, pendulumCount
// body-disjoint revolute pendulums, sensorCount static sensors and
// bulletCount bullets fired through them. It returns EVERY body it creates
// (statics included) so callers can hash the full scene or feed the ids to
// the op interpreter's tally.
//
// The pyramid boxes and bullets enable pre-solve events and custom filtering
// so worlds that install those callbacks (installWorkerStressCallbacks)
// invoke them from the parallel collide and pair-find dispatches; with no
// callback installed the flags are behaviorally inert, so flag-on with no
// callback is identical to the pre-flag scene.
func buildWorkerScene(w *box2d.World, rows, pendulumCount, sensorCount, bulletCount int) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(120.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)
	bodies = append(bodies, ground)

	// Pyramid (same construction as buildPyramidScene, more rows).
	h := 0.5
	for i := range rows {
		y := float64((2.0*float64(i)+1.0)*h) + float64(0.02*float64(i))
		for j := i; j < rows; j++ {
			x := float64(float64(i)*h) + float64(float64(2.0*h)*float64(j-i)) - float64(h*float64(rows-i-1))

			bd := box2d.DefaultBodyDef()
			bd.Type = box2d.DynamicBody
			bd.Position = box2d.Vec2{X: x, Y: y}
			body := w.CreateBody(&bd)

			box := box2d.MakeBox(h, h)
			sd := box2d.DefaultShapeDef()
			sd.Density = 1.0
			sd.EnableSensorEvents = true
			sd.EnablePreSolveEvents = true
			sd.EnableCustomFiltering = true
			w.CreatePolygonShape(body, &sd, &box)
			bodies = append(bodies, body)
		}
	}

	// Independent pendulums: disjoint bodies, so the joints share one graph
	// color and exceed the per-color grain.
	for i := range pendulumCount {
		fi := float64(i)
		anchorPos := box2d.Vec2{X: -60.0 + float64(1.5*fi), Y: 30.0}

		ad := box2d.DefaultBodyDef()
		ad.Position = anchorPos
		anchor := w.CreateBody(&ad)
		bodies = append(bodies, anchor)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: anchorPos.X + 1.0, Y: anchorPos.Y}
		bob := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(bob, &sd, &circle)
		bodies = append(bodies, bob)

		jd := box2d.DefaultRevoluteJointDef()
		jd.Base.BodyIDA = anchor
		jd.Base.BodyIDB = bob
		jd.Base.LocalFrameB.P = box2d.Vec2{X: 1.0, Y: 0.0}
		// Low threshold so the joint-event bit set path runs.
		jd.Base.ForceThreshold = 1.0
		w.CreateRevoluteJoint(&jd)
	}

	// Static sensors inside and around the pyramid.
	for i := range sensorCount {
		fi := float64(i)
		sbd := box2d.DefaultBodyDef()
		sbd.Position = box2d.Vec2{X: -9.5 + float64(0.5*fi), Y: 0.5 + float64(0.2*fi)}
		sensorBody := w.CreateBody(&sbd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.6}
		sd := box2d.DefaultShapeDef()
		sd.IsSensor = true
		sd.EnableSensorEvents = true
		sd.EnableCustomFiltering = true
		w.CreateCircleShape(sensorBody, &sd, &circle)
		bodies = append(bodies, sensorBody)
	}

	// Bullets fired through the sensors and into the pyramid.
	for i := range bulletCount {
		fi := float64(i)
		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.IsBullet = true
		bd.Position = box2d.Vec2{X: -40.0, Y: 0.6 + float64(0.8*fi)}
		bd.LinearVelocity = box2d.Vec2{X: 90.0, Y: 0.0}
		bullet := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.15}
		sd := box2d.DefaultShapeDef()
		sd.Density = 4.0
		sd.EnableSensorEvents = true
		sd.EnablePreSolveEvents = true
		sd.EnableCustomFiltering = true
		w.CreateCircleShape(bullet, &sd, &circle)
		bodies = append(bodies, bullet)
	}

	return bodies
}

// installWorkerStressCallbacks installs every user callback the doc.go
// concurrency contract names, on any world, in a form that cannot change
// results:
//
//   - preSolve: pure read of the manifold sample, returns true (keep contact).
//   - custom filter: pure, returns true (keep pair).
//   - friction/restitution mixing: replicate defaultFrictionCallback and
//     defaultRestitutionCallback (world.go) EXPRESSION FOR EXPRESSION —
//     sqrt(a*b) and max(a,b) — so the mixed values are bit-identical to the
//     defaults. Neither expression contains an add, so no FMA contraction is
//     possible and the replication is exact on every architecture.
//
// A signature comparison between a callbacks-on world and a callbacks-off
// world therefore proves callbacks-on == callbacks-off, and running the
// parallel row under -race proves the concurrent-invocation contract
// (callbacks are called from pool workers, possibly simultaneously).
func installWorkerStressCallbacks(w *box2d.World) {
	w.SetPreSolveCallback(func(shapeIDA, shapeIDB box2d.ShapeID, point, normal box2d.Vec2, ctx any) bool {
		// Pure, thread-safe read of the callback inputs; no shared state.
		_ = shapeIDA
		_ = shapeIDB
		_ = point
		_ = normal
		return true
	}, nil)

	w.SetCustomFilterCallback(func(shapeIDA, shapeIDB box2d.ShapeID, ctx any) bool {
		_ = shapeIDA
		_ = shapeIDB
		return true
	}, nil)

	w.SetFrictionCallback(func(frictionA float64, materialA uint64, frictionB float64, materialB uint64) float64 {
		// Exact replica of defaultFrictionCallback (upstream b2DefaultFrictionCallback).
		return math.Sqrt(frictionA * frictionB)
	})

	w.SetRestitutionCallback(func(restitutionA float64, materialA uint64, restitutionB float64, materialB uint64) float64 {
		// Exact replica of defaultRestitutionCallback (upstream b2DefaultRestitutionCallback).
		if restitutionA > restitutionB {
			return restitutionA
		}
		return restitutionB
	})
}

// foldWorkerEventStreams folds the FULL CONTENTS of every event stream the
// world exposes — body move, contact begin/end/hit, sensor begin/end, joint —
// into the signature hash, field by field, so any cross-worker-count event
// divergence (content OR order, not just count) fails loudly. Only existing
// values are hashed (float bit patterns via math.Float64bits); no arithmetic
// is performed on them. Ids are folded with the world0 owner token masked out
// (worldTokenMask, world_identity_test.go) because the serial and parallel
// signatures come from two distinct worlds with distinct tokens.
func foldWorkerEventStreams(hash uint64, w *box2d.World) uint64 {
	foldShapeID := func(id box2d.ShapeID) {
		hash = djb2Fold(hash, box2d.PackShapeID(id)&^worldTokenMask)
	}
	foldContactID := func(id box2d.ContactID) {
		packed := box2d.PackContactID(id)
		hash = djb2Fold(hash, uint64(packed[0])) // index1
		hash = djb2Fold(hash, uint64(packed[2])) // generation; packed[1] is the world0 token
	}
	foldFloat := func(v float64) {
		hash = djb2Fold(hash, math.Float64bits(v))
	}
	foldBool := func(b bool) {
		if b {
			hash = djb2Fold(hash, 1)
		} else {
			hash = djb2Fold(hash, 0)
		}
	}

	moveEvents := w.BodyEvents().MoveEvents
	hash = djb2Fold(hash, uint64(len(moveEvents)))
	for i := range moveEvents {
		e := &moveEvents[i]
		hash = djb2Fold(hash, e.UserData)
		hash = djb2Fold(hash, box2d.PackBodyID(e.BodyID)&^worldTokenMask)
		foldFloat(e.Transform.P.X)
		foldFloat(e.Transform.P.Y)
		foldFloat(e.Transform.Q.C)
		foldFloat(e.Transform.Q.S)
		foldBool(e.FellAsleep)
	}

	contactEvents := w.ContactEvents()
	hash = djb2Fold(hash, uint64(len(contactEvents.BeginEvents)))
	for i := range contactEvents.BeginEvents {
		e := &contactEvents.BeginEvents[i]
		foldShapeID(e.ShapeIDA)
		foldShapeID(e.ShapeIDB)
		foldContactID(e.ContactID)
	}
	hash = djb2Fold(hash, uint64(len(contactEvents.EndEvents)))
	for i := range contactEvents.EndEvents {
		e := &contactEvents.EndEvents[i]
		foldShapeID(e.ShapeIDA)
		foldShapeID(e.ShapeIDB)
		foldContactID(e.ContactID)
	}
	hash = djb2Fold(hash, uint64(len(contactEvents.HitEvents)))
	for i := range contactEvents.HitEvents {
		e := &contactEvents.HitEvents[i]
		foldShapeID(e.ShapeIDA)
		foldShapeID(e.ShapeIDB)
		foldContactID(e.ContactID)
		foldFloat(e.Point.X)
		foldFloat(e.Point.Y)
		foldFloat(e.Normal.X)
		foldFloat(e.Normal.Y)
		foldFloat(e.ApproachSpeed)
	}

	sensorEvents := w.SensorEvents()
	hash = djb2Fold(hash, uint64(len(sensorEvents.BeginEvents)))
	for i := range sensorEvents.BeginEvents {
		e := &sensorEvents.BeginEvents[i]
		foldShapeID(e.SensorShapeID)
		foldShapeID(e.VisitorShapeID)
	}
	hash = djb2Fold(hash, uint64(len(sensorEvents.EndEvents)))
	for i := range sensorEvents.EndEvents {
		e := &sensorEvents.EndEvents[i]
		foldShapeID(e.SensorShapeID)
		foldShapeID(e.VisitorShapeID)
	}

	jointEvents := w.JointEvents().JointEvents
	hash = djb2Fold(hash, uint64(len(jointEvents)))
	for i := range jointEvents {
		e := &jointEvents[i]
		hash = djb2Fold(hash, box2d.PackJointID(e.JointID)&^worldTokenMask)
		hash = djb2Fold(hash, e.UserData)
	}

	return hash
}

// workerStressSignature steps the stress scene and records, per step, the
// world-state hash folded with the full contents of every event stream and
// the task count. withCallbacks additionally installs the full user-callback
// set (installWorkerStressCallbacks) in its results-preserving form.
func workerStressSignature(workerCount, stepCount int, withCallbacks bool) []uint64 {
	def := box2d.DefaultWorldDef()
	def.WorkerCount = workerCount
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	bodies := buildWorkerStressScene(w)
	if withCallbacks {
		installWorkerStressCallbacks(w)
	}

	signature := make([]uint64, 0, stepCount)
	for range stepCount {
		w.Step(1.0/60.0, 4)

		hash := hashWorldState(w, bodies)
		hash = foldWorkerEventStreams(hash, w)
		hash = djb2Fold(hash, uint64(w.Counters().TaskCount))
		signature = append(signature, hash)
	}

	return signature
}

// TestWorkerStressSceneMatchesSerial proves byte-identity on a scene large
// enough that every stage actually dispatches to the pool (see
// buildWorkerStressScene), including the event streams — the golden scenes
// alone cannot show this because most of their dispatches fall below the
// grain thresholds and run inline. Run with -race to prove the dispatches
// are race-clean.
func TestWorkerStressSceneMatchesSerial(t *testing.T) {
	const stepCount = 150

	want := workerStressSignature(1, stepCount, false)

	for _, workerCount := range goldenWorkerCounts {
		t.Run(fmt.Sprintf("workers=%d", workerCount), func(t *testing.T) {
			got := workerStressSignature(workerCount, stepCount, false)
			require.Equal(t, want, got,
				"stress scene at WorkerCount=%d diverges from serial — solver/finalize/sensor dispatch determinism broken",
				workerCount)
		})
	}
}

// TestWorkerStressCallbacksMatchSerial covers the doc.go concurrent-callback
// contract, which previously had zero test coverage: the full user-callback
// set (preSolve, custom filter, friction/restitution mixing) is installed in
// its results-preserving form (installWorkerStressCallbacks) and the
// signature must equal the callbacks-OFF serial signature. workers=1 proves
// callbacks-on == callbacks-off; workers=8 proves the parallel dispatches
// invoke the callbacks without changing results — and running this test
// under -race (it matches the CI 'TestWorkerStress' pattern) proves the
// concurrent-invocation contract itself.
func TestWorkerStressCallbacksMatchSerial(t *testing.T) {
	// 120 steps still covers first touch (where preSolve runs on every new
	// contact), the bullet flights and the sensor churn.
	const stepCount = 120

	want := workerStressSignature(1, stepCount, false)

	for _, workerCount := range []int{1, 8} {
		t.Run(fmt.Sprintf("workers=%d", workerCount), func(t *testing.T) {
			got := workerStressSignature(workerCount, stepCount, true)
			require.Equal(t, want, got,
				"stress scene with callbacks installed at WorkerCount=%d diverges from the callbacks-off serial run",
				workerCount)
		})
	}
}

// taskCountTrace steps the stress scene and records Counters().TaskCount
// after every step (taskCount resets at the top of each Step, so each entry
// is that step's stage count).
func taskCountTrace(workerCount, stepCount int) []int {
	def := box2d.DefaultWorldDef()
	def.WorkerCount = workerCount
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	buildWorkerStressScene(w)

	trace := make([]int, 0, stepCount)
	for range stepCount {
		w.Step(1.0/60.0, 4)
		trace = append(trace, w.Counters().TaskCount)
	}

	return trace
}

// TestTaskCountParityAcrossWorkerCounts pins the taskCount contract:
// taskCount counts stages, not per-worker tasks (see the world.go header), so
// Counters().TaskCount must be identical at every worker count. Upstream's
// activeTaskCount-style per-task counting would fail this the moment a stage
// fans out.
func TestTaskCountParityAcrossWorkerCounts(t *testing.T) {
	const stepCount = 60

	want := taskCountTrace(1, stepCount)
	got := taskCountTrace(8, stepCount)

	require.Equal(t, want, got,
		"Counters().TaskCount differs between WorkerCount=1 and WorkerCount=8 — taskCount must count stages, not per-worker tasks")
}
