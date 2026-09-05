package e2e_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// Seeded random scenes: the physics analogue of pkg/box2d's op-sequence fuzz. Each seed
// builds a scene of random but valid bodies over a thick static floor, simulates it, and
// checks invariants that hold for any such scene rather than behaviour someone scripted:
//
//   - every component survives the wire format unchanged;
//   - no solid dynamic body ends up inside or below the floor (Box2D sweeps every fast
//     body against static geometry, bullet or not, and speeds stay far below the clamp);
//   - the same seed simulates to the same world twice, and again at a different worker
//     count.
//
// Reproduce a failure with the TEST_SEED printed at startup and -run on the subtest name;
// the per-seed streams derive from that seed and the test name, so both are stable.

const (
	randomSceneSeeds  = 6
	randomSceneBodies = 40
	randomSceneTicks  = 300
	floorTop          = 0.0
	floorHalfHeight   = 5.0
	floorHalfWidth    = 250.0
	floorCategory     = uint64(1)
)

// bodySpec is one generated body. A scene is a fixed list of them, built once per seed
// so every replay spawns the same entities in the same order.
type bodySpec struct {
	label             string
	pb                physics.PhysicsBody2D
	x, y, vx, vy      float64
	solidAgainstFloor bool
}

var (
	specBodyTypes = []physics.BodyType{
		physics.BodyTypeStatic, physics.BodyTypeDynamic, physics.BodyTypeKinematic, physics.BodyTypeManual,
	}
	// specShapes are valid on any body type.
	specShapes = []physics.ShapeType{
		physics.ShapeTypeCircle, physics.ShapeTypeBox, physics.ShapeTypeConvexPolygon,
		physics.ShapeTypeCapsule,
	}
	// specLineShapes are, per the component docs, for static or kinematic bodies only.
	// The plugin does not enforce that (TestExhaustiveBodyMatrix pins the divergence), and
	// the consequence is not subtle: a dynamic body carrying an edge falls straight
	// through a 10 m static floor. So the generator respects the documented rule.
	specLineShapes = []physics.ShapeType{
		physics.ShapeTypeEdge, physics.ShapeTypeStaticChain, physics.ShapeTypeStaticChainLoop,
	}
)

func genScene(r *rand.Rand, n int) []bodySpec {
	specs := make([]bodySpec, 0, n)
	for i := range n {
		kind := specBodyTypes[r.IntN(len(specBodyTypes))]
		shapes := specShapes
		if kind != physics.BodyTypeDynamic {
			shapes = append(append([]physics.ShapeType(nil), specShapes...), specLineShapes...)
		}
		shape := scenario.SampleShape(shapes[r.IntN(len(shapes))])
		shape.Density = 0.5 + r.Float64()*4
		shape.Friction = r.Float64()
		shape.Restitution = r.Float64() * 0.8
		shape.IsSensor = r.IntN(6) == 0
		shape.CategoryBits = 1 << r.IntN(64)
		shape.MaskBits = r.Uint64()
		if r.IntN(2) == 0 {
			shape.MaskBits |= floorCategory
		}
		if kind == physics.BodyTypeKinematic {
			// A kinematic body moves at constant velocity with infinite mass, so one
			// heading downward can squeeze a dynamic body through the static floor.
			// That is correct Box2D behaviour, and it would make "stays above the
			// floor" unprovable, so kinematic bodies here collide with nothing; the
			// scripted bodytypes scenario covers kinematic pushing.
			shape.MaskBits = 0
		}
		shape.GroupIndex = int32(r.IntN(5) - 2)

		pb := physcomp.NewPhysicsBody2D(kind, shape)
		pb.Active = r.IntN(10) != 0
		pb.Awake = r.IntN(4) != 0
		pb.SleepingAllowed = r.IntN(4) != 0
		pb.Bullet = r.IntN(3) == 0
		pb.FixedRotation = r.IntN(3) == 0
		pb.GravityScale = r.Float64()*3 - 1
		pb.LinearDamping = r.Float64() * 2
		pb.AngularDamping = r.Float64() * 2

		s := bodySpec{
			label: fmt.Sprintf("b%02d_%s_%s", i, kindName[kind], shapeName[shape.ShapeType]),
			pb:    pb,
			x:     r.Float64()*80 - 40,
			y:     3 + r.Float64()*27,
		}
		if kind == physics.BodyTypeDynamic || kind == physics.BodyTypeKinematic {
			s.vx = r.Float64()*40 - 20
			s.vy = r.Float64()*40 - 20
		}
		// Only a solid, enabled dynamic body whose filter admits the floor is obliged
		// to stay above it: kinematic bodies pass through by design, sensors never
		// collide, and a mismatched mask means "not a floor as far as I'm concerned".
		s.solidAgainstFloor = kind == physics.BodyTypeDynamic && pb.Active && !shape.IsSensor &&
			shape.MaskBits&floorCategory != 0
		specs = append(specs, s)
	}
	return specs
}

// randomScene wraps a generated spec list as a harness scenario.
func randomScene(name string, specs []bodySpec) harness.Scenario {
	ids := make([]cardinal.EntityID, len(specs))
	return harness.Scenario{
		Name: name,
		Setup: func(c *harness.Ctx) {
			floor := physcomp.NewPhysicsBody2D(physics.BodyTypeStatic, physics.ColliderShape{
				ShapeType: physics.ShapeTypeBox,
				// Wide enough that no body can roll or fly off the end in the run's
				// five seconds and then fall for a legitimate reason.
				HalfExtents:  physics.Vec2{X: floorHalfWidth, Y: floorHalfHeight},
				Friction:     0.5,
				CategoryBits: floorCategory,
				MaskBits:     ^uint64(0),
			})
			c.Spawn("floor", 0, floorTop-floorHalfHeight, floor)

			for i, s := range specs {
				// The wire format must carry every field of every generated body.
				decoded, err := physics.PhysicsBody2D{}.UnmarshalWire(s.pb.MarshalWire())
				if c.NoError("wire decodes: "+s.label, err) {
					c.True("wire round-trip is lossless: "+s.label, reflect.DeepEqual(decoded, s.pb),
						"got %+v\nwant %+v", decoded, s.pb)
				}
				ids[i] = c.SpawnFull(s.label,
					physics.Transform2D{Position: physics.Vec2{X: s.x, Y: s.y}},
					physics.Velocity2D{Linear: physics.Vec2{X: s.vx, Y: s.vy}},
					s.pb)
			}
		},
		Steps: []harness.Step{
			{Tick: randomSceneTicks, Do: func(c *harness.Ctx) {
				for i, s := range specs {
					if !s.solidAgainstFloor {
						continue
					}
					// Centre stays above the floor top minus the largest sample-shape extent.
					if !c.GreaterEq("solid dynamic body did not tunnel through the floor: "+s.label,
						c.Pos(ids[i]).Y, floorTop-1.5) {
						c.Note("%s spawned at (%.2f, %.2f) v=(%.2f, %.2f) bullet=%v gravityScale=%.2f "+
							"density=%.2f restitution=%.2f, ended at (%.2f, %.2f)",
							s.label, s.x, s.y, s.vx, s.vy, s.pb.Bullet, s.pb.GravityScale,
							s.pb.Shapes[0].Density, s.pb.Shapes[0].Restitution,
							c.Pos(ids[i]).X, c.Pos(ids[i]).Y)
					}
				}
			}},
		},
	}
}

// sceneSeedEnv pins TestRandomScenes to one scene seed (hex, as in the subtest name),
// for reproducing a failure: the scene is a pure function of that seed alone.
const sceneSeedEnv = "PHYSICS2D_E2E_SCENE_SEED"

func randomSceneSeedList(t *testing.T) []uint64 {
	t.Helper()
	if pinned := os.Getenv(sceneSeedEnv); pinned != "" {
		seed, err := strconv.ParseUint(pinned, 16, 64)
		require.NoError(t, err, "%s must be a hex scene seed", sceneSeedEnv)
		return []uint64{seed}
	}
	root := testutils.NewRand(t)
	seeds := make([]uint64, randomSceneSeeds)
	for i := range seeds {
		seeds[i] = root.Uint64()
	}
	return seeds
}

func TestRandomScenes(t *testing.T) {
	t.Parallel()
	for _, seed := range randomSceneSeedList(t) {
		t.Run(fmt.Sprintf("seed_%016x", seed), func(t *testing.T) {
			t.Parallel()
			specs := genScene(rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), randomSceneBodies)

			digest := func(workers int) (int, uint64) {
				sc := randomScene("random", specs)
				runner, world, code := runSuite(t, []harness.Scenario{sc}, e2eConfig(t, workers))
				require.Zero(t, code, "invariants failed at workers=%d; reproduce with %s=%016x",
					workers, sceneSeedEnv, seed)
				return runner.Digest(world)
			}
			n0, h0 := digest(0)
			n0b, h0b := digest(0)
			n4, h4 := digest(4)
			require.Equal(t, n0, n0b)
			require.Equal(t, h0, h0b, "same seed simulated twice produced different worlds (scene seed %#016x)", seed)
			require.Equal(t, n0, n4)
			require.Equal(t, h0, h4, "worker count changed the result (scene seed %#016x)", seed)
		})
	}
}
