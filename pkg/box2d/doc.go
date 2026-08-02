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
//   - Execution is single-threaded; the upstream task system is not ported.
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
//   - No Go maps, goroutines, or time sources in the simulation path;
//     iteration orders are explicit and match upstream data structures.
//   - Exactly one sort runs in the simulation path: the per-sensor overlap
//     sort in sensor.go (upstream sorts the same array with qsort). Its
//     comparator is a total order over the (shapeID, generation) visitor key,
//     so the unstable sort still produces a unique result. No other code in
//     the step may sort.
//
// Cross-architecture equivalence is enforced in CI by golden-trace hash
// tests (see testdata/golden).
package box2d
