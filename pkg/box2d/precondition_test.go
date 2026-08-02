// Tests for the always-on public API preconditions: definition structs must be
// built by their Default* constructor, and the few definition fields whose bad
// values would silently corrupt the simulation are validated at creation time.
// These checks are independent of the debugAsserts build flag, so they must
// hold in a normal (release) test build.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

// requirePanicMessage runs fn, requires that it panicked with exactly
// wantMessage, and that the message mentions mustMention (the constructor or
// field name a caller needs in order to fix the mistake).
func requirePanicMessage(t *testing.T, wantMessage, mustMention string, fn func()) {
	t.Helper()

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		fn()
	}()

	require.NotNil(t, recovered, "expected a panic")
	msg, ok := recovered.(string)
	require.True(t, ok, "panic value should be a string, got %T", recovered)
	require.Equal(t, wantMessage, msg)
	require.Contains(t, msg, mustMention)
}

// preconditionWorld creates a world for tests that need one.
func preconditionWorld(t *testing.T) *box2d.World {
	t.Helper()

	def := box2d.DefaultWorldDef()
	w := box2d.NewWorld(&def)
	t.Cleanup(w.Destroy)
	return w
}

// preconditionBody creates a dynamic circle body for tests that need one.
func preconditionBody(t *testing.T, w *box2d.World, x float64) box2d.BodyID {
	t.Helper()

	bd := box2d.DefaultBodyDef()
	bd.Type = box2d.DynamicBody
	bd.Position = box2d.Vec2{X: x, Y: 0.0}
	bodyID := w.CreateBody(&bd)

	circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.25}
	sd := box2d.DefaultShapeDef()
	w.CreateCircleShape(bodyID, &sd, &circle)
	return bodyID
}

// jointBase returns a joint base wired to two bodies of w.
func jointBase(t *testing.T, w *box2d.World) box2d.JointDef {
	t.Helper()

	base := box2d.DefaultDistanceJointDef().Base
	base.BodyIDA = preconditionBody(t, w, 0.0)
	base.BodyIDB = preconditionBody(t, w, 1.0)
	return base
}

func TestZeroValueDefinitionPanics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctor string
		want string
		call func(t *testing.T)
	}{
		{
			name: "NewWorld",
			ctor: "DefaultWorldDef",
			want: "box2d: WorldDef was not initialized by DefaultWorldDef " +
				"(zero-value definition structs are not valid; see DefaultWorldDef)",
			call: func(_ *testing.T) {
				box2d.NewWorld(&box2d.WorldDef{})
			},
		},
		{
			name: "CreateBody",
			ctor: "DefaultBodyDef",
			want: "box2d: BodyDef was not initialized by DefaultBodyDef " +
				"(zero-value definition structs are not valid; see DefaultBodyDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateBody(&box2d.BodyDef{})
			},
		},
		{
			name: "CreateCircleShape",
			ctor: "DefaultShapeDef",
			want: "box2d: ShapeDef was not initialized by DefaultShapeDef " +
				"(zero-value definition structs are not valid; see DefaultShapeDef)",
			call: func(t *testing.T) {
				w := preconditionWorld(t)
				circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
				w.CreateCircleShape(preconditionBody(t, w, 0.0), &box2d.ShapeDef{}, &circle)
			},
		},
		{
			name: "CreateCapsuleShape",
			ctor: "DefaultShapeDef",
			want: "box2d: ShapeDef was not initialized by DefaultShapeDef " +
				"(zero-value definition structs are not valid; see DefaultShapeDef)",
			call: func(t *testing.T) {
				w := preconditionWorld(t)
				capsule := box2d.Capsule{
					Center1: box2d.Vec2{X: -0.5, Y: 0.0},
					Center2: box2d.Vec2{X: 0.5, Y: 0.0},
					Radius:  0.25,
				}
				w.CreateCapsuleShape(preconditionBody(t, w, 0.0), &box2d.ShapeDef{}, &capsule)
			},
		},
		{
			name: "CreatePolygonShape",
			ctor: "DefaultShapeDef",
			want: "box2d: ShapeDef was not initialized by DefaultShapeDef " +
				"(zero-value definition structs are not valid; see DefaultShapeDef)",
			call: func(t *testing.T) {
				w := preconditionWorld(t)
				box := box2d.MakeBox(0.5, 0.5)
				w.CreatePolygonShape(preconditionBody(t, w, 0.0), &box2d.ShapeDef{}, &box)
			},
		},
		{
			name: "CreateSegmentShape",
			ctor: "DefaultShapeDef",
			want: "box2d: ShapeDef was not initialized by DefaultShapeDef " +
				"(zero-value definition structs are not valid; see DefaultShapeDef)",
			call: func(t *testing.T) {
				w := preconditionWorld(t)
				segment := box2d.Segment{
					Point1: box2d.Vec2{X: -1.0, Y: 0.0},
					Point2: box2d.Vec2{X: 1.0, Y: 0.0},
				}
				w.CreateSegmentShape(preconditionBody(t, w, 0.0), &box2d.ShapeDef{}, &segment)
			},
		},
		{
			name: "CreateChain",
			ctor: "DefaultChainDef",
			want: "box2d: ChainDef was not initialized by DefaultChainDef " +
				"(zero-value definition structs are not valid; see DefaultChainDef)",
			call: func(t *testing.T) {
				w := preconditionWorld(t)
				w.CreateChain(preconditionBody(t, w, 0.0), &box2d.ChainDef{})
			},
		},
		{
			name: "CreateDistanceJoint",
			ctor: "DefaultDistanceJointDef",
			want: "box2d: DistanceJointDef was not initialized by DefaultDistanceJointDef " +
				"(zero-value definition structs are not valid; see DefaultDistanceJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateDistanceJoint(&box2d.DistanceJointDef{})
			},
		},
		{
			name: "CreateMotorJoint",
			ctor: "DefaultMotorJointDef",
			want: "box2d: MotorJointDef was not initialized by DefaultMotorJointDef " +
				"(zero-value definition structs are not valid; see DefaultMotorJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateMotorJoint(&box2d.MotorJointDef{})
			},
		},
		{
			name: "CreateFilterJoint",
			ctor: "DefaultFilterJointDef",
			want: "box2d: FilterJointDef was not initialized by DefaultFilterJointDef " +
				"(zero-value definition structs are not valid; see DefaultFilterJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateFilterJoint(&box2d.FilterJointDef{})
			},
		},
		{
			name: "CreatePrismaticJoint",
			ctor: "DefaultPrismaticJointDef",
			want: "box2d: PrismaticJointDef was not initialized by DefaultPrismaticJointDef " +
				"(zero-value definition structs are not valid; see DefaultPrismaticJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreatePrismaticJoint(&box2d.PrismaticJointDef{})
			},
		},
		{
			name: "CreateRevoluteJoint",
			ctor: "DefaultRevoluteJointDef",
			want: "box2d: RevoluteJointDef was not initialized by DefaultRevoluteJointDef " +
				"(zero-value definition structs are not valid; see DefaultRevoluteJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateRevoluteJoint(&box2d.RevoluteJointDef{})
			},
		},
		{
			name: "CreateWeldJoint",
			ctor: "DefaultWeldJointDef",
			want: "box2d: WeldJointDef was not initialized by DefaultWeldJointDef " +
				"(zero-value definition structs are not valid; see DefaultWeldJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateWeldJoint(&box2d.WeldJointDef{})
			},
		},
		{
			name: "CreateWheelJoint",
			ctor: "DefaultWheelJointDef",
			want: "box2d: WheelJointDef was not initialized by DefaultWheelJointDef " +
				"(zero-value definition structs are not valid; see DefaultWheelJointDef)",
			call: func(t *testing.T) {
				preconditionWorld(t).CreateWheelJoint(&box2d.WheelJointDef{})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requirePanicMessage(t, tc.want, tc.ctor, func() { tc.call(t) })
		})
	}
}

func TestCorruptedBodyRotationPanics(t *testing.T) {
	t.Parallel()

	w := preconditionWorld(t)

	bd := box2d.DefaultBodyDef()
	bd.Rotation = box2d.Rot{}

	requirePanicMessage(t,
		"box2d: BodyDef.Rotation is invalid: must be a normalized rotation "+
			"(the zero Rot{} is not; use DefaultBodyDef or MakeRot)",
		"Rotation",
		func() { w.CreateBody(&bd) })
}

func TestNonFiniteShapeFieldsPanic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		want  string
		mutID func(sd *box2d.ShapeDef)
	}{
		{
			name:  "Density",
			field: "Density",
			want:  "box2d: ShapeDef.Density is invalid: must be a finite number",
			mutID: func(sd *box2d.ShapeDef) { sd.Density = math.NaN() },
		},
		{
			name:  "Friction",
			field: "Material.Friction",
			want:  "box2d: ShapeDef.Material.Friction is invalid: must be a finite number",
			mutID: func(sd *box2d.ShapeDef) { sd.Material.Friction = math.Inf(1) },
		},
		{
			name:  "Restitution",
			field: "Material.Restitution",
			want:  "box2d: ShapeDef.Material.Restitution is invalid: must be a finite number",
			mutID: func(sd *box2d.ShapeDef) { sd.Material.Restitution = math.NaN() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := preconditionWorld(t)
			bodyID := preconditionBody(t, w, 0.0)

			sd := box2d.DefaultShapeDef()
			tc.mutID(&sd)

			circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
			requirePanicMessage(t, tc.want, tc.field, func() {
				w.CreateCircleShape(bodyID, &sd, &circle)
			})
		})
	}
}

func TestConstructorBuiltDefinitionsDoNotPanic(t *testing.T) {
	t.Parallel()

	require.NotPanics(t, func() {
		wd := box2d.DefaultWorldDef()
		w := box2d.NewWorld(&wd)
		defer w.Destroy()

		bd := box2d.DefaultBodyDef()
		bd.Type = box2d.DynamicBody
		bodyID := w.CreateBody(&bd)
		require.True(t, bodyID.IsNonNull())

		sd := box2d.DefaultShapeDef()
		circle := box2d.Circle{Center: box2d.Vec2Zero, Radius: 0.5}
		w.CreateCircleShape(bodyID, &sd, &circle)

		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: -0.5, Y: 0.0},
			Center2: box2d.Vec2{X: 0.5, Y: 0.0},
			Radius:  0.25,
		}
		w.CreateCapsuleShape(bodyID, &sd, &capsule)

		box := box2d.MakeBox(0.5, 0.5)
		w.CreatePolygonShape(bodyID, &sd, &box)

		staticDef := box2d.DefaultBodyDef()
		ground := w.CreateBody(&staticDef)

		segment := box2d.Segment{
			Point1: box2d.Vec2{X: -1.0, Y: 0.0},
			Point2: box2d.Vec2{X: 1.0, Y: 0.0},
		}
		w.CreateSegmentShape(ground, &sd, &segment)

		cd := box2d.DefaultChainDef()
		cd.Points = []box2d.Vec2{
			{X: -4.0, Y: -2.0},
			{X: -2.0, Y: -2.0},
			{X: 2.0, Y: -2.0},
			{X: 4.0, Y: -2.0},
		}
		w.CreateChain(ground, &cd)

		base := jointBase(t, w)

		distance := box2d.DefaultDistanceJointDef()
		distance.Base = base
		w.CreateDistanceJoint(&distance)

		motor := box2d.DefaultMotorJointDef()
		motor.Base = base
		w.CreateMotorJoint(&motor)

		filter := box2d.DefaultFilterJointDef()
		filter.Base = base
		w.CreateFilterJoint(&filter)

		prismatic := box2d.DefaultPrismaticJointDef()
		prismatic.Base = base
		w.CreatePrismaticJoint(&prismatic)

		revolute := box2d.DefaultRevoluteJointDef()
		revolute.Base = base
		w.CreateRevoluteJoint(&revolute)

		weld := box2d.DefaultWeldJointDef()
		weld.Base = base
		w.CreateWeldJoint(&weld)

		wheel := box2d.DefaultWheelJointDef()
		wheel.Base = base
		w.CreateWheelJoint(&wheel)
	})
}
