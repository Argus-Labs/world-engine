// Oracle tests for the debug draw port: the b2DrawShape / DrawQueryCallback /
// b2World_Draw section of src/physics_world.c and b2DrawJoint plus the
// per-joint b2Draw*Joint functions of src/joint.c, src/distance_joint.c,
// src/prismatic_joint.c, src/revolute_joint.c, src/weld_joint.c and
// src/wheel_joint.c.
//
// Every expected color and primitive count below is read off the C source, not
// off this Go port. The colour ladder itself is pinned down rung by rung in
// oracle_misc_internal_test.go (TestOracleShapeDrawColorLadder); the tests here
// drive the same ladder through real world state so the wiring between body
// bookkeeping and the ladder is covered too.
//
// The drawRecorder helper comes from draw_test.go in the same test package.

package box2d_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// oracleDrawColors returns the set of colors of every solid shape primitive
// recorded, so a test can assert on the color a body was drawn with without
// depending on the traversal order of the broad-phase trees.
func oracleDrawColors(r *drawRecorder) []box2d.HexColor {
	colors := make([]box2d.HexColor, 0,
		len(r.solidPolygons)+len(r.solidCircles)+len(r.solidCapsules)+len(r.lines))
	for _, record := range r.solidPolygons {
		colors = append(colors, record.color)
	}
	for _, record := range r.solidCircles {
		colors = append(colors, record.color)
	}
	for _, record := range r.solidCapsules {
		colors = append(colors, record.color)
	}
	for _, record := range r.lines {
		colors = append(colors, record.color)
	}
	return colors
}

// oracleDrawShapes renders only the shapes of a world and returns the recorder.
func oracleDrawShapes(w *box2d.World) *drawRecorder {
	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawShapes = true
	w.Draw(&draw)
	return &recorder
}

// TestOracleDrawColorByBodyState drives the C colour ladder
// (src/physics_world.c:959-1009) through real world state. Each sub-case builds
// exactly one visible shape so the recorded colour is unambiguous.
func TestOracleDrawColorByBodyState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		build func(t *testing.T, w *box2d.World)
		want  box2d.HexColor
	}{
		{
			// C 994: b2_staticBody -> b2_colorPaleGreen.
			name: "static body",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorPaleGreen,
		},
		{
			// C 998: b2_kinematicBody -> b2_colorRoyalBlue. A kinematic body
			// has zero mass, so this also proves the rung 965 "bad body" test
			// is restricted to dynamic bodies.
			name: "kinematic body",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyDef.Type = box2d.KinematicBody
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorRoyalBlue,
		},
		{
			// C 1002: an awake dynamic body with mass -> b2_colorPink.
			name: "awake dynamic body",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyDef.Type = box2d.DynamicBody
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorPink,
		},
		{
			// C 965: a dynamic body whose mass is zero is the "bad body" case
			// -> b2_colorRed. Zero density gives zero mass.
			name: "dynamic body with zero mass",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyDef.Type = box2d.DynamicBody
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				shapeDef.Density = 0.0
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorRed,
		},
		{
			// C 974: shape->sensorIndex != B2_NULL_INDEX -> b2_colorWheat. The
			// sensor sits on a static body, so this also proves rung 974 beats
			// rung 994.
			name: "sensor shape",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				shapeDef.IsSensor = true
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorWheat,
		},
		{
			// C 961: a non-zero customColor short circuits the whole ladder,
			// even on a sensor shape (rung 974) attached to a static body.
			name: "custom color",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				shapeDef.IsSensor = true
				shapeDef.Material.CustomColor = uint32(box2d.ColorMagenta)
				box := box2d.MakeBox(1.0, 1.0)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorMagenta,
		},
		{
			// C 982: (bodySim->flags & b2_isBullet) && setIndex == b2_awakeSet
			// -> b2_colorTurquoise. b2Body_SetBullet sets the sim flag
			// permanently, so a resting awake bullet is enough.
			name: "awake bullet",
			build: func(_ *testing.T, w *box2d.World) {
				bodyDef := box2d.DefaultBodyDef()
				bodyDef.Type = box2d.DynamicBody
				bodyDef.IsBullet = true
				bodyID := w.CreateBody(&bodyDef)
				shapeDef := box2d.DefaultShapeDef()
				box := box2d.MakeBox(0.25, 0.25)
				w.CreatePolygonShape(bodyID, &shapeDef, &box)
			},
			want: box2d.ColorTurquoise,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			worldDef := box2d.DefaultWorldDef()
			worldDef.Gravity = box2d.Vec2{}
			world := box2d.NewWorld(&worldDef)
			t.Cleanup(world.Destroy)

			test.build(t, world)

			recorder := oracleDrawShapes(world)
			colors := oracleDrawColors(recorder)
			require.Len(t, colors, 1, "the scene must contain exactly one visible shape")
			assert.Equal(t, test.want, colors[0])
		})
	}
}

// TestOracleDrawColorSleepingDynamicIsGray drives the final `else` of the C
// ladder (src/physics_world.c:1006): a dynamic body that has fallen asleep is
// no longer in b2_awakeSet, so none of the earlier rungs apply.
func TestOracleDrawColorSleepingDynamicIsGray(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.5, 0.5)
	world.CreatePolygonShape(bodyID, &shapeDef, &box)

	// With no gravity and no velocity the body accumulates sleep time and
	// leaves the awake set after b2_timeToSleep (0.5 s).
	for range 60 {
		world.Step(1.0/60.0, 4)
	}
	require.False(t, world.IsBodyAwake(bodyID), "the body must fall asleep for this case")

	colors := oracleDrawColors(oracleDrawShapes(world))
	require.Len(t, colors, 1)
	assert.Equal(t, box2d.ColorGray, colors[0])
}

// TestOracleDrawColorSpeedCappedIsYellow drives rung 986 of the C ladder
// (`body->flags & b2_isSpeedCapped`). b2IntegrateVelocitiesTask
// (src/solver.c) sets b2_isSpeedCapped when the integrated velocity exceeds
// the world maximum linear speed, and b2FinalizeBodiesTask copies the sim flag
// onto the body for debug draw.
//
// A tiny maximum linear speed plus a large gravity guarantees the clamp on the
// very first step, while keeping the body slow enough that it is neither fast
// (rung 990) nor a time-of-impact body (rung 978).
func TestOracleDrawColorSpeedCappedIsYellow(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{X: 0.0, Y: -1000.0}
	worldDef.MaximumLinearSpeed = 0.5
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.5, 0.5)
	world.CreatePolygonShape(bodyID, &shapeDef, &box)

	world.Step(1.0/60.0, 4)

	// C: the clamp caps |v| at exactly the world maximum.
	velocity := world.BodyLinearVelocity(bodyID)
	assert.InDelta(t, 0.5, box2d.Length(velocity), 1e-9)

	colors := oracleDrawColors(oracleDrawShapes(world))
	require.Len(t, colors, 1)
	assert.Equal(t, box2d.ColorYellow, colors[0])
}

// TestOracleDrawColorFastIsSalmon drives rung 990 of the C ladder
// (`bodySim->flags & b2_isFast`). b2FinalizeBodiesTask (src/solver.c) sets
// b2_isFast on a dynamic body when
//
//	maxVelocity * timeStep > 0.5 * sim->minExtent
//
// and continuous collision is enabled. A small, fast, non-bullet body in an
// otherwise empty world satisfies that without being speed capped (rung 986)
// and without any time of impact (rung 978).
func TestOracleDrawColorFastIsSalmon(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	// 3 m/s at 1/60 s is 0.05 m per step, more than half the 0.05 m extent of
	// the shape below, and well under the 400 m/s default speed cap.
	bodyDef.LinearVelocity = box2d.Vec2{X: 3.0, Y: 0.0}
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Radius: 0.05}
	world.CreateCircleShape(bodyID, &shapeDef, &circle)

	world.Step(1.0/60.0, 4)

	colors := oracleDrawColors(oracleDrawShapes(world))
	require.Len(t, colors, 1)
	assert.Equal(t, box2d.ColorSalmon, colors[0])
}

// TestOracleDrawColorTimeOfImpactIsLime drives rung 978 of the C ladder
// (`body->flags & b2_hadTimeOfImpact`). b2ContinuousQueryCallback
// (src/solver_continuous.c) sets b2_hadTimeOfImpact on the fast body sim when
// a sweep finds an impact, and the following b2FinalizeBodiesTask copies the
// flag onto the body, so the flag is visible one step after the impact.
func TestOracleDrawColorTimeOfImpactIsLime(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	// A thin static wall at x = 0 that a fast small body would tunnel through
	// without continuous collision.
	wallDef := box2d.DefaultBodyDef()
	wallID := world.CreateBody(&wallDef)
	wallShapeDef := box2d.DefaultShapeDef()
	wall := box2d.MakeBox(0.05, 5.0)
	world.CreatePolygonShape(wallID, &wallShapeDef, &wall)

	bulletDef := box2d.DefaultBodyDef()
	bulletDef.Type = box2d.DynamicBody
	bulletDef.Position = box2d.Vec2{X: -2.0, Y: 0.0}
	bulletDef.LinearVelocity = box2d.Vec2{X: 60.0, Y: 0.0}
	bulletID := world.CreateBody(&bulletDef)
	bulletShapeDef := box2d.DefaultShapeDef()
	pellet := box2d.Circle{Radius: 0.05}
	world.CreateCircleShape(bulletID, &bulletShapeDef, &pellet)

	// Step until the body has a recorded time of impact. 60 m/s covers 1 m per
	// step, so the wall is reached within a couple of steps; the extra steps
	// let b2FinalizeBodiesTask publish the flag onto the body.
	found := false
	for range 6 {
		world.Step(1.0/60.0, 4)

		for _, color := range oracleDrawColors(oracleDrawShapes(world)) {
			if color == box2d.ColorLime {
				found = true
			}
		}
		if found {
			break
		}
	}

	assert.True(t, found, "a tunnelling body must be drawn with b2_colorLime after its time of impact")
}

// oracleJointDrawWorld builds a ground body plus a dynamic box and returns
// both ids, ready for a joint.
func oracleJointDrawWorld(t *testing.T) (*box2d.World, box2d.BodyID, box2d.BodyID) {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	groundDef := box2d.DefaultBodyDef()
	groundID := world.CreateBody(&groundDef)

	boxDef := box2d.DefaultBodyDef()
	boxDef.Type = box2d.DynamicBody
	boxDef.Position = box2d.Vec2{X: 2.0, Y: 0.0}
	boxID := world.CreateBody(&boxDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.25, 0.25)
	world.CreatePolygonShape(boxID, &shapeDef, &box)

	return world, groundID, boxID
}

// oracleDrawJoints renders only the joints of a world.
func oracleDrawJoints(w *box2d.World, extras bool) *drawRecorder {
	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawJoints = true
	draw.DrawJointExtras = extras
	w.Draw(&draw)
	return &recorder
}

// oracleLineColors returns the colours of every recorded line primitive.
func oracleLineColors(r *drawRecorder) []box2d.HexColor {
	colors := make([]box2d.HexColor, 0, len(r.lines))
	for _, record := range r.lines {
		colors = append(colors, record.color)
	}
	return colors
}

// oraclePointColors returns the colours of every recorded point primitive.
func oraclePointColors(r *drawRecorder) []box2d.HexColor {
	colors := make([]box2d.HexColor, 0, len(r.points))
	for _, record := range r.points {
		colors = append(colors, record.color)
	}
	return colors
}

// TestOracleDrawFilterJoint pins down the b2_filterJoint arm of b2DrawJoint,
// src/joint.c:1459-1462:
//
//	draw->DrawLineFcn( pA, pB, b2_colorGold, draw->context );
//
// Exactly one gold line, nothing else.
func TestOracleDrawFilterJoint(t *testing.T) {
	t.Parallel()

	world, groundID, boxID := oracleJointDrawWorld(t)

	def := box2d.DefaultFilterJointDef()
	def.Base.BodyIDA = groundID
	def.Base.BodyIDB = boxID
	world.CreateFilterJoint(&def)

	recorder := oracleDrawJoints(world, false)

	assert.Equal(t, []box2d.HexColor{box2d.ColorGold}, oracleLineColors(recorder))
	assert.Empty(t, recorder.points)
	assert.Empty(t, recorder.circles)
	assert.Empty(t, recorder.polygons)
}

// TestOracleDrawMotorJoint pins down the b2_motorJoint arm of b2DrawJoint,
// src/joint.c: a yellow-green point at frame A, a plum point at frame B and a
// light gray line between them, all at point size 8.
func TestOracleDrawMotorJoint(t *testing.T) {
	t.Parallel()

	world, groundID, boxID := oracleJointDrawWorld(t)

	def := box2d.DefaultMotorJointDef()
	def.Base.BodyIDA = groundID
	def.Base.BodyIDB = boxID
	world.CreateMotorJoint(&def)

	recorder := oracleDrawJoints(world, false)

	assert.Equal(t, []box2d.HexColor{box2d.ColorLightGray}, oracleLineColors(recorder))
	assert.Equal(t,
		[]box2d.HexColor{box2d.ColorYellowGreen, box2d.ColorPlum},
		oraclePointColors(recorder))
	for _, record := range recorder.points {
		assert.InDelta(t, 8.0, record.size, 0.0)
	}
}

// TestOracleDrawWeldJoint pins down b2DrawWeldJoint,
// src/weld_joint.c:465: two four-vertex boxes, the frame A box in dark orange
// and the frame B box in dark cyan. Nothing else is drawn for a weld joint.
func TestOracleDrawWeldJoint(t *testing.T) {
	t.Parallel()

	world, groundID, boxID := oracleJointDrawWorld(t)

	def := box2d.DefaultWeldJointDef()
	def.Base.BodyIDA = groundID
	def.Base.BodyIDB = boxID
	world.CreateWeldJoint(&def)

	recorder := oracleDrawJoints(world, false)

	require.Len(t, recorder.polygons, 2)
	assert.Equal(t, box2d.ColorDarkOrange, recorder.polygons[0].color)
	assert.Equal(t, box2d.ColorDarkCyan, recorder.polygons[1].color)
	for _, record := range recorder.polygons {
		assert.Len(t, record.vertices, 4, "b2MakeBox produces a four vertex box")
	}
	assert.Empty(t, recorder.lines)
	assert.Empty(t, recorder.points)
}

// TestOracleDrawPrismaticJoint pins down both arms of the limit branch of
// b2DrawPrismaticJoint, src/prismatic_joint.c:676.
//
// Without a limit the C code draws the joint line plus one gray axis line.
// With a limit it draws the joint line, a gray line between the limits and two
// perpendicular ticks, green at the lower limit and red at the upper. The
// spring flag adds a violet point at the target translation. Both frames are
// always marked with a gray and a blue point.
func TestOracleDrawPrismaticJoint(t *testing.T) {
	t.Parallel()

	t.Run("no limit no spring", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultPrismaticJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		world.CreatePrismaticJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorDimGray, box2d.ColorGray},
			oracleLineColors(recorder))
		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorGray, box2d.ColorBlue},
			oraclePointColors(recorder))
	})

	t.Run("limit and spring", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultPrismaticJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		def.EnableLimit = true
		def.LowerTranslation = -1.0
		def.UpperTranslation = 3.0
		def.EnableSpring = true
		def.Hertz = 1.0
		def.DampingRatio = 0.5
		def.TargetTranslation = 1.0
		world.CreatePrismaticJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorDimGray, box2d.ColorGray, box2d.ColorGreen, box2d.ColorRed},
			oracleLineColors(recorder))
		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorViolet, box2d.ColorGray, box2d.ColorBlue},
			oraclePointColors(recorder))

		// The violet spring point sits at frameA.P + targetTranslation * axisA.
		// frame A is the ground origin and the default local frames make the
		// axis +x, so the target point is (1, 0).
		assert.InDelta(t, 1.0, recorder.points[0].p1.X, 1e-9)
		assert.InDelta(t, 0.0, recorder.points[0].p1.Y, 1e-9)
	})
}

// TestOracleDrawWheelJoint pins down both arms of the limit branch of
// b2DrawWheelJoint, src/wheel_joint.c:537. The C code names the colours c1..c5
// and draws the joint line in c5 (blue), then either the limit trio
// (gray / green / red) or a single gray axis line, then a gray point at frame
// A and a dim gray point at frame B.
func TestOracleDrawWheelJoint(t *testing.T) {
	t.Parallel()

	t.Run("no limit", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultWheelJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		world.CreateWheelJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorBlue, box2d.ColorGray},
			oracleLineColors(recorder))
		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorGray, box2d.ColorDimGray},
			oraclePointColors(recorder))
	})

	t.Run("limit", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultWheelJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		def.EnableLimit = true
		def.LowerTranslation = -2.0
		def.UpperTranslation = 2.0
		world.CreateWheelJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorBlue, box2d.ColorGray, box2d.ColorGreen, box2d.ColorRed},
			oracleLineColors(recorder))
	})
}

// TestOracleDrawRevoluteJoint pins down b2DrawRevoluteJoint,
// src/revolute_joint.c:512. The base drawing is a gray circle at frame B, a
// gray radius line on frame A, a blue radius line on frame B and three gold
// connector lines. The limit flag adds a green and a red line; the spring flag
// adds a violet line; DrawJointExtras adds the joint angle label.
func TestOracleDrawRevoluteJoint(t *testing.T) {
	t.Parallel()

	t.Run("plain", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultRevoluteJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		world.CreateRevoluteJoint(&def)

		recorder := oracleDrawJoints(world, false)

		require.Len(t, recorder.circles, 1)
		assert.Equal(t, box2d.ColorGray, recorder.circles[0].color)
		assert.Equal(t,
			[]box2d.HexColor{
				box2d.ColorGray, box2d.ColorBlue,
				box2d.ColorGold, box2d.ColorGold, box2d.ColorGold,
			},
			oracleLineColors(recorder))
		assert.Empty(t, recorder.strings)
	})

	t.Run("limit and spring", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultRevoluteJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		def.EnableLimit = true
		def.LowerAngle = -1.0
		def.UpperAngle = 1.0
		def.EnableSpring = true
		def.Hertz = 1.0
		def.DampingRatio = 0.5
		def.TargetAngle = 0.5
		world.CreateRevoluteJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t,
			[]box2d.HexColor{
				box2d.ColorGray, box2d.ColorBlue,
				box2d.ColorGreen, box2d.ColorRed,
				box2d.ColorViolet,
				box2d.ColorGold, box2d.ColorGold, box2d.ColorGold,
			},
			oracleLineColors(recorder))
	})

	t.Run("extras add the angle label", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultRevoluteJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		world.CreateRevoluteJoint(&def)

		recorder := oracleDrawJoints(world, true)

		// b2DrawRevoluteJoint writes " %.1f deg" and b2DrawJoint writes the
		// force/torque label, so there are two strings.
		require.Len(t, recorder.strings, 2)
		assert.Contains(t, recorder.strings[0].text, "deg")
		assert.Equal(t, box2d.ColorWhite, recorder.strings[0].color)
		assert.Contains(t, recorder.strings[1].text, "f = [")
		assert.Equal(t, box2d.ColorAzure, recorder.strings[1].color)
	})
}

// TestOracleDrawDistanceJoint pins down every branch of b2DrawDistanceJoint,
// src/distance_joint.c:543.
//
// The unconditional part is a white line between the anchors plus a white point
// at each anchor. The limit block only runs when minLength < maxLength AND the
// limit is enabled, and inside it:
//
//	minLength > b2_linearSlop  -> a light green tick at the minimum
//	maxLength < B2_HUGE        -> a red tick at the maximum
//	both                       -> a gray line between the ticks
//
// The spring block adds a blue point at the rest length when hertz > 0 and the
// spring is enabled.
func TestOracleDrawDistanceJoint(t *testing.T) {
	t.Parallel()

	t.Run("rigid", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultDistanceJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		def.Length = 2.0
		world.CreateDistanceJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t, []box2d.HexColor{box2d.ColorWhite}, oracleLineColors(recorder))
		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorWhite, box2d.ColorWhite},
			oraclePointColors(recorder))
	})

	t.Run("limit both ends", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultDistanceJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		def.Length = 2.0
		def.EnableLimit = true
		def.MinLength = 1.0
		def.MaxLength = 3.0
		world.CreateDistanceJoint(&def)

		recorder := oracleDrawJoints(world, false)

		assert.Equal(t,
			[]box2d.HexColor{
				box2d.ColorLightGreen, box2d.ColorRed, box2d.ColorGray,
				box2d.ColorWhite,
			},
			oracleLineColors(recorder))
	})

	t.Run("limit with spring", func(t *testing.T) {
		t.Parallel()

		world, groundID, boxID := oracleJointDrawWorld(t)

		def := box2d.DefaultDistanceJointDef()
		def.Base.BodyIDA = groundID
		def.Base.BodyIDB = boxID
		def.Length = 2.0
		def.EnableLimit = true
		def.MinLength = 1.0
		def.MaxLength = 3.0
		def.EnableSpring = true
		def.Hertz = 2.0
		def.DampingRatio = 0.5
		world.CreateDistanceJoint(&def)

		recorder := oracleDrawJoints(world, false)

		// Two white anchor points plus the blue rest-length point.
		assert.Equal(t,
			[]box2d.HexColor{box2d.ColorWhite, box2d.ColorWhite, box2d.ColorBlue},
			oraclePointColors(recorder))

		// The rest point is pA + length * axis. Frame A is the ground origin,
		// frame B is (2, 0), so the axis is +x and the rest point is (2, 0).
		rest := recorder.points[2].p1
		assert.InDelta(t, 2.0, rest.X, 1e-9)
		assert.InDelta(t, 0.0, rest.Y, 1e-9)
	})
}

// TestOracleDrawJointSkipsDisabledBodies encodes the guard at the head of
// b2DrawJoint, src/joint.c:
//
//	if ( bodyA->setIndex == b2_disabledSet || bodyB->setIndex == b2_disabledSet )
//	    return;
//
// Disabling either attached body silences the whole joint.
func TestOracleDrawJointSkipsDisabledBodies(t *testing.T) {
	t.Parallel()

	world, groundID, boxID := oracleJointDrawWorld(t)

	def := box2d.DefaultRevoluteJointDef()
	def.Base.BodyIDA = groundID
	def.Base.BodyIDB = boxID
	world.CreateRevoluteJoint(&def)

	before := oracleDrawJoints(world, false)
	require.NotEmpty(t, before.lines)

	world.DisableBody(boxID)

	after := oracleDrawJoints(world, false)
	assert.Empty(t, after.lines)
	assert.Empty(t, after.circles)
	assert.Empty(t, after.points)
}

// TestOracleDrawJointGraphColor encodes the DrawGraphColors block of
// b2DrawJoint (src/joint.c): when the joint holds a constraint graph colour,
// a size 5 point in the b2_graphColors palette is drawn at the midpoint of the
// two anchors. A joint between a static body and a dynamic body enters the
// graph after a step.
func TestOracleDrawJointGraphColor(t *testing.T) {
	t.Parallel()

	world, groundID, boxID := oracleJointDrawWorld(t)

	def := box2d.DefaultRevoluteJointDef()
	def.Base.BodyIDA = groundID
	def.Base.BodyIDB = boxID
	world.CreateRevoluteJoint(&def)

	// No step: b2CreateJointInternal already assigns the constraint graph
	// colour for a joint between two non-sleeping bodies, and stepping would
	// move the bodies away from the exact anchors asserted below.

	var plain drawRecorder
	plainDraw := plain.debugDraw()
	plainDraw.DrawJoints = true
	world.Draw(&plainDraw)

	var graph drawRecorder
	graphDraw := graph.debugDraw()
	graphDraw.DrawJoints = true
	graphDraw.DrawGraphColors = true
	world.Draw(&graphDraw)

	require.Len(t, graph.points, len(plain.points)+1,
		"DrawGraphColors adds exactly one point per graph-coloured joint")
	added := graph.points[len(graph.points)-1]
	assert.InDelta(t, 5.0, added.size, 0.0)

	// The midpoint of the two anchors: frame A at the origin, frame B at
	// (2, 0), so the graph point is at (1, 0).
	assert.InDelta(t, 1.0, added.p1.X, 1e-6)
	assert.InDelta(t, 0.0, added.p1.Y, 1e-6)
}

// TestOracleDrawChainSegmentShape pins down the b2_chainSegmentShape arm of
// b2DrawShape, src/physics_world.c:
//
//	draw->DrawLineFcn( p1, p2, color, context );
//	draw->DrawPointFcn( p2, 4.0f, color, context );
//	draw->DrawLineFcn( p1, b2Lerp( p1, p2, 0.1f ), b2_colorPaleGreen, context );
//
// So one chain segment produces two lines and one point: the segment itself in
// the body colour, a pale green stub marking the first tenth of the segment,
// and a size 4 point at the far end.
func TestOracleDrawChainSegmentShape(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	chainDef := box2d.DefaultChainDef()
	// Five points make exactly one solid segment plus ghosts on both ends
	// (src/shape.c: an open chain of n points has n - 3 segments).
	chainDef.Points = []box2d.Vec2{
		{X: -3.0, Y: 0.0},
		{X: 0.0, Y: 0.0},
		{X: 10.0, Y: 0.0},
		{X: 13.0, Y: 0.0},
	}
	world.CreateChain(bodyID, &chainDef)

	recorder := oracleDrawShapes(world)

	require.Len(t, recorder.lines, 2)
	require.Len(t, recorder.points, 1)

	// The solid segment runs (0, 0) -> (10, 0) and lives on a static body, so
	// b2DrawShape gets b2_colorPaleGreen from the ladder's static rung.
	segment := recorder.lines[0]
	assert.Equal(t, box2d.ColorPaleGreen, segment.color)
	assert.InDelta(t, 0.0, segment.p1.X, 1e-12)
	assert.InDelta(t, 10.0, segment.p2.X, 1e-12)

	// The stub covers the first tenth: b2Lerp( p1, p2, 0.1 ) is (1, 0).
	stub := recorder.lines[1]
	assert.Equal(t, box2d.ColorPaleGreen, stub.color)
	assert.InDelta(t, 0.0, stub.p1.X, 1e-12)
	assert.InDelta(t, 1.0, stub.p2.X, 1e-12)

	// The point marks p2 at size 4.
	assert.InDelta(t, 10.0, recorder.points[0].p1.X, 1e-12)
	assert.InDelta(t, 4.0, recorder.points[0].size, 0.0)
}

// TestOracleDrawSegmentShape pins down the b2_segmentShape arm of b2DrawShape:
// a single line between the two transformed end points, and nothing else. The
// segment is placed on a body with a non-identity rotation so the
// b2TransformPoint calls of the C are exercised rather than short-circuited.
func TestOracleDrawSegmentShape(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Position = box2d.Vec2{X: 4.0, Y: 3.0}
	// A quarter turn maps the local +x axis onto the world +y axis.
	bodyDef.Rotation = box2d.MakeRot(0.5 * box2d.Pi)
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	segment := box2d.Segment{
		Point1: box2d.Vec2{X: 0.0, Y: 0.0},
		Point2: box2d.Vec2{X: 2.0, Y: 0.0},
	}
	world.CreateSegmentShape(bodyID, &shapeDef, &segment)

	recorder := oracleDrawShapes(world)

	require.Len(t, recorder.lines, 1)
	assert.Empty(t, recorder.points)

	line := recorder.lines[0]
	assert.Equal(t, box2d.ColorPaleGreen, line.color)
	assert.InDelta(t, 4.0, line.p1.X, 1e-9)
	assert.InDelta(t, 3.0, line.p1.Y, 1e-9)
	assert.InDelta(t, 4.0, line.p2.X, 1e-9)
	assert.InDelta(t, 5.0, line.p2.Y, 1e-9)
}

// TestOracleDrawBoundsUsesTheFatAABB encodes the drawBounds block of
// DrawQueryCallback, src/physics_world.c:1014-1024: the four corners of
// shape->fatAABB in counter-clockwise order, drawn in b2_colorGold.
func TestOracleDrawBoundsUsesTheFatAABB(t *testing.T) {
	t.Parallel()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	bodyDef := box2d.DefaultBodyDef()
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Radius: 1.0}
	shapeID := world.CreateCircleShape(bodyID, &shapeDef, &circle)

	var recorder drawRecorder
	draw := recorder.debugDraw()
	draw.DrawBounds = true
	world.Draw(&draw)

	require.Len(t, recorder.polygons, 1)
	corners := recorder.polygons[0].vertices
	require.Len(t, corners, 4)
	assert.Equal(t, box2d.ColorGold, recorder.polygons[0].color)

	// The C literal is
	//   { lower.x, lower.y }, { upper.x, lower.y },
	//   { upper.x, upper.y }, { lower.x, upper.y }
	// so opposite corners share coordinates pairwise.
	assert.InDelta(t, corners[0].X, corners[3].X, 0.0)
	assert.InDelta(t, corners[1].X, corners[2].X, 0.0)
	assert.InDelta(t, corners[0].Y, corners[1].Y, 0.0)
	assert.InDelta(t, corners[2].Y, corners[3].Y, 0.0)

	// The fat AABB is at least the tight AABB reported by b2Shape_GetAABB.
	tight := world.ShapeAABB(shapeID)
	assert.LessOrEqual(t, corners[0].X, tight.LowerBound.X)
	assert.LessOrEqual(t, corners[0].Y, tight.LowerBound.Y)
	assert.GreaterOrEqual(t, corners[2].X, tight.UpperBound.X)
	assert.GreaterOrEqual(t, corners[2].Y, tight.UpperBound.Y)
}
