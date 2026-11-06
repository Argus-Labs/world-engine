// External robustness tests for the internal worker pool (worker_pool.go):
// panic propagation with the ORIGINAL panic value through a parallel Step,
// deadlock-free Destroy after a worker panic, the goroutine close-join
// (no leak across NewWorld/Destroy cycles), and the locked-world guard on
// World.Destroy that prevents a callback-driven pool.close() self-join
// deadlock.

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

// boundedTimeout limits how long runBounded waits for completion.
const boundedTimeout = 30 * time.Second

// assertFailedMsg is the shared panic string every box2d assert emits in the
// box2d_asserts build; the Destroy guard regression asserts this exact value.
const assertFailedMsg = "box2d: assertion failed"

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
	id := w.ID()

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
	require.Nil(t, runBounded(t, w.Destroy, "World.Destroy deadlocked after a worker panic"),
		"Destroy must not panic on the poisoned world")
	// Destroy must actually tear the poisoned world down, not silently skip
	// cleanup: the saved id is now stale. Returning alone (the runBounded
	// check above) would be satisfied by a locked-world early return that
	// leaves the worker pool alive.
	require.False(t, w.IsWorldValid(id), "Destroy must invalidate the saved id after a panicked Step")

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

			// A live body id for the per-body read paths below.
			probeDef := box2d.DefaultBodyDef()
			probeDef.Type = box2d.DynamicBody
			probeDef.Position = box2d.Vec2{X: 0.0, Y: 30.0}
			probeBody := w.CreateBody(&probeDef)
			require.True(t, w.IsBodyValid(probeBody))

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

			// The read paths must fail the same way. The latched world lock
			// would otherwise make every query answer with a zero value — a
			// wrong answer, not a no-op: a recovering game loop reads (line
			// of sight, triggers, event drain) before it steps, so it would
			// act on "nothing is there" for a tick before the Step panic
			// ever fires.
			for name, call := range map[string]func(){
				"CastRay": func() {
					w.CastRay(box2d.Vec2{X: -20, Y: 1}, box2d.Vec2{X: 40, Y: 0}, box2d.DefaultQueryFilter(),
						func(box2d.ShapeID, box2d.Vec2, box2d.Vec2, float64, any) float64 { return 1 }, nil)
				},
				"OverlapAABB": func() {
					w.OverlapAABB(box2d.AABB{LowerBound: box2d.Vec2{X: -20, Y: -20}, UpperBound: box2d.Vec2{X: 20, Y: 20}},
						box2d.DefaultQueryFilter(), func(box2d.ShapeID, any) bool { return true }, nil)
				},
				"BodyEvents":    func() { w.BodyEvents() },
				"ContactEvents": func() { w.ContactEvents() },
				// The per-body read paths feed physics2d's contact gather; a
				// silent zero capacity there turns into a spurious ContactEnd
				// for every persisted touching pair.
				"BodyContactCapacity": func() { w.BodyContactCapacity(probeBody) },
				"BodyContactData":     func() { w.BodyContactData(probeBody, nil) },
				"ComputeBodyAABB":     func() { w.ComputeBodyAABB(probeBody) },
			} {
				var got any
				func() {
					defer func() { got = recover() }()
					call()
				}()
				require.NotNilf(t, got, "%s on a poisoned world must panic, not return a zero value", name)
				gotMsg, isString := got.(string)
				require.Truef(t, isString, "%s poison panic must be the explicit message string, got %T: %v", name, got, got)
				require.Contains(t, gotMsg, "unwound by a panic")
			}
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

// TestDestroyFromCallbackGuarded locks in the locked-world guard on
// World.Destroy: a user callback that calls Destroy from inside Step must not
// reach pool.close() mid-dispatch (which self-joins on the calling worker's
// done token and deadlocks) or wipe *w under a continuing Step. In release the
// callback's Destroy is a silent no-op; in the box2d_asserts build the guard's
// assert fires and Step panics with "box2d: assertion failed". The guard
// exempts poisoned worlds so Destroy still reclaims the worker pool after a
// panic-unwound Step.
//
// workers_1 (serial, no pool) and workers_4 (pool-enabled) cover the inline
// and pool-worker callback paths; workers_4 does not by itself guarantee the
// callback runs on a background worker (worker 0 can win the CAS). The
// "self_join_background_worker" case forces the callback onto a background
// pool worker by excluding worker 0 (the Step goroutine, which is not in the
// pool's done WaitGroup, so a CAS it wins does not reproduce the deadlock);
// the saved-id validity check rejects a wiped-world false positive that would
// share the assertion string.
func TestDestroyFromCallbackGuarded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		workers        int
		backgroundOnly bool
		stepTimeoutMsg string
		firedMsg       string
		destroyMsg     string
	}{
		{
			name:           "workers_1",
			workers:        1,
			stepTimeoutMsg: fmt.Sprintf("Step did not return within %s", boundedTimeout),
			firedMsg:       "preSolve callback did not run",
			destroyMsg:     "Destroy did not return",
		},
		{
			name:           "workers_4",
			workers:        4,
			stepTimeoutMsg: fmt.Sprintf("Step did not return within %s", boundedTimeout),
			firedMsg:       "preSolve callback did not run",
			destroyMsg:     "Destroy did not return — pool.close() must join the workers",
		},
		{
			name:           "self_join_background_worker",
			workers:        4,
			backgroundOnly: true,
			stepTimeoutMsg: "Step timed out while testing Destroy from a background worker",
			firedMsg:       "no background worker hit a touching pre-solve contact — self-join path not exercised",
			destroyMsg:     "Destroy did not return on the world — pool.close() must join the workers",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			def := box2d.DefaultWorldDef()
			def.WorkerCount = tc.workers
			w := box2d.NewWorld(&def)
			var stepDone atomic.Bool
			// Register cleanup after NewWorld so an early failed require still
			// joins the pool goroutines; the explicit Destroy below remains to
			// verify teardown and id invalidation. Skip cleanup while a bounded
			// Step is still in flight (a timeout leaves it running): the guard
			// no-ops Destroy while the world is locked, and a Destroy racing a
			// still-running Step could double-close the pool's jobs channel.
			t.Cleanup(func() {
				if !stepDone.Load() {
					return
				}
				w.Destroy()
			})
			id := w.ID()

			buildWorkerStressScene(w)

			// stepG holds the Step goroutine's id for the background case.
			// The store precedes Step, whose job-channel sends publish it
			// to the workers, so the callback's read is race-free without
			// an atomic; capturing the parent test goroutine here would let
			// worker 0 satisfy the filter too.
			var stepG uint64
			var fired atomic.Bool
			w.SetPreSolveCallback(func(_, _ box2d.ShapeID, _, _ box2d.Vec2, _ any) bool {
				// backgroundOnly: skip worker 0 (the Step goroutine) so only a
				// background pool worker may fire and the self-join path is
				// reached. Check fired first so later contacts skip the
				// runtime.Stack parse; the CAS still arbitrates the winner.
				if tc.backgroundOnly && (fired.Load() || goroutineID() == stepG) {
					return true
				}
				if fired.CompareAndSwap(false, true) {
					w.Destroy()
				}
				return true
			}, nil)

			panicVal := runBounded(t, func() {
				if tc.backgroundOnly {
					stepG = goroutineID()
				}
				defer stepDone.Store(true)
				w.Step(1.0/60.0, 4)
			}, tc.stepTimeoutMsg)
			// Check the callback fired before the panic value: if the background
			// case never reaches a pool worker, the Equal/Nil failure below would
			// otherwise hide the diagnostic that the self-join path was missed.
			require.True(t, fired.Load(), tc.firedMsg)
			if buildWithAsserts {
				require.Equal(t, assertFailedMsg, panicVal,
					"Step must panic with the guard's assertion, not a raw wiped-state crash")
			} else {
				require.Nil(t, panicVal, "Step must not panic when Destroy is guarded out")
			}

			// The guard rejected the callback's Destroy, so the world is
			// still live (not wiped): the saved id is still valid. The
			// shared "box2d: assertion failed" string alone cannot identify
			// this guard — a wiped world would trip a different assert and
			// satisfy the panic check above — so the validity check
			// distinguishes a real guard from that false positive.
			require.True(t, w.IsWorldValid(id),
				"the guard must reject the callback's Destroy, not wipe the world mid-Step")

			// A second Step confirms the world is healthy when the guard
			// rejected the callback's Destroy (release only; the asserts
			// build leaves the world poisoned). Bound it so a reuse-only hang
			// fails this test instead of waiting for the package timeout.
			if !buildWithAsserts {
				require.Nil(t, runBounded(t, func() { w.Step(1.0/60.0, 4) },
					fmt.Sprintf("second Step did not return within %s", boundedTimeout)),
					"second Step must not panic when the guard rejected the callback's Destroy")
			}

			// Destroy reclaims the worker pool. On the asserts-build
			// poisoned world the guard's stepPanicked exemption lets it
			// run.
			require.Nil(t, runBounded(t, w.Destroy, tc.destroyMsg),
				"Destroy must not panic on the world")
			require.False(t, w.IsWorldValid(id), "Destroy must invalidate the saved id")
		})
	}
}

// goroutineID returns the numeric goroutine id from [runtime.Stack]'s header
// ("goroutine N [...]"). It is used only to distinguish the Step goroutine
// (worker 0, which runs inline) from background pool workers (1..n-1), so the
// self-join regression is exercised deterministically rather than by CAS luck.
func goroutineID() uint64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	const prefix = "goroutine "
	id := uint64(0)
	i := len(prefix)
	for i < n && buf[i] >= '0' && buf[i] <= '9' {
		id = id*10 + uint64(buf[i]-'0')
		i++
	}
	return id
}

// runBounded runs fn on a goroutine and fails the test (with msg) if it does
// not return within boundedTimeout, recovering any panic value for the caller
// to assert. A mid-Step pool.close() self-join or a hung Destroy surfaces here
// as a timeout instead of blocking this test past the deadline. On timeout the
// calling test fails; fn is not stopped and may keep running.
func runBounded(t *testing.T, fn func(), msg string) any {
	t.Helper()
	var panicVal any
	done := make(chan struct{})
	go func() {
		defer func() { panicVal = recover(); close(done) }()
		fn()
	}()
	select {
	case <-done:
	case <-time.After(boundedTimeout):
		t.Fatal(msg)
	}
	return panicVal
}
