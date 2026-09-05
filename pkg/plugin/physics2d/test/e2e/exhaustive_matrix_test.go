package e2e_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/scenario"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/require"
)

// Exhaustive body matrix: every body type x every shape type x every combination of
// the five body flags, all in one zero-gravity world. For each combination it checks
// that the flags actually reached the engine (read back through Engine(), not the
// component, which is exactly where a flag that never crossed still looks right),
// that the component survives the wire format, and that a full rebuild through
// Plugin.Reset recreates the body with the same engine state. This is the
// enumerated form of the question "does Go's false-by-default leak anywhere C
// defaults to true?" — testutils.Gen walks every case, so no permutation is
// left to a hand-picked sample.

var (
	matrixKinds = []physics.BodyType{
		physics.BodyTypeStatic, physics.BodyTypeDynamic, physics.BodyTypeKinematic, physics.BodyTypeManual,
	}
	matrixShapes = []physics.ShapeType{
		physics.ShapeTypeCircle, physics.ShapeTypeBox, physics.ShapeTypeConvexPolygon,
		physics.ShapeTypeStaticChain, physics.ShapeTypeStaticChainLoop,
		physics.ShapeTypeEdge, physics.ShapeTypeCapsule,
	}
	kindName = map[physics.BodyType]string{
		physics.BodyTypeStatic: "static", physics.BodyTypeDynamic: "dynamic",
		physics.BodyTypeKinematic: "kinematic", physics.BodyTypeManual: "manual",
	}
	shapeName = map[physics.ShapeType]string{
		physics.ShapeTypeCircle: "circle", physics.ShapeTypeBox: "box",
		physics.ShapeTypeConvexPolygon: "polygon", physics.ShapeTypeStaticChain: "chain",
		physics.ShapeTypeStaticChainLoop: "chainloop", physics.ShapeTypeEdge: "edge",
		physics.ShapeTypeCapsule: "capsule",
	}
	// engineKind is the Box2D body type each ECS kind must map to. Manual bodies are
	// kinematic in the engine; the ECS layer owns their position.
	engineKind = map[physics.BodyType]box2d.BodyType{
		physics.BodyTypeStatic:    box2d.StaticBody,
		physics.BodyTypeDynamic:   box2d.DynamicBody,
		physics.BodyTypeKinematic: box2d.KinematicBody,
		physics.BodyTypeManual:    box2d.KinematicBody,
	}
)

type matrixCombo struct {
	kind                                   physics.BodyType
	shape                                  physics.ShapeType
	active, awake, sleep, bullet, fixedRot bool
}

func (m matrixCombo) label() string {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	return fmt.Sprintf("%s/%s/active%d-awake%d-sleep%d-bullet%d-fixed%d",
		kindName[m.kind], shapeName[m.shape], b(m.active), b(m.awake), b(m.sleep), b(m.bullet), b(m.fixedRot))
}

func (m matrixCombo) body() physics.PhysicsBody2D {
	pb := physcomp.NewPhysicsBody2D(m.kind, scenario.SampleShape(m.shape))
	pb.Active = m.active
	pb.Awake = m.awake
	pb.SleepingAllowed = m.sleep
	pb.Bullet = m.bullet
	pb.FixedRotation = m.fixedRot
	return pb
}

func (m matrixCombo) lineShape() bool {
	return m.shape == physics.ShapeTypeStaticChain ||
		m.shape == physics.ShapeTypeStaticChainLoop || m.shape == physics.ShapeTypeEdge
}

// expectBody is the documented rule for which combinations the plugin accepts:
// chains and edges belong on static or kinematic bodies, never dynamic ones.
func (m matrixCombo) expectBody() bool {
	return !m.lineShape() || m.kind != physics.BodyTypeDynamic
}

// knownDivergence names a combination the plugin accepts today although the
// documented rule says it should not. The matrix asserts today's behaviour for these,
// so enforcing the rule flips the check and the entry gets removed on purpose rather
// than the mismatch being absorbed silently.
func (m matrixCombo) knownDivergence() (string, bool) {
	if m.lineShape() && m.kind == physics.BodyTypeDynamic {
		return "a chain or edge on a dynamic body is accepted although the component docs " +
			"say static or kinematic only; TestRandomScenes shows why the rule exists " +
			"(a dynamic edge body falls through a 10 m static floor)", true
	}
	return "", false
}

// expectAwake is Box2D's creation rule, (isAwake || !enableSleep) && isEnabled, which
// also holds after a rebuild because nothing in this world moves or touches.
func (m matrixCombo) expectAwake() bool {
	return (m.awake || !m.sleep) && m.active
}

func allMatrixCombos() []matrixCombo {
	var combos []matrixCombo
	g := testutils.NewGen()
	for !g.Done() {
		combos = append(combos, matrixCombo{
			kind:     testutils.Pick(g, matrixKinds),
			shape:    testutils.Pick(g, matrixShapes),
			active:   g.Bool(),
			awake:    g.Bool(),
			sleep:    g.Bool(),
			bullet:   g.Bool(),
			fixedRot: g.Bool(),
		})
	}
	return combos
}

// matrixScene spawns every combination on a grid and checks the engine twice: once
// after creation, once after a Plugin.Reset rebuild.
func matrixScene(combos []matrixCombo) harness.Scenario {
	ids := make([]cardinal.EntityID, len(combos))
	const (
		perRow  = 32
		spacing = 4.0
	)
	checkEngine := func(c *harness.Ctx, phase string) {
		eng := c.Plugin().Engine()
		for i, m := range combos {
			label := phase + ": " + m.label()
			bodyID, ok := c.Plugin().BodyID(ids[i])
			want, detail := m.expectBody(), "has body=%v, documented rule says %v"
			if reason, known := m.knownDivergence(); known {
				want = true
				detail = "has body=%v, want %v: the known divergence is gone (" + reason +
					"), so remove it from knownDivergence"
			}
			if !c.True(label+": body existence", ok == want, detail, ok, want) || !ok {
				continue
			}
			c.True(label+": body type reached the engine", eng.BodyType(bodyID) == engineKind[m.kind],
				"engine type %v, want %v", eng.BodyType(bodyID), engineKind[m.kind])
			c.True(label+": Active reached the engine", eng.IsBodyEnabled(bodyID) == m.active,
				"engine enabled=%v, want %v", eng.IsBodyEnabled(bodyID), m.active)
			c.True(label+": SleepingAllowed reached the engine", eng.IsBodySleepEnabled(bodyID) == m.sleep,
				"engine sleep enabled=%v, want %v", eng.IsBodySleepEnabled(bodyID), m.sleep)
			c.True(label+": Bullet reached the engine", eng.IsBodyBullet(bodyID) == m.bullet,
				"engine bullet=%v, want %v", eng.IsBodyBullet(bodyID), m.bullet)
			c.True(label+": FixedRotation reached the engine", eng.BodyMotionLocks(bodyID).AngularZ == m.fixedRot,
				"engine angular lock=%v, want %v", eng.BodyMotionLocks(bodyID).AngularZ, m.fixedRot)
			if m.kind != physics.BodyTypeStatic {
				// Static bodies are never awake in Box2D, whatever the flag says.
				c.True(label+": Awake follows Box2D's creation rule", eng.IsBodyAwake(bodyID) == m.expectAwake(),
					"engine awake=%v, want (awake||!sleep)&&active = %v", eng.IsBodyAwake(bodyID), m.expectAwake())
				// The post-step mirror covers dynamic and kinematic bodies only. Manual
				// bodies are ECS-owned and skipped by writeback, so their component keeps
				// its creation-time Awake even where Box2D overrode it (Awake=false with
				// SleepingAllowed=false is forced awake) — a documented gap, pinned here
				// by its absence from the check.
				if m.active && m.kind != physics.BodyTypeManual {
					c.True(label+": component Awake mirrors the engine", c.Body(ids[i]).Awake == eng.IsBodyAwake(bodyID),
						"component awake=%v, engine awake=%v", c.Body(ids[i]).Awake, eng.IsBodyAwake(bodyID))
				}
			}
		}
	}
	return harness.Scenario{
		Name: "matrix",
		Setup: func(c *harness.Ctx) {
			for i, m := range combos {
				pb := m.body()
				decoded, err := physics.PhysicsBody2D{}.UnmarshalWire(pb.MarshalWire())
				if c.NoError("wire decodes: "+m.label(), err) {
					c.True("wire round-trip is lossless: "+m.label(), reflect.DeepEqual(decoded, pb),
						"got %+v\nwant %+v", decoded, pb)
				}
				ids[i] = c.Spawn(m.label(), float64(i%perRow)*spacing, float64(i/perRow)*spacing, pb)
			}
		},
		Steps: []harness.Step{
			{Tick: 2, Do: func(c *harness.Ctx) { checkEngine(c, "created") }},
			{Tick: 4, Do: func(c *harness.Ctx) {
				c.ExpectWorldReset()
				c.Plugin().Reset()
			}},
			// Tick 5 rebuilds without stepping; 6 is the first step and mirror after it.
			{Tick: 8, Do: func(c *harness.Ctx) { checkEngine(c, "after Reset rebuild") }},
		},
	}
}

func TestExhaustiveBodyMatrix(t *testing.T) {
	t.Parallel()
	combos := allMatrixCombos()
	require.Len(t, combos, len(matrixKinds)*len(matrixShapes)*32, "generator did not walk every combination")

	cfg := e2eConfig(t, 0)
	cfg.Gravity = physics.Vec2{} // nothing moves, so awake state is exactly the creation rule
	runner, _, code := runSuite(t, []harness.Scenario{matrixScene(combos)}, cfg)
	pass, fail, _ := runner.Report().Totals()
	t.Logf("%d combinations, %d checks pass, %d fail", len(combos), pass, fail)
	require.Zero(t, code, "%d check(s) failed", fail)
}
