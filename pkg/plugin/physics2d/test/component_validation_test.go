package physics2d_test

import (
	"encoding/json"
	"math"
	"testing"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	phycomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Transform2D.Validate
// ---------------------------------------------------------------------------

func TestValidate_Transform2D_Valid(t *testing.T) {
	t.Parallel()
	err := phycomp.Transform2D{
		Position: phycomp.Vec2{X: 1, Y: -2.5},
		Rotation: 3.14,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_Transform2D_ZeroIsValid(t *testing.T) {
	t.Parallel()
	require.NoError(t, phycomp.Transform2D{}.Validate())
}

func TestValidate_Transform2D_NaNPosition(t *testing.T) {
	t.Parallel()
	err := phycomp.Transform2D{
		Position: phycomp.Vec2{X: math.NaN(), Y: 0},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "position")
}

func TestValidate_Transform2D_InfPosition(t *testing.T) {
	t.Parallel()
	err := phycomp.Transform2D{
		Position: phycomp.Vec2{X: 0, Y: math.Inf(1)},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "position")
}

func TestValidate_Transform2D_NaNRotation(t *testing.T) {
	t.Parallel()
	err := phycomp.Transform2D{Rotation: math.NaN()}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "rotation")
}

func TestValidate_Transform2D_InfRotation(t *testing.T) {
	t.Parallel()
	err := phycomp.Transform2D{Rotation: math.Inf(-1)}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "rotation")
}

// ---------------------------------------------------------------------------
// Velocity2D.Validate
// ---------------------------------------------------------------------------

func TestValidate_Velocity2D_Valid(t *testing.T) {
	t.Parallel()
	err := phycomp.Velocity2D{
		Linear:  phycomp.Vec2{X: 5, Y: -3},
		Angular: 1.5,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_Velocity2D_ZeroIsValid(t *testing.T) {
	t.Parallel()
	require.NoError(t, phycomp.Velocity2D{}.Validate())
}

func TestValidate_Velocity2D_NaNLinear(t *testing.T) {
	t.Parallel()
	err := phycomp.Velocity2D{
		Linear: phycomp.Vec2{X: math.NaN(), Y: 0},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "linear")
}

func TestValidate_Velocity2D_InfAngular(t *testing.T) {
	t.Parallel()
	err := phycomp.Velocity2D{Angular: math.Inf(1)}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "angular")
}

// ---------------------------------------------------------------------------
// ShapeSlot.Validate
// ---------------------------------------------------------------------------

func TestValidate_ShapeSlot_Valid(t *testing.T) {
	t.Parallel()
	require.NoError(t, phycomp.Slot(7).At(phycomp.Vec2{X: 1, Y: 2}, 0.5).Validate())
	require.NoError(t, phycomp.ShapeSlot{}.Validate(), "resolvability is checked at attach, not here")
}

func TestValidate_ShapeSlot_NaNLocalOffset(t *testing.T) {
	t.Parallel()
	err := phycomp.Slot(1).At(phycomp.Vec2{X: math.NaN(), Y: 0}, 0).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "local_offset")
}

func TestValidate_ShapeSlot_InfLocalRotation(t *testing.T) {
	t.Parallel()
	err := phycomp.Slot(1).At(phycomp.Vec2{}, math.Inf(1)).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "local_rotation")
}

// ---------------------------------------------------------------------------
// ShapeCommon.Validate
// ---------------------------------------------------------------------------

func TestShapeCommon_Defaults(t *testing.T) {
	t.Parallel()
	c := phycomp.DefaultShapeCommon()
	require.NoError(t, c.Validate())
	require.False(t, c.IsSensor)
	require.InDelta(t, 0.6, c.Friction, 0)
	require.InDelta(t, 0.0, c.Restitution, 0)
	require.InDelta(t, 1.0, c.Density, 0)
	require.Equal(t, uint64(1), c.CategoryBits)
	require.Equal(t, ^uint64(0), c.MaskBits)
	require.Equal(t, int32(0), c.GroupIndex)
}

func TestValidate_ShapeCommon_NaNFriction(t *testing.T) {
	t.Parallel()
	c := phycomp.DefaultShapeCommon()
	c.Friction = math.NaN()
	err := c.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "friction")
}

func TestValidate_ShapeCommon_InfRestitution(t *testing.T) {
	t.Parallel()
	c := phycomp.DefaultShapeCommon()
	c.Restitution = math.Inf(1)
	err := c.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "restitution")
}

func TestValidate_ShapeCommon_InfDensity(t *testing.T) {
	t.Parallel()
	c := phycomp.DefaultShapeCommon()
	c.Density = math.Inf(-1)
	err := c.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "density")
}

// ---------------------------------------------------------------------------
// Geometry components
// ---------------------------------------------------------------------------

func TestValidate_Geometry_Valid(t *testing.T) {
	t.Parallel()
	require.NoError(t, phycomp.CircleGeom{Radius: 0.5}.Validate())
	require.NoError(t, phycomp.BoxGeom{HalfExtents: phycomp.Vec2{X: 1, Y: 0.5}}.Validate())
	require.NoError(t, phycomp.PolygonGeom{
		Vertices: [phycomp.MaxPolygonVertices]phycomp.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1}},
		Count:    3,
	}.Validate())
	require.NoError(t, phycomp.ChainGeom{Points: []phycomp.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}}, Loop: true}.Validate())
	require.NoError(t, phycomp.ChainGeom{}.Validate(), "point-count rules are Box2D's, at attach")
	require.NoError(t, phycomp.EdgeGeom{A: phycomp.Vec2{X: 0, Y: 0}, B: phycomp.Vec2{X: 3, Y: 0}}.Validate())
	require.NoError(t, phycomp.CapsuleGeom{A: phycomp.Vec2{}, B: phycomp.Vec2{X: 1}, Radius: 0.25}.Validate())
}

func TestValidate_CircleGeom_NaNRadius(t *testing.T) {
	t.Parallel()
	err := phycomp.CircleGeom{Radius: math.NaN()}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "radius")
}

func TestValidate_BoxGeom_InfHalfExtents(t *testing.T) {
	t.Parallel()
	err := phycomp.BoxGeom{HalfExtents: phycomp.Vec2{X: math.Inf(1), Y: 1}}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "half_extents")
}

func TestValidate_PolygonGeom_CountBounds(t *testing.T) {
	t.Parallel()
	var g phycomp.PolygonGeom
	g.Count = 2
	err := g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "count")

	g.Count = phycomp.MaxPolygonVertices + 1
	err = g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "count")
}

func TestValidate_PolygonGeom_NaNVertex(t *testing.T) {
	t.Parallel()
	g := phycomp.PolygonGeom{
		Vertices: [phycomp.MaxPolygonVertices]phycomp.Vec2{{X: 0, Y: 0}, {X: math.NaN(), Y: 0}, {X: 0, Y: 1}},
		Count:    3,
	}
	err := g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "vertices[1]")
}

func TestValidate_PolygonGeom_UnusedSlotsIgnored(t *testing.T) {
	t.Parallel()
	g := phycomp.PolygonGeom{
		Vertices: [phycomp.MaxPolygonVertices]phycomp.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: math.NaN()}},
		Count:    3,
	}
	require.NoError(t, g.Validate(), "slots past Count are not part of the polygon")
}

func TestValidate_EdgeGeom_NaNEndpoint(t *testing.T) {
	t.Parallel()
	err := phycomp.EdgeGeom{A: phycomp.Vec2{X: math.NaN(), Y: 0}, B: phycomp.Vec2{X: 1, Y: 0}}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "a:")
}

func TestValidate_CapsuleGeom_NaNRadius(t *testing.T) {
	t.Parallel()
	err := phycomp.CapsuleGeom{B: phycomp.Vec2{X: 1}, Radius: math.NaN()}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "radius")
}

func TestValidate_ChainGeom_NaNPoint(t *testing.T) {
	t.Parallel()
	err := phycomp.ChainGeom{
		Points: []phycomp.Vec2{{X: 0, Y: 0}, {X: 0, Y: math.Inf(1)}},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "points[1]")
}

// ---------------------------------------------------------------------------
// PhysicsBody2D.Validate
// ---------------------------------------------------------------------------

func TestValidate_PhysicsBody2D_Valid(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Slot(3))
	require.NoError(t, pb.Validate())
}

func TestValidate_PhysicsBody2D_NoShapes(t *testing.T) {
	t.Parallel()
	pb := phycomp.PhysicsBody2D{BodyType: phycomp.BodyTypeDynamic}
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "shapes")
}

func TestValidate_PhysicsBody2D_InvalidBodyType(t *testing.T) {
	t.Parallel()
	pb := phycomp.PhysicsBody2D{
		BodyType: 99,
		Shapes:   []phycomp.ShapeSlot{phycomp.Slot(3)},
	}
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "body_type")
}

func TestValidate_PhysicsBody2D_NaNLinearDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Slot(3))
	pb.LinearDamping = math.NaN()
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "linear_damping")
}

func TestValidate_PhysicsBody2D_InfAngularDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Slot(3))
	pb.AngularDamping = math.Inf(1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "angular_damping")
}

func TestValidate_PhysicsBody2D_InfGravityScale(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Slot(3))
	pb.GravityScale = math.Inf(-1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "gravity_scale")
}

func TestValidate_PhysicsBody2D_InvalidSlot(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic,
		phycomp.Slot(3).At(phycomp.Vec2{X: math.NaN()}, 0))
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "shapes[0]")
}

func TestValidate_PhysicsBody2D_AllBodyTypes(t *testing.T) {
	t.Parallel()
	for _, bt := range []phycomp.BodyType{
		phycomp.BodyTypeStatic,
		phycomp.BodyTypeDynamic,
		phycomp.BodyTypeKinematic,
		phycomp.BodyTypeManual,
	} {
		require.NoError(t, phycomp.NewPhysicsBody2D(bt, phycomp.Slot(3)).Validate(), "body type %d", bt)
	}
}

// ---------------------------------------------------------------------------
// NewPhysicsBody2D constructor defaults
// ---------------------------------------------------------------------------

func TestNewPhysicsBody2D_Defaults(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Slot(3))
	require.Equal(t, phycomp.BodyTypeDynamic, pb.BodyType)
	require.InDelta(t, 1.0, pb.GravityScale, 1e-12)
	require.True(t, pb.Active)
	require.True(t, pb.Awake)
	require.True(t, pb.SleepingAllowed)
	require.False(t, pb.Bullet)
	require.False(t, pb.FixedRotation)
	require.InDelta(t, 0.0, pb.LinearDamping, 1e-12)
	require.InDelta(t, 0.0, pb.AngularDamping, 1e-12)
	require.Len(t, pb.Shapes, 1)
}

func TestNewPhysicsBody2D_MultipleShapes(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeStatic,
		phycomp.Slot(3), phycomp.Slot(4).At(phycomp.Vec2{X: 1}, 0))
	require.Len(t, pb.Shapes, 2)
	require.Equal(t, phycomp.Slot(4).At(phycomp.Vec2{X: 1}, 0), pb.Shapes[1])
}

// ---------------------------------------------------------------------------
// PhysicsBody2D JSON unmarshal (defaults for missing fields)
// ---------------------------------------------------------------------------

func TestUnmarshalPhysicsBody2D_MissingFieldsGetDefaults(t *testing.T) {
	t.Parallel()
	// Minimal JSON: only body_type and shapes
	data := `{
		"body_type": 2,
		"shapes": [{"shape": 7}]
	}`
	var pb phycomp.PhysicsBody2D
	require.NoError(t, json.Unmarshal([]byte(data), &pb))
	require.Equal(t, phycomp.BodyTypeDynamic, pb.BodyType)
	require.InDelta(t, 1.0, pb.GravityScale, 1e-12, "missing gravity_scale defaults to 1")
	require.True(t, pb.Active, "missing active defaults to true")
	require.True(t, pb.Awake, "missing awake defaults to true")
	require.True(t, pb.SleepingAllowed, "missing sleeping_allowed defaults to true")
	require.False(t, pb.Bullet)
	require.False(t, pb.FixedRotation)
	require.Equal(t, []phycomp.ShapeSlot{phycomp.Slot(7)}, pb.Shapes)
}

func TestUnmarshalPhysicsBody2D_ExplicitFalsePreserved(t *testing.T) {
	t.Parallel()
	data := `{
		"body_type": 2,
		"active": false,
		"awake": false,
		"sleeping_allowed": false,
		"gravity_scale": 0,
		"shapes": [{"shape": 7}]
	}`
	var pb phycomp.PhysicsBody2D
	require.NoError(t, json.Unmarshal([]byte(data), &pb))
	require.False(t, pb.Active, "explicit false preserved")
	require.False(t, pb.Awake, "explicit false preserved")
	require.False(t, pb.SleepingAllowed, "explicit false preserved")
	require.InDelta(t, 0.0, pb.GravityScale, 1e-12, "explicit 0 preserved")
}

func TestUnmarshalPhysicsBody2D_FullPayload(t *testing.T) {
	t.Parallel()
	data := `{
		"body_type": 3,
		"linear_damping": 0.5,
		"angular_damping": 0.3,
		"gravity_scale": 2.0,
		"active": true,
		"awake": true,
		"sleeping_allowed": false,
		"bullet": true,
		"fixed_rotation": true,
		"shapes": [
			{"shape": 7, "local_offset": {"x": 1, "y": 2}, "local_rotation": 0.5}
		]
	}`
	var pb phycomp.PhysicsBody2D
	require.NoError(t, json.Unmarshal([]byte(data), &pb))
	require.Equal(t, phycomp.BodyTypeKinematic, pb.BodyType)
	require.InDelta(t, 0.5, pb.LinearDamping, 1e-12)
	require.InDelta(t, 0.3, pb.AngularDamping, 1e-12)
	require.InDelta(t, 2.0, pb.GravityScale, 1e-12)
	require.True(t, pb.Active)
	require.True(t, pb.Awake)
	require.False(t, pb.SleepingAllowed)
	require.True(t, pb.Bullet)
	require.True(t, pb.FixedRotation)
	require.Len(t, pb.Shapes, 1)
	require.Equal(t, phycomp.Slot(7).At(phycomp.Vec2{X: 1, Y: 2}, 0.5), pb.Shapes[0])
}

// ---------------------------------------------------------------------------
// Component Name() methods
// ---------------------------------------------------------------------------

// Event names are a wire contract: renaming one silently breaks every consumer
// that subscribes by name, and nothing in the simulation would fail. Kept beside
// the component names for that reason.
func TestEventNames(t *testing.T) {
	t.Parallel()
	require.Equal(t, "physics2d_contact_begin", physics.ContactBeginEvent{}.Name())
	require.Equal(t, "physics2d_contact_end", physics.ContactEndEvent{}.Name())
	require.Equal(t, "physics2d_trigger_begin", physics.TriggerBeginEvent{}.Name())
	require.Equal(t, "physics2d_trigger_end", physics.TriggerEndEvent{}.Name())
}

func TestComponentNames(t *testing.T) {
	t.Parallel()
	require.Equal(t, "transform_2d", phycomp.Transform2D{}.Name())
	require.Equal(t, "velocity_2d", phycomp.Velocity2D{}.Name())
	require.Equal(t, "physics_body_2d", phycomp.PhysicsBody2D{}.Name())
	require.Equal(t, "physics_singleton_tag", phycomp.PhysicsSingletonTag{}.Name())
	require.Equal(t, "active_contacts", phycomp.ActiveContacts{}.Name())
	require.Equal(t, "shape_common_2d", phycomp.ShapeCommon{}.Name())
	require.Equal(t, "circle_geom_2d", phycomp.CircleGeom{}.Name())
	require.Equal(t, "box_geom_2d", phycomp.BoxGeom{}.Name())
	require.Equal(t, "polygon_geom_2d", phycomp.PolygonGeom{}.Name())
	require.Equal(t, "chain_geom_2d", phycomp.ChainGeom{}.Name())
	require.Equal(t, "edge_geom_2d", phycomp.EdgeGeom{}.Name())
	require.Equal(t, "capsule_geom_2d", phycomp.CapsuleGeom{}.Name())
}
