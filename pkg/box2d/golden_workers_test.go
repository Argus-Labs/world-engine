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
// Worker counts above runtime.GOMAXPROCS(0) are clamped by NewWorld; the w=8
// rows still check the byte-identity claim on smaller machines because
// results are partition-independent by construction (worker_pool.go).

package box2d_test

import (
	"encoding/json"
	"fmt"
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
// thresholds so the joint-event bookkeeping runs), 20 sensors inside the
// pyramid (past the sensor grain), and 6 bullets fired through the sensors
// (bullet gather + serial bullet stage + continuous sensor hits).
func buildWorkerStressScene(w *box2d.World) []box2d.BodyID {
	var bodies []box2d.BodyID

	gd := box2d.DefaultBodyDef()
	gd.Position = box2d.Vec2{X: 0.0, Y: -10.0}
	ground := w.CreateBody(&gd)
	groundBox := box2d.MakeBox(120.0, 10.0)
	gsd := box2d.DefaultShapeDef()
	w.CreatePolygonShape(ground, &gsd, &groundBox)

	// Pyramid (same construction as buildPyramidScene, more rows).
	const rows = 20
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
			w.CreatePolygonShape(body, &sd, &box)
			bodies = append(bodies, body)
		}
	}

	// Independent pendulums: disjoint bodies, so the joints share one graph
	// color and exceed the per-color grain.
	for i := range 80 {
		fi := float64(i)
		anchorPos := box2d.Vec2{X: -60.0 + float64(1.5*fi), Y: 30.0}

		ad := box2d.DefaultBodyDef()
		ad.Position = anchorPos
		anchor := w.CreateBody(&ad)

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
	for i := range 20 {
		fi := float64(i)
		sbd := box2d.DefaultBodyDef()
		sbd.Position = box2d.Vec2{X: -9.5 + fi, Y: 0.5 + float64(0.35*fi)}
		sensorBody := w.CreateBody(&sbd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.6}
		sd := box2d.DefaultShapeDef()
		sd.IsSensor = true
		sd.EnableSensorEvents = true
		w.CreateCircleShape(sensorBody, &sd, &circle)
	}

	// Bullets fired through the sensors and into the pyramid.
	for i := range 6 {
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
		w.CreateCircleShape(bullet, &sd, &circle)
		bodies = append(bodies, bullet)
	}

	return bodies
}

// workerStressSignature steps the stress scene and records, per step, the
// world-state hash folded with every event count and the task count.
func workerStressSignature(workerCount, stepCount int) []uint64 {
	def := box2d.DefaultWorldDef()
	def.WorkerCount = workerCount
	w := box2d.NewWorld(&def)
	defer w.Destroy()

	bodies := buildWorkerStressScene(w)

	signature := make([]uint64, 0, stepCount)
	for range stepCount {
		w.Step(1.0/60.0, 4)

		hash := hashWorldState(w, bodies)
		sensorEvents := w.SensorEvents()
		contactEvents := w.ContactEvents()
		hash = djb2Fold(hash, uint64(len(sensorEvents.BeginEvents)))
		hash = djb2Fold(hash, uint64(len(sensorEvents.EndEvents)))
		hash = djb2Fold(hash, uint64(len(contactEvents.BeginEvents)))
		hash = djb2Fold(hash, uint64(len(contactEvents.EndEvents)))
		hash = djb2Fold(hash, uint64(len(contactEvents.HitEvents)))
		hash = djb2Fold(hash, uint64(len(w.JointEvents().JointEvents)))
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

	want := workerStressSignature(1, stepCount)

	for _, workerCount := range goldenWorkerCounts {
		t.Run(fmt.Sprintf("workers=%d", workerCount), func(t *testing.T) {
			got := workerStressSignature(workerCount, stepCount)
			require.Equal(t, want, got,
				"stress scene at WorkerCount=%d diverges from serial — solver/finalize/sensor dispatch determinism broken",
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
