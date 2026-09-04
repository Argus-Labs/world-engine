// Tests for the internal worker pool (worker_pool.go): the partition
// functions (workerRange, forRangeWorkers), the fan-out of a real pool
// dispatch, the NewWorld pool-engagement wiring, and the runtime.Goexit
// self-heal path. Internal package tests because the partition functions and
// the pool itself are unexported by design.
//
// The partition properties pinned here are the cornerstone of the
// byte-identical-for-every-worker-count guarantee: every dispatch bound,
// presize bound and merge bound in the engine is the same pure function of
// (item count, grain), so any regression in these functions silently breaks
// determinism everywhere at once.

package box2d

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPoolWorkerRangeProperties exhaustively checks the partition invariants
// of workerRange over n in [0, 300] and workerCount in [1, 64]: the ranges
// are contiguous, ascending, disjoint, exactly cover [0, n), and empty ranges
// appear only at the tail. Plain if/t.Fatalf keeps the ~1.2M iterations fast.
func TestPoolWorkerRangeProperties(t *testing.T) {
	t.Parallel()

	for workerCount := 1; workerCount <= 64; workerCount++ {
		for n := 0; n <= 300; n++ {
			prevEnd := 0
			sawEmpty := false

			for k := range workerCount {
				start, end := workerRange(n, workerCount, k)

				if start != prevEnd {
					t.Fatalf("workerRange(%d, %d, %d) start=%d, want %d (contiguous ascending)",
						n, workerCount, k, start, prevEnd)
				}
				if end < start || end > n {
					t.Fatalf("workerRange(%d, %d, %d) = [%d, %d): invalid range", n, workerCount, k, start, end)
				}
				if start == end {
					sawEmpty = true
				} else if sawEmpty {
					t.Fatalf("workerRange(%d, %d, %d) = [%d, %d): non-empty range after an empty one",
						n, workerCount, k, start, end)
				}

				prevEnd = end
			}

			if prevEnd != n {
				t.Fatalf("workerRange(%d, %d, ·) covers [0, %d), want [0, %d)", n, workerCount, prevEnd, n)
			}
		}
	}
}

// TestPoolForRangeWorkersProperties exhaustively checks the capped engagement
// function: the result is always in [1, workerCount], equals n/grain whenever
// that quotient lands in range, and clamps to the nearer bound otherwise.
func TestPoolForRangeWorkersProperties(t *testing.T) {
	t.Parallel()

	for _, grain := range []int{1, 16, 64} {
		for workerCount := 1; workerCount <= 64; workerCount++ {
			for n := 0; n <= 300; n++ {
				got := forRangeWorkers(n, grain, workerCount)

				if got < 1 || got > workerCount {
					t.Fatalf("forRangeWorkers(%d, %d, %d) = %d, outside [1, %d]", n, grain, workerCount, got, workerCount)
				}

				switch quotient := n / grain; {
				case n <= 0 || quotient < 1:
					if got != 1 {
						t.Fatalf("forRangeWorkers(%d, %d, %d) = %d, want 1", n, grain, workerCount, got)
					}
				case quotient > workerCount:
					if got != workerCount {
						t.Fatalf("forRangeWorkers(%d, %d, %d) = %d, want cap %d", n, grain, workerCount, got, workerCount)
					}
				default:
					if got != quotient {
						t.Fatalf("forRangeWorkers(%d, %d, %d) = %d, want n/grain = %d", n, grain, workerCount, got, quotient)
					}
				}
			}
		}
	}
}

// forRangeProbe dispatches fn-free probe work on the pool and returns the set
// of worker indices that executed a range. The mutex is test-only overhead;
// production dispatches never share state between workers.
func forRangeProbe(p *workerPool, n, grain int) map[int]bool {
	var mu sync.Mutex
	seen := make(map[int]bool)

	p.forRange(n, grain, func(workerIndex, startIndex, endIndex int) {
		mu.Lock()
		seen[workerIndex] = true
		mu.Unlock()
	})

	return seen
}

// TestPoolForRangeFanOut probes a real 8-worker pool: a large dispatch
// engages every worker, a small one engages exactly the capped count, and a
// below-grain one runs inline on worker 0 only.
func TestPoolForRangeFanOut(t *testing.T) {
	t.Parallel()

	pool := newWorkerPool(8)
	defer pool.close()

	require.Equal(t, map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true},
		forRangeProbe(pool, 1000, 10), "n=1000 grain=10 must engage all 8 workers")

	require.Equal(t, map[int]bool{0: true, 1: true},
		forRangeProbe(pool, 25, 10), "n=25 grain=10 must engage exactly workers {0,1} (capped engagement)")

	require.Equal(t, map[int]bool{0: true},
		forRangeProbe(pool, 5, 10), "n=5 grain=10 must run inline on worker 0")
}

// TestPoolEngagementCanary pins the NewWorld wiring: a WorkerCount above one
// creates a live pool at exactly that width, and 0/1 stay serial (pool ==
// nil). This is the test that makes a future silently-ignored WorkerCount
// loud — if NewWorld ever starts clamping or dropping the value, this fails
// before any golden matrix can go vacuously green.
func TestPoolEngagementCanary(t *testing.T) {
	t.Parallel()

	def := DefaultWorldDef()
	def.WorkerCount = 8
	w := NewWorld(&def)
	require.NotNil(t, w.pool, "WorkerCount=8 must create the worker pool")
	require.Equal(t, 8, w.workerCount, "effective worker count must match WorkerCount")
	require.Equal(t, 8, w.pool.workerCount, "pool width must match WorkerCount")
	w.Destroy()

	def = DefaultWorldDef()
	def.WorkerCount = 1
	w = NewWorld(&def)
	require.Nil(t, w.pool, "WorkerCount=1 must stay serial (pool == nil)")
	require.Equal(t, 1, w.workerCount)
	w.Destroy()

	def = DefaultWorldDef()
	def.WorkerCount = 0
	w = NewWorld(&def)
	require.Nil(t, w.pool, "WorkerCount=0 means 1 (serial), pool == nil")
	require.Equal(t, 1, w.workerCount)
	w.Destroy()
}

// TestGoexitSelfHealRespawnsWorker exercises the workerLoop self-heal: a
// dispatched fn that exits via runtime.Goexit (t.Fatal inside a user callback
// under test, for example) must surface as the sentinel panic on the
// dispatcher — not a deadlock — and the pool must respawn the dead goroutine
// so the NEXT dispatch on the same pool still engages every worker. The
// whole sequence runs on a spawned goroutine under an explicit timeout guard
// so a regression fails fast instead of hanging the run.
func TestGoexitSelfHealRespawnsWorker(t *testing.T) {
	t.Parallel()

	pool := newWorkerPool(4)

	type outcome struct {
		recovered any
		seen      map[int]bool
	}
	resultCh := make(chan outcome, 1)

	go func() {
		var result outcome

		func() {
			defer func() { result.recovered = recover() }()

			var fired atomic.Bool
			// 400 items at grain 100 engages all 4 workers; index 2 is always
			// dispatched to a pool goroutine (worker 0 runs inline on the
			// dispatcher), so the Goexit deterministically kills a pool worker.
			pool.forRange(400, 100, func(workerIndex, startIndex, endIndex int) {
				if workerIndex == 2 && fired.CompareAndSwap(false, true) {
					runtime.Goexit()
				}
			})
		}()

		// Second dispatch on the SAME pool: it only completes if the respawned
		// worker goroutine serves, which is exactly the self-heal contract.
		var mu sync.Mutex
		result.seen = make(map[int]bool)
		pool.forRange(400, 100, func(workerIndex, startIndex, endIndex int) {
			mu.Lock()
			result.seen[workerIndex] = true
			mu.Unlock()
		})

		resultCh <- result
	}()

	select {
	case result := <-resultCh:
		require.NotNil(t, result.recovered, "forRange must panic when a worker exits via runtime.Goexit")
		sentinel, ok := result.recovered.(string)
		require.Truef(t, ok, "Goexit sentinel must be the recorded string, got %T: %v",
			result.recovered, result.recovered)
		require.Contains(t, sentinel, "runtime.Goexit")
		require.Contains(t, sentinel, "worker 2")
		require.Equal(t, map[int]bool{0: true, 1: true, 2: true, 3: true}, result.seen,
			"dispatch after Goexit must engage all 4 workers — self-heal respawn broken")
	case <-time.After(30 * time.Second):
		t.Fatal("worker pool deadlocked after runtime.Goexit inside a dispatched fn")
	}

	closed := make(chan struct{})
	go func() {
		pool.close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(30 * time.Second):
		t.Fatal("pool.close deadlocked after Goexit self-heal")
	}

	// A fresh parallel world stays healthy end to end.
	def := DefaultWorldDef()
	def.WorkerCount = 4
	w := NewWorld(&def)
	defer w.Destroy()
	for range 3 {
		w.Step(1.0/60.0, 4)
	}
}
