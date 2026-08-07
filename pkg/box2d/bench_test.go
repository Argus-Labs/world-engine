// Engine-only benchmarks for the float64 port of Box2D v3.2.0 (pkg/box2d).
// This file has no upstream counterpart.
//
// The scenes mirror the ECS-level suite in pkg/plugin/physics2d/test so the
// engine swap PR can compare like for like: whatever those numbers show, the
// difference against these is the wrapper overhead. Scene randomness reuses the
// fuzzer's seeded xorshift64 source (fuzz_ops_test.go) so every scene is
// reproducible and no benchmark depends on math/rand.

package box2d_test

import (
	"fmt"
	"math"
	"runtime"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	benchTimeStep     = 1.0 / 60.0
	benchSubStepCount = 4

	// benchPyramidRows yields benchPyramidRows*(benchPyramidRows+1)/2 == 210
	// stacked boxes.
	benchPyramidRows = 20

	// benchSettleSteps are run outside the timed loop so the measured steps
	// operate on a steady contact state rather than on free fall.
	benchSettleSteps = 30

	// benchQueryShapeCount is the shape count of the shared query scene used
	// by the ray cast and overlap benchmarks.
	benchQueryShapeCount = 1000

	// benchChurnCount is the number of bodies created and destroyed per
	// iteration of BenchmarkBodyCreateDestroy.
	benchChurnCount = 1000
)

// benchBodyCounts mirrors the body counts swept by the physics2d suite.
var benchBodyCounts = [4]int{100, 500, 1000, 5000}

// ---------------------------------------------------------------------------
// Shared scene construction
// ---------------------------------------------------------------------------

// newBenchWorld creates a gravity world with sleeping disabled. A step
// benchmark re-steps one world for the whole timed loop, which is thousands of
// simulated seconds: with sleeping on, every pile is asleep within the first
// few percent of the run and the benchmark reports the cost of an empty awake
// set (~15 ns/op) instead of solver throughput. Sleeping stays enabled in the
// determinism fuzzer, which does exercise the sleep and wake paths.
//
// workerCount is passed to WorldDef.WorkerCount (0/1 = serial). Results are
// byte-identical for every value; the step benchmarks sweep it to measure the
// internal worker pool (worker_pool.go), and the Workers_1 rows double as the
// zero-overhead regression check for the serial path (pool == nil).
func newBenchWorld(workerCount int) *box2d.World {
	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{X: 0.0, Y: -10.0}
	worldDef.EnableSleep = false
	worldDef.WorkerCount = workerCount

	return box2d.NewWorld(&worldDef)
}

// benchWorkerCounts returns the worker counts the step benchmarks sweep:
// serial plus the machine's full core count when it has more than one.
func benchWorkerCounts() []int {
	counts := []int{1}
	if n := runtime.NumCPU(); n > 1 {
		counts = append(counts, n)
	}

	return counts
}

// addBenchGround adds a static box spanning [-halfWidth, halfWidth] whose top
// surface sits at y == 0.
func addBenchGround(world *box2d.World, halfWidth float64) box2d.BodyID {
	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Position = box2d.Vec2{X: 0.0, Y: -1.0}
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Material.Friction = 0.6
	ground := box2d.MakeBox(halfWidth, 1.0)
	world.CreatePolygonShape(bodyID, &shapeDef, &ground)

	return bodyID
}

// addBenchChain adds an open static chain forming a shallow bumpy strip just
// above the ground box. Points run from +x to -x: the chain normal points to
// the right of the segment direction, so descending x puts the collidable side
// up.
func addBenchChain(world *box2d.World, bodyID box2d.BodyID, halfWidth float64, segments int) {
	points := make([]box2d.Vec2, segments+1)
	step := 2.0 * halfWidth / float64(segments)

	for i := range points {
		x := halfWidth - float64(float64(i)*step)
		points[i] = box2d.Vec2{X: x, Y: 0.25 * math.Sin(0.25*x)}
	}

	chainDef := box2d.DefaultChainDef()
	chainDef.Points = points
	world.CreateChain(bodyID, &chainDef)
}

// addBenchMixedBody adds one dynamic body carrying a single circle, box or
// capsule, cycling shape type by index so every narrow-phase pair type is
// exercised.
func addBenchMixedBody(world *box2d.World, source opSource, index int, position box2d.Vec2) {
	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyDef.Position = position
	bodyDef.Rotation = box2d.MakeRot(srcRange(source, -box2d.Pi, box2d.Pi))
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Density = 1.0
	shapeDef.Material.Friction = 0.3
	shapeDef.Material.Restitution = 0.2

	switch index % 3 {
	case 0:
		circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}
		world.CreateCircleShape(bodyID, &shapeDef, &circle)
	case 1:
		square := box2d.MakeSquare(0.5)
		world.CreatePolygonShape(bodyID, &shapeDef, &square)
	default:
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.25, Y: 0.0},
			Center2: box2d.Vec2{X: 0.25, Y: 0.0},
			Radius:  0.35,
		}
		world.CreateCapsuleShape(bodyID, &shapeDef, &capsule)
	}
}

// buildBenchPyramidScene stacks rows*(rows+1)/2 boxes into a pyramid on a static
// ground, following the upstream b2 sample layout.
func buildBenchPyramidScene(rows, workerCount int) *box2d.World {
	world := newBenchWorld(workerCount)
	addBenchGround(world, 100.0)

	const halfExtent = 0.5

	shift := halfExtent
	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Density = 1.0
	shapeDef.Material.Friction = 0.6
	box := box2d.MakeSquare(halfExtent)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody

	for i := range rows {
		y := float64((2.0*float64(i)+1.0)*shift) + halfExtent

		for j := i; j < rows; j++ {
			x := float64((float64(i)+1.0)*shift) + float64(float64(2.0*float64(j-i))*shift) - float64(float64(rows)*halfExtent)

			bodyDef.Position = box2d.Vec2{X: x, Y: y}
			bodyID := world.CreateBody(&bodyDef)
			world.CreatePolygonShape(bodyID, &shapeDef, &box)
		}
	}

	return world
}

// buildBenchMixedRainScene drops bodyCount mixed shapes onto a ground box plus a
// static chain and settles them for benchSettleSteps steps.
func buildBenchMixedRainScene(bodyCount, workerCount int) *box2d.World {
	world := newBenchWorld(workerCount)
	ground := addBenchGround(world, 200.0)
	addBenchChain(world, ground, 190.0, 64)

	source := newSeedSource(0x5eed5eed5eed5eed)
	cols := int(math.Ceil(math.Sqrt(float64(bodyCount))))

	for i := range bodyCount {
		position := box2d.Vec2{
			X: float64(i%cols)*2.0 - float64(cols),
			Y: float64(i/cols)*2.0 + 3.0,
		}
		addBenchMixedBody(world, source, i, position)
	}

	for range benchSettleSteps {
		world.Step(benchTimeStep, benchSubStepCount)
	}

	return world
}

// buildBenchJointedScene builds a 100-link revolute chain hanging from a static
// anchor plus 50 distance springs tying every other link back to the anchor
// body.
func buildBenchJointedScene(workerCount int) *box2d.World {
	const (
		linkCount   = 100
		springCount = 50
		linkHalf    = 0.4
		linkSpacing = 1.0
	)

	world := newBenchWorld(workerCount)

	anchorDef := box2d.DefaultBodyDef()
	anchorDef.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	anchor := world.CreateBody(&anchorDef)

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Density = 1.0
	shapeDef.Material.Friction = 0.2
	// The links must not collide with each other; a single negative group
	// index disables the whole chain's self-collision.
	shapeDef.Filter.GroupIndex = -1
	link := box2d.MakeBox(linkHalf, 0.125)

	links := make([]box2d.BodyID, 0, linkCount)
	previous := anchor

	for i := range linkCount {
		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Type = box2d.DynamicBody
		bodyDef.Position = box2d.Vec2{X: linkSpacing*float64(i) + linkHalf, Y: 0.0}
		bodyID := world.CreateBody(&bodyDef)
		world.CreatePolygonShape(bodyID, &shapeDef, &link)

		pivot := box2d.Vec2{X: linkSpacing * float64(i), Y: 0.0}
		revolute := box2d.DefaultRevoluteJointDef()
		revolute.Base.BodyIDA = previous
		revolute.Base.BodyIDB = bodyID
		revolute.Base.LocalFrameA = box2d.Transform{
			P: world.BodyLocalPoint(previous, pivot),
			Q: box2d.RotIdentity,
		}
		revolute.Base.LocalFrameB = box2d.Transform{
			P: world.BodyLocalPoint(bodyID, pivot),
			Q: box2d.RotIdentity,
		}
		world.CreateRevoluteJoint(&revolute)

		links = append(links, bodyID)
		previous = bodyID
	}

	for k := range springCount {
		bodyID := links[2*k]
		attach := box2d.Vec2{X: linkSpacing * float64(2*k), Y: 4.0}

		spring := box2d.DefaultDistanceJointDef()
		spring.Base.BodyIDA = anchor
		spring.Base.BodyIDB = bodyID
		spring.Base.LocalFrameA = box2d.Transform{
			P: world.BodyLocalPoint(anchor, attach),
			Q: box2d.RotIdentity,
		}
		spring.Base.LocalFrameB = box2d.Transform{P: box2d.Vec2{}, Q: box2d.RotIdentity}
		spring.Length = 4.0
		spring.EnableSpring = true
		spring.Hertz = 2.0
		spring.DampingRatio = 0.5
		world.CreateDistanceJoint(&spring)
	}

	for range benchSettleSteps {
		world.Step(benchTimeStep, benchSubStepCount)
	}

	return world
}

// buildBenchQueryScene lays out benchQueryShapeCount static circles on a grid,
// matching the physics2d gridSpawnSystem layout (radius 1, spacing 3).
func buildBenchQueryScene() *box2d.World {
	world := newBenchWorld(1)

	cols := int(math.Ceil(math.Sqrt(float64(benchQueryShapeCount))))
	const spacing = 3.0

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Material.Friction = 0.3
	circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 1.0}

	bodyDef := box2d.DefaultBodyDef()

	for i := range benchQueryShapeCount {
		bodyDef.Position = box2d.Vec2{
			X: float64(float64(i%cols)*spacing) - float64(float64(cols)*spacing/2.0),
			Y: float64(float64(i/cols)*spacing) - float64(float64(cols)*spacing/2.0),
		}
		bodyID := world.CreateBody(&bodyDef)
		world.CreateCircleShape(bodyID, &shapeDef, &circle)
	}

	// One step builds the static tree and the broad-phase pairs.
	world.Step(benchTimeStep, benchSubStepCount)

	return world
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

// BenchmarkStepPyramid measures solver throughput on a deep, well-conditioned
// box stack (210 boxes, 20 rows), at WorkerCount 1 and the machine's core
// count.
func BenchmarkStepPyramid(b *testing.B) {
	for _, workerCount := range benchWorkerCounts() {
		b.Run(fmt.Sprintf("Workers_%d", workerCount), func(b *testing.B) {
			b.ReportAllocs()

			world := buildBenchPyramidScene(benchPyramidRows, workerCount)
			defer world.Destroy()

			for range benchSettleSteps {
				world.Step(benchTimeStep, benchSubStepCount)
			}

			b.ResetTimer()

			for range b.N {
				world.Step(benchTimeStep, benchSubStepCount)
			}
		})
	}
}

// BenchmarkStepMixedRain measures broad-phase, narrow-phase and solver
// throughput on mixed shapes resting on a ground box plus a static chain, at
// WorkerCount 1 and the machine's core count.
func BenchmarkStepMixedRain(b *testing.B) {
	for _, bodyCount := range benchBodyCounts {
		b.Run(fmt.Sprintf("Bodies_%d", bodyCount), func(b *testing.B) {
			for _, workerCount := range benchWorkerCounts() {
				b.Run(fmt.Sprintf("Workers_%d", workerCount), func(b *testing.B) {
					b.ReportAllocs()

					world := buildBenchMixedRainScene(bodyCount, workerCount)
					defer world.Destroy()

					b.ResetTimer()

					for range b.N {
						world.Step(benchTimeStep, benchSubStepCount)
					}
				})
			}
		})
	}
}

// BenchmarkStepJointed measures the joint solver on a 100-link revolute chain
// carrying 50 distance springs, at WorkerCount 1 and the machine's core
// count.
func BenchmarkStepJointed(b *testing.B) {
	for _, workerCount := range benchWorkerCounts() {
		b.Run(fmt.Sprintf("Workers_%d", workerCount), func(b *testing.B) {
			b.ReportAllocs()

			world := buildBenchJointedScene(workerCount)
			defer world.Destroy()

			b.ResetTimer()

			for range b.N {
				world.Step(benchTimeStep, benchSubStepCount)
			}
		})
	}
}

// BenchmarkRayCastClosest fires a full-width ray across the 1000-shape query
// scene once per iteration.
func BenchmarkRayCastClosest(b *testing.B) {
	b.ReportAllocs()

	world := buildBenchQueryScene()
	defer world.Destroy()

	origin := box2d.Vec2{X: -500.0, Y: 0.0}
	translation := box2d.Vec2{X: 1000.0, Y: 0.0}
	filter := box2d.DefaultQueryFilter()

	hits := 0

	b.ResetTimer()

	for range b.N {
		if world.CastRayClosest(origin, translation, filter).Hit {
			hits++
		}
	}

	b.StopTimer()

	if hits != b.N {
		b.Fatalf("ray missed the scene: %d hits out of %d casts", hits, b.N)
	}
}

// BenchmarkOverlapAABB queries a 20x20 box out of the same 1000-shape scene.
func BenchmarkOverlapAABB(b *testing.B) {
	b.ReportAllocs()

	world := buildBenchQueryScene()
	defer world.Destroy()

	aabb := box2d.AABB{
		LowerBound: box2d.Vec2{X: -10.0, Y: -10.0},
		UpperBound: box2d.Vec2{X: 10.0, Y: 10.0},
	}
	filter := box2d.DefaultQueryFilter()

	overlaps := 0
	collect := func(_ box2d.ShapeID, _ any) bool {
		overlaps++

		return true
	}

	b.ResetTimer()

	for range b.N {
		world.OverlapAABB(aabb, filter, collect, nil)
	}

	b.StopTimer()

	if overlaps == 0 {
		b.Fatal("overlap query matched no shapes")
	}
}

// BenchmarkBodyCreateDestroy churns benchChurnCount single-shape dynamic
// bodies per iteration, measuring allocation and id-pool recycling cost rather
// than simulation cost.
func BenchmarkBodyCreateDestroy(b *testing.B) {
	b.ReportAllocs()

	world := newBenchWorld(1)
	defer world.Destroy()

	addBenchGround(world, 100.0)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Density = 1.0
	circle := box2d.Circle{Center: box2d.Vec2{}, Radius: 0.5}

	bodies := make([]box2d.BodyID, 0, benchChurnCount)

	b.ResetTimer()

	for range b.N {
		for i := range benchChurnCount {
			bodyDef.Position = box2d.Vec2{X: float64(float64(i%50)*1.5) - 37.5, Y: float64(float64(i/50)*1.5) + 2.0}
			bodyID := world.CreateBody(&bodyDef)
			world.CreateCircleShape(bodyID, &shapeDef, &circle)
			bodies = append(bodies, bodyID)
		}

		for _, bodyID := range bodies {
			world.DestroyBody(bodyID)
		}

		bodies = bodies[:0]
	}
}
