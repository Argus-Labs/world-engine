// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/types.h and src/types.c.

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

func TestDefaultWorldDef(t *testing.T) {
	t.Parallel()

	// The package default is 1 length unit per meter; the upstream defaults
	// scale by that factor.
	lengthUnits := box2d.GetLengthUnitsPerMeter()
	require.InDelta(t, 1.0, lengthUnits, 0)

	def := box2d.DefaultWorldDef()

	assert.Equal(t, box2d.Vec2{X: 0.0, Y: -10.0}, def.Gravity)
	assert.InDelta(t, 1.0*lengthUnits, def.HitEventThreshold, 0)
	assert.InDelta(t, 1.0*lengthUnits, def.RestitutionThreshold, 0)
	assert.InDelta(t, 3.0*lengthUnits, def.ContactSpeed, 0)
	assert.InDelta(t, 30.0, def.ContactHertz, 0)
	assert.InDelta(t, 10.0, def.ContactDampingRatio, 0)
	assert.InDelta(t, 400.0*lengthUnits, def.MaximumLinearSpeed, 0)
	assert.True(t, def.EnableSleep)
	assert.True(t, def.EnableContinuous)

	// Not set by the upstream constructor.
	assert.False(t, def.EnableContactSoftening)
	assert.Nil(t, def.FrictionCallback)
	assert.Nil(t, def.RestitutionCallback)
	assert.Zero(t, def.UserData)
}

func TestDefaultBodyDef(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultBodyDef()

	assert.Equal(t, box2d.StaticBody, def.Type)
	assert.Equal(t, box2d.RotIdentity, def.Rotation)
	assert.InDelta(t, 0.05*box2d.GetLengthUnitsPerMeter(), def.SleepThreshold, 0)
	assert.InDelta(t, 1.0, def.GravityScale, 0)
	assert.True(t, def.EnableSleep)
	assert.True(t, def.IsAwake)
	assert.True(t, def.IsEnabled)

	// Not set by the upstream constructor.
	assert.Equal(t, box2d.Vec2{}, def.Position)
	assert.Equal(t, box2d.Vec2{}, def.LinearVelocity)
	assert.Zero(t, def.AngularVelocity)
	assert.Zero(t, def.LinearDamping)
	assert.Zero(t, def.AngularDamping)
	assert.Empty(t, def.Name)
	assert.Zero(t, def.UserData)
	assert.Equal(t, box2d.MotionLocks{}, def.MotionLocks)
	assert.False(t, def.IsBullet)
	assert.False(t, def.AllowFastRotation)
}

func TestDefaultFilter(t *testing.T) {
	t.Parallel()

	filter := box2d.DefaultFilter()

	assert.Equal(t, uint64(1), filter.CategoryBits)
	assert.Equal(t, uint64(math.MaxUint64), filter.MaskBits)
	assert.Zero(t, filter.GroupIndex)

	assert.Equal(t, box2d.DefaultCategoryBits, filter.CategoryBits)
	assert.Equal(t, box2d.DefaultMaskBits, filter.MaskBits)
}

func TestDefaultQueryFilter(t *testing.T) {
	t.Parallel()

	filter := box2d.DefaultQueryFilter()

	assert.Equal(t, uint64(1), filter.CategoryBits)
	assert.Equal(t, uint64(math.MaxUint64), filter.MaskBits)
}

func TestDefaultShapeDef(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultShapeDef()

	assert.InDelta(t, 0.6, def.Material.Friction, 0)
	assert.InDelta(t, 1.0, def.Density, 0)
	assert.Equal(t, box2d.DefaultFilter(), def.Filter)
	assert.True(t, def.UpdateBodyMass)
	assert.True(t, def.InvokeContactCreation)

	// Not set by the upstream constructor.
	assert.Zero(t, def.UserData)
	assert.Zero(t, def.Material.Restitution)
	assert.Zero(t, def.Material.RollingResistance)
	assert.Zero(t, def.Material.TangentSpeed)
	assert.Zero(t, def.Material.UserMaterialID)
	assert.Zero(t, def.Material.CustomColor)
	assert.False(t, def.EnableCustomFiltering)
	assert.False(t, def.IsSensor)
	assert.False(t, def.EnableSensorEvents)
	assert.False(t, def.EnableContactEvents)
	assert.False(t, def.EnableHitEvents)
	assert.False(t, def.EnablePreSolveEvents)
}

func TestDefaultSurfaceMaterial(t *testing.T) {
	t.Parallel()

	material := box2d.DefaultSurfaceMaterial()

	assert.InDelta(t, 0.6, material.Friction, 0)
	assert.Zero(t, material.Restitution)
	assert.Zero(t, material.RollingResistance)
	assert.Zero(t, material.TangentSpeed)
	assert.Zero(t, material.UserMaterialID)
	assert.Zero(t, material.CustomColor)
}

func TestDefaultChainDef(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultChainDef()

	// Upstream: materialCount = 1 with a single friction-0.6 material.
	require.Len(t, def.Materials, 1)
	assert.InDelta(t, 0.6, def.Materials[0].Friction, 0)
	assert.Equal(t, box2d.DefaultSurfaceMaterial(), def.Materials[0])
	assert.Equal(t, box2d.DefaultFilter(), def.Filter)

	// Not set by the upstream constructor.
	assert.Zero(t, def.UserData)
	assert.Empty(t, def.Points)
	assert.False(t, def.IsLoop)
	assert.False(t, def.EnableSensorEvents)

	// Each call must hand back an independent material slice (upstream shares
	// a static default; this port does not).
	other := box2d.DefaultChainDef()
	def.Materials[0].Friction = 0.1
	assert.InDelta(t, 0.6, other.Materials[0].Friction, 0)
}

func TestDefaultDebugDraw(t *testing.T) {
	t.Parallel()

	draw := box2d.DefaultDebugDraw()

	// Every callback must be non-nil so callers may implement a subset.
	assert.NotNil(t, draw.DrawPolygonFcn)
	assert.NotNil(t, draw.DrawSolidPolygonFcn)
	assert.NotNil(t, draw.DrawCircleFcn)
	assert.NotNil(t, draw.DrawSolidCircleFcn)
	assert.NotNil(t, draw.DrawSolidCapsuleFcn)
	assert.NotNil(t, draw.DrawLineFcn)
	assert.NotNil(t, draw.DrawTransformFcn)
	assert.NotNil(t, draw.DrawPointFcn)
	assert.NotNil(t, draw.DrawStringFcn)

	assert.Equal(t, box2d.Vec2{X: -math.MaxFloat32, Y: -math.MaxFloat32}, draw.DrawingBounds.LowerBound)
	assert.Equal(t, box2d.Vec2{X: math.MaxFloat32, Y: math.MaxFloat32}, draw.DrawingBounds.UpperBound)
	assert.InDelta(t, 1.0, draw.ForceScale, 0)
	assert.InDelta(t, 1.0, draw.JointScale, 0)
	assert.True(t, draw.DrawShapes)

	// Not set by the upstream constructor.
	assert.Equal(t, box2d.DrawContactsNone, draw.ContactDrawType)
	assert.False(t, draw.DrawJoints)
	assert.False(t, draw.DrawJointExtras)
	assert.False(t, draw.DrawBounds)
	assert.False(t, draw.DrawMass)
	assert.False(t, draw.DrawBodyNames)
	assert.False(t, draw.DrawGraphColors)
	assert.False(t, draw.DrawContactFeatures)
	assert.False(t, draw.DrawContactNormals)
	assert.False(t, draw.DrawContactForces)
	assert.False(t, draw.DrawFrictionForces)
	assert.False(t, draw.DrawIslands)
	assert.Nil(t, draw.Context)

	// The empty callbacks must be safe to invoke.
	draw.DrawPolygonFcn(nil, box2d.ColorRed, nil)
	draw.DrawSolidPolygonFcn(box2d.TransformIdentity, nil, 0, box2d.ColorRed, nil)
	draw.DrawCircleFcn(box2d.Vec2{}, 1, box2d.ColorRed, nil)
	draw.DrawSolidCircleFcn(box2d.TransformIdentity, 1, box2d.ColorRed, nil)
	draw.DrawSolidCapsuleFcn(box2d.Vec2{}, box2d.Vec2{}, 1, box2d.ColorRed, nil)
	draw.DrawLineFcn(box2d.Vec2{}, box2d.Vec2{}, box2d.ColorRed, nil)
	draw.DrawTransformFcn(box2d.TransformIdentity, nil)
	draw.DrawPointFcn(box2d.Vec2{}, 1, box2d.ColorRed, nil)
	draw.DrawStringFcn(box2d.Vec2{}, "", box2d.ColorRed, nil)
}

// TestDefaultEnumOrdinals pins the upstream enum ordinals that the defs above
// depend on.
func TestDefaultEnumOrdinals(t *testing.T) {
	t.Parallel()

	assert.Equal(t, box2d.StaticBody, box2d.BodyType(0))
	assert.Equal(t, box2d.KinematicBody, box2d.BodyType(1))
	assert.Equal(t, box2d.DynamicBody, box2d.BodyType(2))
	assert.Equal(t, box2d.BodyTypeCount, box2d.BodyType(3))

	assert.Equal(t, box2d.CircleShape, box2d.ShapeType(0))
	assert.Equal(t, box2d.CapsuleShape, box2d.ShapeType(1))
	assert.Equal(t, box2d.SegmentShape, box2d.ShapeType(2))
	assert.Equal(t, box2d.PolygonShape, box2d.ShapeType(3))
	assert.Equal(t, box2d.ChainSegmentShape, box2d.ShapeType(4))
	assert.Equal(t, box2d.ShapeTypeCount, box2d.ShapeType(5))

	assert.Equal(t, box2d.DistanceJoint, box2d.JointType(0))
	assert.Equal(t, box2d.FilterJoint, box2d.JointType(1))
	assert.Equal(t, box2d.MotorJoint, box2d.JointType(2))
	assert.Equal(t, box2d.PrismaticJoint, box2d.JointType(3))
	assert.Equal(t, box2d.RevoluteJoint, box2d.JointType(4))
	assert.Equal(t, box2d.WeldJoint, box2d.JointType(5))
	assert.Equal(t, box2d.WheelJoint, box2d.JointType(6))

	assert.Equal(t, box2d.DrawContactsNone, box2d.ContactDrawType(0))
	assert.Equal(t, box2d.DrawContactsClip, box2d.ContactDrawType(1))
	assert.Equal(t, box2d.DrawContactsAnchorA, box2d.ContactDrawType(2))
	assert.Equal(t, box2d.DrawContactsAnchorB, box2d.ContactDrawType(3))
	assert.Equal(t, box2d.DrawContactsAverage, box2d.ContactDrawType(4))

	assert.Equal(t, box2d.ColorBox2DRed, box2d.HexColor(0xDC3132))
	assert.Equal(t, box2d.ColorBlack, box2d.HexColor(0x000000))
	assert.Equal(t, box2d.ColorWhite, box2d.HexColor(0xFFFFFF))
}
