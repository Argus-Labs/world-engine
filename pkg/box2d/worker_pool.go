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
//     splits [0, n) into static contiguous ascending ranges by ceiling
//     division (workerRange), a pure function of (n, workerCount, k).
//     Per-item arithmetic is unchanged by the split, and every per-worker
//     output is merged either by bit-OR (order-free) or by ascending-worker
//     concatenation, which equals ascending item order because the ranges
//     are contiguous ascending. Results are therefore byte-identical for
//     every worker count — a stronger guarantee than upstream, which
//     tolerates scheduling-dependent internal orders (its bulletBodies
//     atomic append, for example). See the merge points in world_step.go,
//     solver.go and broad_phase.go.
//   - Below a per-stage grain threshold (upstream minRange) the dispatch
//     runs inline on worker 0. That is just the workerCount==1 partition, so
//     it cannot change results.
//
// DETERMINISM: this file contains no floating-point arithmetic — the pool
// moves only ints, funcs and captured panics. The no-FMA assembly gate
// (nofma_test.go) covers it for free.

package box2d

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// workerPanicSlot records a panic captured on one worker during a dispatch.
// Slots are padded to a cache line so concurrent writes by different workers
// never share a line.
type workerPanicSlot struct {
	val   any
	stack []byte
	_     [64]byte
}

// workerPool is the persistent per-World worker pool. It exists only when the
// world's effective worker count is greater than one (World.pool is nil
// otherwise and every stage runs today's exact serial call).
//
// Synchronization contract: only the Step goroutine (worker 0) dispatches,
// one job at a time. The job fields (n, fn) are published before the channel
// sends and read by workers after the receive, so the channel gives the
// happens-before edge in; wg.Done/wg.Wait gives the edge out. The race
// detector understands both, which makes the -race golden runs a genuine
// synchronization proof.
type workerPool struct {
	// jobs carries worker indices. Each dispatch sends indices
	// 1..workerCount-1; the receiving goroutine executes that worker's range
	// (worker identity travels in the message, so any parked goroutine may
	// serve any index — the range itself is a pure function of the index).
	// Closed by close() to terminate the workers.
	jobs chan int

	// wg is the fan-in barrier for the current dispatch.
	wg sync.WaitGroup

	// done tracks worker-goroutine lifetime so close() can join them.
	done sync.WaitGroup

	workerCount int

	// Current job, valid between publish and wg.Wait: item count and the
	// range function. fn must write only worker-local or item-owned state.
	n  int
	fn func(workerIndex, startIndex, endIndex int)

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
	close(p.jobs)
	p.done.Wait()
}

// workerLoop parks on the job channel and executes one range per received
// worker index until the channel closes.
func (p *workerPool) workerLoop() {
	defer p.done.Done()
	for k := range p.jobs {
		p.runRange(k)
	}
}

// runRange executes the current job's range for worker k. A panic is captured
// into the worker's padded slot and wg.Done runs in the defer, so a panicking
// worker always releases the barrier and survives to serve the next job.
func (p *workerPool) runRange(k int) {
	defer func() {
		if r := recover(); r != nil {
			p.panics[k].val = r
			p.panics[k].stack = debug.Stack()
		}
		p.wg.Done()
	}()

	startIndex, endIndex := workerRange(p.n, p.workerCount, k)
	if startIndex < endIndex {
		p.fn(k, startIndex, endIndex)
	}
}

// forRange runs fn over [0, n) split into static contiguous ascending ranges,
// one per worker, and returns only after every range completed (full
// barrier). fn(workerIndex, startIndex, endIndex) must write only
// worker-local or item-owned state; empty ranges are skipped. When
// forRangeWorkers(n, grain, workerCount) == 1 the whole range runs inline on
// worker 0 — a serial consumer that later walks per-worker results must
// derive the partition with the same forRangeWorkers/workerRange calls so
// both sides agree (see the broad-phase pair creation loop).
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

	if forRangeWorkers(n, grain, p.workerCount) == 1 {
		fn(0, 0, n)
		return
	}

	p.n = n
	p.fn = fn

	p.wg.Add(p.workerCount)
	for k := 1; k < p.workerCount; k++ {
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
	// with the original stack preserved in the message.
	for k := range p.panics {
		if p.panics[k].val != nil {
			val := p.panics[k].val
			stack := p.panics[k].stack
			for j := range p.panics {
				p.panics[j].val = nil
				p.panics[j].stack = nil
			}
			panic(fmt.Sprintf("box2d: worker %d panicked during World.Step: %v\n%s", k, val, stack))
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

// forRangeWorkers returns the number of workers forRange uses for n items at
// the given grain (upstream minRange): 1 (inline on worker 0) below the grain
// threshold, workerCount otherwise. Exposed separately so serial consumers of
// per-worker results use the exact partition the dispatch used.
func forRangeWorkers(n, grain, workerCount int) int {
	if n < grain {
		return 1
	}
	return workerCount
}
