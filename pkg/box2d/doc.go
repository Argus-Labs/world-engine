// Package box2d is a native Go port of Box2D v3.2.0, the 2D physics engine
// by Erin Catto (https://github.com/erincatto/box2d).
//
// The port targets full Box2D v3 feature parity: rigid bodies, the v3
// TGS-soft solver with sub-stepping, all shape types, all seven joint types,
// sensors, continuous collision (bullets), and world queries. Source files
// mirror the upstream C files one-to-one so changes can be diffed against
// upstream mechanically. Portions of the collision code are seeded from the
// hand-written ByteArena Go port of Box2D 2.4 (zlib license); see LICENSE.
//
// # Differences from upstream
//
//   - All arithmetic is float64 (upstream is float32).
//   - Worlds are values returned by NewWorld, not entries in a global
//     registry; every b2World_*/b2Body_*/b2Shape_*/b2Joint_* function is a
//     method on *World.
//   - The upstream task system is ported as an internal goroutine pool
//     (worker_pool.go): WorldDef.WorkerCount selects how many workers
//     World.Step may use (0 means 1, i.e. fully serial), and the World owns
//     its worker goroutines from NewWorld to Destroy. Upstream's
//     user-supplied enqueueTask/finishTask callbacks are NOT exposed.
//
// # Multithreading
//
// With WorldDef.WorkerCount > 1, World.Step fans its parallel stages out to a
// persistent internal worker pool. Unlike upstream — where results may vary
// with the worker count and scheduling — simulation results here are
// BYTE-IDENTICAL for every worker count: work is split into static contiguous
// ascending ranges (a pure function of the item count and worker count,
// no work stealing), per-item arithmetic is unchanged by the split, and every
// per-worker output is merged either order-free (bit-set OR) or in ascending
// worker order, which equals the serial order. WorkerCount is therefore
// purely a throughput knob; it can differ between machines, or between a live
// run and a replay, without affecting determinism. The count is taken as
// given (upstream does not clamp it either): values above the machine's core
// count oversubscribe the scheduler and cost throughput, never correctness.
// Small dispatches engage fewer workers than WorkerCount — each stage has a
// minimum items-per-worker grain (upstream minRange) — so tiny worlds run
// mostly inline no matter how many workers were requested. The golden suites
// enforce determinism by re-running every scene at WorkerCount 2/4/8 against
// the serial golden files (golden_workers_test.go).
//
// Everything outside Step keeps the single-goroutine contract: a World must
// not be stepped, mutated, or queried from two goroutines at once, nor
// queried while Step is running.
//
// User callbacks run concurrently when WorkerCount > 1: the preSolve callback
// (SetPreSolveCallback), the custom filter callback (SetCustomFilterCallback)
// and the friction/restitution mixing callbacks are invoked from pool workers
// during the parallel stages, possibly simultaneously from several
// goroutines. They must be safe to call concurrently and must not mutate
// shared state — a callback that gives scheduling-dependent answers also
// voids the byte-identical-for-every-worker-count guarantee.
//
// # Determinism
//
// The port is bit-deterministic for identical inputs across runs, platforms,
// and architectures (amd64/arm64). This relies on package-wide coding rules:
//
//   - No *unrounded* product may reach a + or -. The Go compiler may fuse
//     such a pair into a single FMA instruction, which rounds once instead
//     of twice and so produces different bits on targets that have FMA
//     (arm64 always, amd64 at GOAMD64=v3) than on targets that do not. The
//     product does not have to be written inline to be fusable: it also
//     fuses when it arrives through a local, a struct field, or a function
//     parameter that is later inlined. Products are therefore rounded with
//     float64(...) where they are formed, and the arithmetic helpers in
//     math_fma.go additionally round the operands they receive.
//     This rule is enforced mechanically by TestNoFusedMultiplyAdd, which
//     compiles the package for FMA-capable targets and fails if any FMA
//     instruction is emitted — reviewing source for this is not reliable.
//   - Transcendentals use the ported upstream approximations (Atan2,
//     ComputeCosSin). Only exactly-rounded stdlib math functions are used
//     otherwise (Sqrt, Remainder, Floor, Abs).
//   - No Go maps or time sources in the simulation path; iteration orders
//     are explicit and match upstream data structures. The only goroutines
//     are the internal worker pool's (worker_pool.go), whose static
//     partitioning and ordered merges keep every iteration order equal to
//     the serial one (see Multithreading above).
//   - Exactly one sort runs in the simulation path: the per-sensor overlap
//     sort in sensor.go (upstream sorts the same array with qsort). Its
//     comparator is a total order over the (shapeID, generation) visitor key,
//     so the unstable sort still produces a unique result. No other code in
//     the step may sort.
//
// Cross-architecture equivalence is enforced in CI by golden-trace hash
// tests (see testdata/golden).
//
// # Profile-guided optimization
//
// The package ships a committed CPU profile, default.pgo, merged from the
// step benchmarks in bench_test.go (mixed-shape rain at 1000 and 5000
// bodies at WorkerCount 1 and the full core count; the pyramid stack and
// the jointed chain at WorkerCount 1). Go applies PGO where the final binary is built,
// so the file does not act on this package by itself: library consumers who
// want PGO-shaped codegen for the engine put a profile named default.pgo in
// their own main package directory (ideally collected from production runs
// of their game, which then also covers their own hot code) or build with
// -pgo=<file>. This package's own tests and benchmarks opt in the same way:
// go test -pgo=default.pgo — the profile is not picked up automatically for
// a library's test binary.
//
// PGO cannot change simulation results: it only steers inlining and code
// layout, the float64(...) roundings that forbid FMA fusion are semantic and
// survive any inlining decision, and TestNoFusedMultiplyAdd repeats its
// FMA-instruction scan with -pgo=default.pgo on every checked architecture
// to prove it.
package box2d
