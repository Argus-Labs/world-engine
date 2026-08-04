// External robustness tests for the internal worker pool (worker_pool.go):
// panic propagation with the ORIGINAL panic value through a parallel Step,
// deadlock-free Destroy after a worker panic, and the goroutine close-join
// (no leak across NewWorld/Destroy cycles).

package box2d_test

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// workerBoom is a distinct panic payload type: the type assertion below fails
// if the pool ever wraps, stringifies or otherwise replaces the value a
// callback panicked with.
type workerBoom struct {
	tag string
}

// TestPanicInCallbackPropagatesOriginalValue drives a WorkerCount=4 world
// whose contact count is far past collideMinRange (so the collide stage
// genuinely fans out), panics inside the preSolve callback with a custom
// struct value, and requires: Step panics; the recovered value IS the
// original value (type and contents); Destroy afterwards completes without
// deadlock; and a fresh world steps fine — the process is healthy.
func TestPanicInCallbackPropagatesOriginalValue(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultWorldDef()
	def.WorkerCount = 4
	w := box2d.NewWorld(&def)

	buildWorkerStressScene(w)

	// Install the callback BEFORE the first Step: preSolve only runs from
	// updateContact, and the contact-recycling optimization skips
	// updateContact for settled contacts — first-touch contacts (the whole
	// pyramid's ground rows on step one) always run the narrow phase. The
	// stress scene's pyramid boxes and bullets set EnablePreSolveEvents.
	//
	// Panic on exactly one contact. The CAS keeps it to a single winner even
	// though the callback runs concurrently on several workers.
	var fired atomic.Bool
	w.SetPreSolveCallback(func(shapeIDA, shapeIDB box2d.ShapeID, point, normal box2d.Vec2, ctx any) bool {
		if fired.CompareAndSwap(false, true) {
			panic(workerBoom{tag: "pre-solve boom"})
		}
		return true
	}, nil)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		w.Step(1.0/60.0, 4)
	}()

	require.True(t, fired.Load(), "preSolve callback never ran — scene lost its touching pre-solve contacts")
	require.NotNil(t, recovered, "Step must re-raise a panic from a worker callback")
	boom, ok := recovered.(workerBoom)
	require.Truef(t, ok,
		"recovered value must be the ORIGINAL panic value (workerBoom), got %T: %v", recovered, recovered)
	require.Equal(t, "pre-solve boom", boom.tag)

	// Destroy must join the pool workers and return even though the panic
	// aborted Step mid-stage.
	destroyed := make(chan struct{})
	go func() {
		w.Destroy()
		close(destroyed)
	}()
	select {
	case <-destroyed:
	case <-time.After(30 * time.Second):
		t.Fatal("World.Destroy deadlocked after a worker panic")
	}

	// Fresh world: the process (and the package state) is healthy.
	def2 := box2d.DefaultWorldDef()
	def2.WorkerCount = 4
	w2 := box2d.NewWorld(&def2)
	defer w2.Destroy()
	buildWorkerScene(w2, 6, 4, 4, 1)
	for range 5 {
		w2.Step(1.0/60.0, 4)
	}
}

// TestStepAfterCallbackPanicFailsLoudly locks in the poison contract: a
// panic that unwinds Step abandons a half-integrated simulation state, so
// re-stepping the SAME world must fail loudly with a panic naming the cause —
// in every build — rather than freeze on the latched world lock. A server
// with panic-recovery middleware around its tick would otherwise keep calling
// Step and silently stop simulating, which is far worse to diagnose than a
// crash; and a world resumed from half a step would silently diverge from
// every deterministic replica. Destroy stays valid on the poisoned world
// (the t.Cleanup exercises it). Serial and parallel unwind paths differ
// (direct panic vs pool capture-and-re-raise), so both worker counts run.
func TestStepAfterCallbackPanicFailsLoudly(t *testing.T) {
	t.Parallel()

	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers_%d", workers), func(t *testing.T) {
			t.Parallel()

			def := box2d.DefaultWorldDef()
			def.WorkerCount = workers
			w := box2d.NewWorld(&def)
			t.Cleanup(w.Destroy)

			buildWorkerStressScene(w)

			var fired atomic.Bool
			w.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, _ any) bool {
				if fired.CompareAndSwap(false, true) {
					panic(workerBoom{tag: "poison boom"})
				}
				return true
			}, nil)

			require.Panics(t, func() { w.Step(1.0/60.0, 4) }, "the callback panic must propagate out of Step")
			require.True(t, fired.Load(), "preSolve callback never ran — scene lost its touching pre-solve contacts")

			// Re-stepping the same world must panic with the explicit poison
			// message, not silently no-op (release) or trip a bare assert
			// (box2d_asserts build).
			var second any
			func() {
				defer func() { second = recover() }()
				w.Step(1.0/60.0, 4)
			}()
			require.NotNil(t, second, "re-stepping a panicked world must panic, not silently no-op")
			msg, ok := second.(string)
			require.Truef(t, ok, "poison panic must be the explicit message string, got %T: %v", second, second)
			require.Contains(t, msg, "unwound by a panic")
		})
	}
}

// TestGoroutineLeakOnWorldDestroy locks in the close-join fix: 50 cycles of
// NewWorld(WorkerCount=8) -> 2 Steps -> Destroy must not accumulate
// goroutines (each world parks 7 pool workers that Destroy must join). The
// settle loop tolerates runtime bookkeeping goroutines finishing late.
func TestGoroutineLeakOnWorldDestroy(t *testing.T) {
	before := runtime.NumGoroutine()

	for range 50 {
		def := box2d.DefaultWorldDef()
		def.WorkerCount = 8
		w := box2d.NewWorld(&def)

		// Small scene: the leak check targets pool lifetime, not dispatch.
		gd := box2d.DefaultBodyDef()
		gd.Position = box2d.Vec2{X: 0.0, Y: -1.0}
		ground := w.CreateBody(&gd)
		groundBox := box2d.MakeBox(10.0, 1.0)
		gsd := box2d.DefaultShapeDef()
		w.CreatePolygonShape(ground, &gsd, &groundBox)

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bd.Position = box2d.Vec2{X: 0.0, Y: 2.0}
		body := w.CreateBody(&bd)
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		sd := box2d.DefaultShapeDef()
		sd.Density = 1.0
		w.CreateCircleShape(body, &sd, &circle)

		w.Step(1.0/60.0, 4)
		w.Step(1.0/60.0, 4)

		w.Destroy()
	}

	deadline := time.Now().Add(time.Second)
	after := runtime.NumGoroutine()
	for after-before > 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		after = runtime.NumGoroutine()
	}

	require.LessOrEqualf(t, after-before, 2,
		"goroutines leaked across 50 NewWorld(WorkerCount=8)/Destroy cycles: before=%d after=%d — pool close-join broken",
		before, after)
}
