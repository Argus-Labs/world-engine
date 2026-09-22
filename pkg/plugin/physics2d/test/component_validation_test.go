package physics2d_test

import (
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/immutable"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	phycomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
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
// ShapeRef.Validate
// ---------------------------------------------------------------------------

func TestValidate_ShapeRef_Valid(t *testing.T) {
	t.Parallel()
	require.NoError(t, phycomp.Ref(7).At(phycomp.Vec2{X: 1, Y: 2}, 0.5).Validate())
	require.NoError(t, phycomp.ShapeRef{}.Validate(), "resolvability is checked at attach, not here")
}

func TestValidate_ShapeRef_NaNLocalOffset(t *testing.T) {
	t.Parallel()
	err := phycomp.Ref(1).At(phycomp.Vec2{X: math.NaN(), Y: 0}, 0).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "local_offset")
}

func TestValidate_ShapeRef_InfLocalRotation(t *testing.T) {
	t.Parallel()
	err := phycomp.Ref(1).At(phycomp.Vec2{}, math.Inf(1)).Validate()
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
}

func TestShapeRef_FilterDefaults(t *testing.T) {
	t.Parallel()
	ref := phycomp.Ref(1)
	require.Equal(t, uint64(1), ref.CategoryBits, "Ref carries Box2D's default category")
	require.Equal(t, ^uint64(0), ref.MaskBits, "Ref carries Box2D's default mask")
	require.Equal(t, int32(0), ref.GroupIndex)

	ref = ref.Filter(0x2, 0x4).Group(-1)
	require.Equal(t, uint64(0x2), ref.CategoryBits)
	require.Equal(t, uint64(0x4), ref.MaskBits)
	require.Equal(t, int32(-1), ref.GroupIndex)

	none := phycomp.Ref(1).Filter(0, 0)
	require.Equal(t, uint64(0), none.CategoryBits, "an explicit zero is kept: collides with nothing")

	var fromJSON phycomp.ShapeRef
	require.NoError(t, json.Unmarshal([]byte(`{"shape": 7}`), &fromJSON))
	require.Equal(t, phycomp.Ref(7), fromJSON, "a JSON ref with no filter gets the defaults")
	require.NoError(t, json.Unmarshal([]byte(`{"shape": 7, "category_bits": 0, "mask_bits": 0}`), &fromJSON))
	require.Equal(t, phycomp.Ref(7).Filter(0, 0), fromJSON, "an explicit JSON zero is kept")
}

func TestValidate_ShapeCommon_Negative(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]phycomp.ShapeCommon{
		"friction":    {Friction: -0.1, Density: 1},
		"restitution": {Restitution: -1, Density: 1},
		"density":     {Density: -1},
	} {
		err := c.Validate()
		require.Error(t, err, name)
		require.Contains(t, err.Error(), name)
	}
	require.NoError(t, phycomp.ShapeCommon{}.Validate(), "zero material is valid")
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
	require.NoError(t, phycomp.ChainGeom{
		Points: immutable.SliceOf(phycomp.Vec2{}, phycomp.Vec2{X: 1}, phycomp.Vec2{X: 1, Y: 1}, phycomp.Vec2{Y: 1}),
		Loop:   true,
	}.Validate())
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

// Slots past Count still take part in ==, so a NaN there would make the shape unequal to
// itself and the mirror would rebuild its fixtures every tick.
func TestValidate_PolygonGeom_UnusedSlotsMustBeFinite(t *testing.T) {
	t.Parallel()
	g := phycomp.PolygonGeom{
		Vertices: [phycomp.MaxPolygonVertices]phycomp.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: math.NaN()}},
		Count:    3,
	}
	err := g.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "vertices[3]")
}

func TestValidate_PolygonGeom_FlatRejected(t *testing.T) {
	t.Parallel()
	g := phycomp.PolygonGeom{
		Vertices: [phycomp.MaxPolygonVertices]phycomp.Vec2{{X: -1, Y: 0}, {X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}},
		Count:    4,
	}
	require.Error(t, g.Validate(), "collinear points have no hull, so attach would fail")
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
		Points: immutable.SliceOf(
			phycomp.Vec2{X: 0, Y: 0}, phycomp.Vec2{X: 0, Y: math.Inf(1)}, phycomp.Vec2{X: 1}, phycomp.Vec2{X: 2},
		),
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "points[1]")
}

func TestValidate_ChainGeom_TooFewPoints(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 2, 3} {
		pts := make([]phycomp.Vec2, n)
		for i := range pts {
			pts[i] = phycomp.Vec2{X: float64(i)}
		}
		err := phycomp.ChainGeom{Points: immutable.SliceOf(pts...)}.Validate()
		require.Error(t, err, "%d points", n)
		require.Contains(t, err.Error(), "at least 4")
	}
}

// ---------------------------------------------------------------------------
// PhysicsBody2D.Validate
// ---------------------------------------------------------------------------

func TestValidate_PhysicsBody2D_Valid(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Ref(3))
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
		Shapes:   immutable.SliceOf(phycomp.Ref(3)),
	}
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "body_type")
}

func TestValidate_PhysicsBody2D_NaNLinearDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Ref(3))
	pb.LinearDamping = math.NaN()
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "linear_damping")
}

func TestValidate_PhysicsBody2D_InfAngularDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Ref(3))
	pb.AngularDamping = math.Inf(1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "angular_damping")
}

func TestValidate_PhysicsBody2D_InfGravityScale(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Ref(3))
	pb.GravityScale = math.Inf(-1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "gravity_scale")
}

func TestValidate_PhysicsBody2D_InvalidSlot(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic,
		phycomp.Ref(3).At(phycomp.Vec2{X: math.NaN()}, 0))
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
		require.NoError(t, phycomp.NewPhysicsBody2D(bt, phycomp.Ref(3)).Validate(), "body type %d", bt)
	}
}

// ---------------------------------------------------------------------------
// NewPhysicsBody2D constructor defaults
// ---------------------------------------------------------------------------

func TestNewPhysicsBody2D_Defaults(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Ref(3))
	require.Equal(t, phycomp.BodyTypeDynamic, pb.BodyType)
	require.InDelta(t, 1.0, pb.GravityScale, 1e-12)
	require.True(t, pb.Active)
	require.True(t, pb.Awake)
	require.True(t, pb.SleepingAllowed)
	require.False(t, pb.Bullet)
	require.False(t, pb.FixedRotation)
	require.InDelta(t, 0.0, pb.LinearDamping, 1e-12)
	require.InDelta(t, 0.0, pb.AngularDamping, 1e-12)
	require.Equal(t, 1, pb.Shapes.Len())
}

func TestValidate_PhysicsBody2D_DuplicateTag(t *testing.T) {
	t.Parallel()
	a, b := phycomp.Ref(3), phycomp.Ref(4)
	a.Tag, b.Tag = "hull", "hull"
	err := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, a, b).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `tag "hull"`)
}

func TestPhysicsBody2D_SlotsByTag(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic)
	pb, err := pb.AddShape("hull", phycomp.Ref(3))
	require.NoError(t, err)
	pb, err = pb.AddShape("", phycomp.Ref(4))
	require.NoError(t, err, "an empty tag adds an untagged slot")
	pb, err = pb.AddShape("aggro", phycomp.Ref(5))
	require.NoError(t, err)
	require.NoError(t, pb.Validate())

	require.Equal(t, 0, pb.ShapeIndex("hull"))
	require.Equal(t, 2, pb.ShapeIndex("aggro"))
	require.Equal(t, -1, pb.ShapeIndex("nope"))
	require.Equal(t, -1, pb.ShapeIndex(""))
	require.Equal(t, "aggro", pb.ShapeTag(2))
	require.Empty(t, pb.ShapeTag(1))
	require.Empty(t, pb.ShapeTag(3))

	_, err = pb.AddShape("hull", phycomp.Ref(9))
	require.ErrorContains(t, err, `"hull"`, "AddShape refuses a used tag")

	replaced, err := pb.ReplaceShape("hull", phycomp.Ref(9))
	require.NoError(t, err)
	require.Equal(t, 3, replaced.Shapes.Len())
	want := phycomp.Ref(9)
	want.Tag = "hull"
	require.Equal(t, want, replaced.Shapes.At(0), "replaces in place and stamps the tag")
	_, err = pb.ReplaceShape("nope", phycomp.Ref(9))
	require.ErrorContains(t, err, `"nope"`, "ReplaceShape refuses an unknown tag")

	removed, err := pb.RemoveShape("hull")
	require.NoError(t, err)
	require.Equal(t, 2, removed.Shapes.Len())
	require.Equal(t, 1, removed.ShapeIndex("aggro"), "later slots move down")
	_, err = pb.RemoveShape("nope")
	require.ErrorContains(t, err, `"nope"`, "RemoveShape refuses an unknown tag")

	// With and Without write through their array, so the receiver, and any ECS component
	// sharing that array, must be copied rather than shifted or overwritten under it.
	original := []phycomp.ShapeRef{
		phycomp.Ref(3), phycomp.Ref(4), phycomp.Ref(5),
	}
	original[0].Tag, original[2].Tag = "hull", "aggro"
	require.Equal(t, original, slices.Collect(pb.Shapes.Values()), "the receiver is untouched")
}

func TestNewPhysicsBody2D_MultipleShapes(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeStatic,
		phycomp.Ref(3), phycomp.Ref(4).At(phycomp.Vec2{X: 1}, 0))
	require.Equal(t, 2, pb.Shapes.Len())
	require.Equal(t, phycomp.Ref(4).At(phycomp.Vec2{X: 1}, 0), pb.Shapes.At(1))
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
	require.Equal(t, immutable.SliceOf(phycomp.Ref(7)), pb.Shapes)
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
	require.Equal(t, 1, pb.Shapes.Len())
	require.Equal(t, phycomp.Ref(7).At(phycomp.Vec2{X: 1, Y: 2}, 0.5), pb.Shapes.At(0))
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

// ---------------------------------------------------------------------------
// Geometry wire round trip
// ---------------------------------------------------------------------------

// PolygonGeom stores a fixed array and travels whole, so the slots past Count are payload
// even though Box2D never reads them and Shape.Vertices never returns them. Nothing above
// this line would notice them going missing: the restore harness reads shapes back through
// the public API, which stops at Count.
func TestWire_PolygonGeom_KeepsSlotsPastCount(t *testing.T) {
	t.Parallel()
	var p phycomp.PolygonGeom
	p.Count = 3
	p.Vertices[0] = phycomp.Vec2{X: 0, Y: 0}
	p.Vertices[1] = phycomp.Vec2{X: 1, Y: 0}
	p.Vertices[2] = phycomp.Vec2{X: 0, Y: 1}
	p.Vertices[phycomp.MaxPolygonVertices-1] = phycomp.Vec2{X: 9, Y: -9}

	back, err := phycomp.PolygonGeom{}.UnmarshalWire(p.MarshalWire())
	require.NoError(t, err)
	require.Equal(t, p, back, "a vertex slot past Count was dropped by the wire")
}

// wireComponent is Cardinal's component wire contract; the interface itself is internal.
type wireComponent interface {
	Name() string
	MarshalWire() []byte
	UnmarshalWire([]byte) (any, error)
}

// The other geometries have no hidden slots, but they are the rest of what a shape entity
// snapshots, so round-trip them together.
func TestWire_Geometry_RoundTrips(t *testing.T) {
	t.Parallel()
	for _, c := range []wireComponent{
		phycomp.ShapeCommon{IsSensor: true, Friction: 0.11, Restitution: 0.22, Density: 0.33},
		phycomp.CircleGeom{Radius: 0.5},
		phycomp.BoxGeom{HalfExtents: phycomp.Vec2{X: 1, Y: 2}},
		phycomp.ChainGeom{
			Points: immutable.SliceOf(phycomp.Vec2{}, phycomp.Vec2{X: 1}, phycomp.Vec2{X: 1, Y: 1}, phycomp.Vec2{Y: 1}),
			Loop:   true,
		},
		phycomp.EdgeGeom{A: phycomp.Vec2{X: -1}, B: phycomp.Vec2{X: 1}},
		phycomp.CapsuleGeom{A: phycomp.Vec2{X: -1}, B: phycomp.Vec2{X: 1}, Radius: 0.25},
	} {
		back, err := c.UnmarshalWire(c.MarshalWire())
		require.NoError(t, err, c.Name())
		require.Equal(t, c, back, c.Name())
	}
}
