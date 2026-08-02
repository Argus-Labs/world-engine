// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — replaces the user task
// system declared in include/box2d/types.h (b2EnqueueTaskCallback,
// b2FinishTaskCallback, b2WorldDef.workerCount) and the serial defaults in
// src/physics_world.c (b2DefaultAddTaskFcn, b2DefaultFinishTaskFcn).
//
// DESIGN DEVIATION (approved): internal goroutine pool instead of user tasks.
//
// Upstream creates no threads: the user supplies enqueueTask/finishTask
// callbacks plus worker threads, and Box2D partitions parallel-for ranges
// into blocks that workers claim by CAS with work stealing (solver.c
// b2ExecuteStage). This port keeps the whole task system internal:
//
//   - WorldDef.WorkerCount selects the worker count; the upstream callbacks
//     are NOT exposed. NewWorld spawns workerCount-1 persistent goroutines
//     that park on a channel between dispatches; World.Destroy tears them
//     down. The Step goroutine is worker 0 and always executes its own range
//     inline, matching upstream where the main thread runs stages too.
//   - NO work stealing and NO atomics for partitioning. Every dispatch
//     engages forRangeWorkers(n, grain, workerCount) workers — grain is the
//     minimum items per worker (upstream minRange semantics, types.h) — and
//     splits [0, n) into static contiguous ascending ranges by ceiling
//     division (workerRange), a pure function of (n, grain, workerCount, k).
//     Per-item arithmetic is unchanged by the split, and every per-worker
//     output is merged either by bit-OR (order-free) or by ascending-worker
//     concatenation, which equals ascending item order because the ranges
//     are contiguous ascending. Results are therefore byte-identical for
//     every worker count — a stronger guarantee than upstream, which
//     tolerates scheduling-dependent internal orders (its bulletBodies
//     atomic append, for example). See the merge points in world_step.go,
//     solver.go and broad_phase.go.
//   - When forRangeWorkers returns 1 (fewer than 2*grain items) the dispatch
//     runs inline on worker 0. That is just the single-worker partition, so
//     it cannot change results.
//
// DETERMINISM: this file contains no floating-point arithmetic — the pool
// moves only ints, funcs and captured panics. The no-FMA assembly gate
// (nofma_test.go) covers it for free.

package box2d

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// workerPanicSlot records a panic captured on one worker during a dispatch.
// Slots are padded to a cache line so concurrent writes by different workers
// never share a line.
type workerPanicSlot struct {
	val any
	_   [64]byte
}

// workerPool is the persistent per-World worker pool. It exists only when the
// world's effective worker count is greater than one (World.pool is nil
// otherwise and every stage runs today's exact serial call).
//
// Synchronization contract: only the Step goroutine (worker 0) dispatches,
// one job at a time. The job fields (n, jobWorkers, fn) are published before
// the channel sends and read by workers after the receive, so the channel
// gives the happens-before edge in; wg.Done/wg.Wait gives the edge out. The
// race detector understands both, which makes the -race golden runs a genuine
// synchronization proof.
type workerPool struct {
	// jobs carries worker indices. Each dispatch sends indices
	// 1..jobWorkers-1; the receiving goroutine executes that worker's range
	// (worker identity travels in the message, so any parked goroutine may
	// serve any index — the range itself is a pure function of the index).
	// Closed by close() to terminate the workers.
	jobs chan int

	// wg is the fan-in barrier for the current dispatch.
	wg sync.WaitGroup

	// done tracks worker-goroutine lifetime so close() can join them.
	done sync.WaitGroup

	// closed is set at the top of close() so a worker goroutine dying
	// abnormally during shutdown is not respawned (see workerLoop).
	closed atomic.Bool

	workerCount int

	// Current job, valid between publish and wg.Wait: item count, the number
	// of workers this dispatch engages (jobWorkers <= workerCount, from
	// forRangeWorkers) and the range function. fn must write only
	// worker-local or item-owned state.
	n          int
	jobWorkers int
	fn         func(workerIndex, startIndex, endIndex int)

	// panics holds per-worker captured panics, indexed by worker.
	panics []workerPanicSlot
}

// newWorkerPool creates the pool and parks workerCount-1 goroutines on the
// job channel. The goroutines live until close() — a World that is never
// Destroyed leaks them, the same way upstream leaks whatever threads the user
// task system created.
func newWorkerPool(workerCount int) *workerPool {
	assert(workerCount > 1)

	p := &workerPool{
		// Buffered so the dispatcher can fire every wakeup without blocking
		// and start its own range immediately.
		jobs:        make(chan int, workerCount-1),
		workerCount: workerCount,
		panics:      make([]workerPanicSlot, workerCount),
	}

	p.done.Add(workerCount - 1)
	for range workerCount - 1 {
		go p.workerLoop()
	}

	return p
}

// close terminates the worker goroutines and joins them: when it returns, no
// worker is running or will run again, so the caller may invalidate anything
// they could reference. Must be called with no dispatch in flight
// (World.Destroy runs strictly outside Step).
func (p *workerPool) close() {
	p.closed.Store(true)
	close(p.jobs)
	p.done.Wait()
}

// workerLoop parks on the job channel and executes one range per received
// worker index until the channel closes.
//
// Self-heal: a dispatched fn that calls runtime.Goexit (t.Fatal inside a user
// callback under test, for example) unwinds this goroutine without a panic.
// runRange has already recorded the sentinel and released the barrier, but
// the goroutine itself is dying — so the deferred check below respawns a
// replacement that inherits this slot's done token (done.Done is NOT called),
// keeping the pool at full strength for the next dispatch instead of
// deadlocking it. A normal channel-closed exit reaches finished=true and
// releases the done token as before.
func (p *workerPool) workerLoop() {
	finished := false
	defer func() {
		if !finished && !p.closed.Load() {
			go p.workerLoop()
			return
		}
		p.done.Done()
	}()

	for k := range p.jobs {
		p.runRange(k)
	}
	finished = true
}

// runRange executes the current job's range for worker k. A panic is captured
// into the worker's padded slot and wg.Done runs in the defer, so a panicking
// worker always releases the barrier and survives to serve the next job. A
// fn that exits via runtime.Goexit instead of panicking (recover() == nil but
// the frame never completed) records a sentinel value so forRange still
// reports the failure; workerLoop then respawns the dying goroutine.
func (p *workerPool) runRange(k int) {
	completed := false
	defer func() {
		if r := recover(); r != nil {
			p.panics[k].val = r
		} else if !completed {
			p.panics[k].val = fmt.Sprintf(
				"box2d: worker %d exited via runtime.Goexit inside a callback", k)
		}
		p.wg.Done()
	}()

	startIndex, endIndex := workerRange(p.n, p.jobWorkers, k)
	if startIndex < endIndex {
		p.fn(k, startIndex, endIndex)
	}
	completed = true
}

// forRange runs fn over [0, n) split into static contiguous ascending ranges
// among forRangeWorkers(n, grain, workerCount) workers, and returns only
// after every range completed (full barrier). fn(workerIndex, startIndex,
// endIndex) must write only worker-local or item-owned state; empty ranges
// are skipped. When the engaged count is 1 the whole range runs inline on
// worker 0. A serial consumer that later walks per-worker results must derive
// the partition with the same forRangeWorkers/workerRange calls so both sides
// agree (see the broad-phase pair creation loop).
//
// The dispatch itself allocates nothing: the job travels through pool fields
// and plain ints on the channel. (The fn closure is built once per stage call
// site by the caller, not per worker.)
//
// Panic safety: worker panics are captured per worker; after the barrier the
// lowest-indexed captured panic is re-raised on the Step goroutine, so
// World.Step never deadlocks on a crashed worker and the panic surfaces where
// the caller can see it. After a worker panic the world is in an undefined
// mid-step state (arena allocations live, World locked) — the same contract
// as a panic inside the serial Step.
func (p *workerPool) forRange(n, grain int, fn func(workerIndex, startIndex, endIndex int)) {
	if n <= 0 {
		return
	}

	jobWorkers := forRangeWorkers(n, grain, p.workerCount)
	if jobWorkers == 1 {
		fn(0, 0, n)
		return
	}

	p.n = n
	p.jobWorkers = jobWorkers
	p.fn = fn

	p.wg.Add(jobWorkers)
	for k := 1; k < jobWorkers; k++ {
		p.jobs <- k
	}

	// Worker 0 is this goroutine. runRange captures a panic in the inline
	// share too, so wg.Wait below always runs and the barrier cannot be
	// abandoned with workers still executing.
	p.runRange(0)
	p.wg.Wait()

	// Drop the job so the closure (and anything it captures, e.g. the step
	// context) does not outlive the dispatch.
	p.fn = nil

	// Re-raise the lowest-indexed captured panic — a deterministic choice —
	// with the ORIGINAL panic value, unchanged, so the parallel path matches
	// the below-grain inline path (which propagates the value natively) and
	// callers can type-assert it. The failing worker's own stack is not
	// preserved in the new trace; results are byte-identical across worker
	// counts, so any panic reproduces at WorkerCount=1 with a full native
	// stack — that is the supported debugging path. A runtime.Goexit inside a
	// callback surfaces here as its sentinel string (see runRange).
	for k := range p.panics {
		if p.panics[k].val != nil {
			val := p.panics[k].val
			for j := range p.panics {
				p.panics[j].val = nil
			}
			panic(val)
		}
	}
}

// workerRange returns worker k's contiguous half-open range [start, end) of n
// items divided among workerCount workers by ceiling division. It is a pure
// function of (n, workerCount, k) — no scheduling input — which is the
// cornerstone of the byte-identical-for-every-workerCount guarantee: because
// the ranges are contiguous and ascending in k, walking workers in ascending
// index visits items in exactly the serial ascending-item order.
func workerRange(n, workerCount, k int) (int, int) {
	perWorker := (n + workerCount - 1) / workerCount
	start := min(k*perWorker, n)
	end := min(start+perWorker, n)
	return start, end
}

// forRangeWorkers returns the number of workers forRange engages for n items
// at the given grain: clamp(n/grain, 1, workerCount). grain is the minimum
// number of items per worker (upstream minRange semantics, types.h), so small
// dispatches engage few workers instead of slicing the range workerCount
// ways. It is a pure function of (n, grain, workerCount) — the same value
// must be used for a stage's dispatch, its taskContext presize and its merge,
// so every bound stays partition-independent. Exposed separately so serial
// consumers of per-worker results use the exact partition the dispatch used.
func forRangeWorkers(n, grain, workerCount int) int {
	if n <= 0 {
		return 1
	}
	return min(max(n/grain, 1), workerCount)
}
