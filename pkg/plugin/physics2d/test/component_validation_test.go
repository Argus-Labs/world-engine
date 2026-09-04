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
// ColliderShape.Validate
// ---------------------------------------------------------------------------

func TestValidate_ColliderShape_ValidCircle(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       0.5,
		Density:      1,
		Friction:     0.3,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_ColliderShape_ValidBox(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeBox,
		HalfExtents:  phycomp.Vec2{X: 1, Y: 0.5},
		Density:      1,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_ColliderShape_ValidPolygon(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType: phycomp.ShapeTypeConvexPolygon,
		Vertices: []phycomp.Vec2{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0.5, Y: 1},
		},
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_ColliderShape_ValidChain(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:     phycomp.ShapeTypeStaticChain,
		ChainGeometry: 7, // resolvability is checked at fixture attach, not here
		CategoryBits:  0xFFFF,
		MaskBits:      0xFFFF,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_ColliderShape_ValidChainLoop(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:     phycomp.ShapeTypeStaticChainLoop,
		ChainGeometry: 7,
		CategoryBits:  0xFFFF,
		MaskBits:      0xFFFF,
	}.Validate()
	require.NoError(t, err)
}

// TestValidate_ColliderShape_RejectsForeignGeometry pins the union discipline: a shape may
// only carry the geometry its own type reads.
func TestValidate_ColliderShape_RejectsForeignGeometry(t *testing.T) {
	t.Parallel()

	box := phycomp.Box(1, 1)
	box.Radius = 0.5 // circle geometry on a box
	err := box.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "radius: not used by shape_type")

	circle := phycomp.Circle(0.5)
	circle.ChainGeometry = 3
	err = circle.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain_geometry: not used by shape_type")

	// Capsule legitimately shares Radius with Circle.
	require.NoError(t, phycomp.Capsule(phycomp.Vec2{}, phycomp.Vec2{X: 1}, 0.25).Validate())
}

// TestShapeConstructors_DefaultsAndOptions: every constructor yields a valid shape carrying
// Box2D's default material and filter, and the options set exactly what they say.
func TestShapeConstructors_DefaultsAndOptions(t *testing.T) {
	t.Parallel()

	shapes := []phycomp.ColliderShape{
		phycomp.Circle(0.5),
		phycomp.Box(1, 2),
		phycomp.Polygon(phycomp.Vec2{}, phycomp.Vec2{X: 1}, phycomp.Vec2{Y: 1}),
		phycomp.Chain(1),
		phycomp.ChainLoop(1),
		phycomp.Edge(phycomp.Vec2{}, phycomp.Vec2{X: 1}),
		phycomp.Capsule(phycomp.Vec2{}, phycomp.Vec2{X: 1}, 0.25),
	}
	for _, s := range shapes {
		require.NoError(t, s.Validate(), "shape_type %d", s.ShapeType)
		require.InDelta(t, 0.6, s.Friction, 0)
		require.InDelta(t, 1.0, s.Density, 0)
		require.Equal(t, uint64(1), s.CategoryBits)
		require.Equal(t, ^uint64(0), s.MaskBits)
		require.False(t, s.IsSensor)
	}

	s := phycomp.Box(1, 1).
		At(phycomp.Vec2{X: 2, Y: 3}, 0.5).
		AsSensor().
		Material(0.1, 0.2, 0.3).
		Filter(0x2, 0x4).
		Group(-1)
	require.Equal(t, phycomp.Vec2{X: 2, Y: 3}, s.LocalOffset)
	require.InDelta(t, 0.5, s.LocalRotation, 0)
	require.True(t, s.IsSensor)
	require.Equal(t, [3]float64{0.1, 0.2, 0.3}, [3]float64{s.Friction, s.Restitution, s.Density})
	require.Equal(t, [2]uint64{0x2, 0x4}, [2]uint64{s.CategoryBits, s.MaskBits})
	require.Equal(t, int32(-1), s.GroupIndex)
	require.NoError(t, s.Validate())
}

func TestValidate_ColliderShape_ValidEdge(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeEdge,
		EdgeVertices: [2]phycomp.Vec2{{X: 0, Y: 0}, {X: 3, Y: 0}},
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}.Validate()
	require.NoError(t, err)
}

func TestValidate_ColliderShape_InvalidShapeType(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{ShapeType: 99}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "shape_type")
}

func TestValidate_ColliderShape_NaNRadius(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType: phycomp.ShapeTypeCircle,
		Radius:    math.NaN(),
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "radius")
}

func TestValidate_ColliderShape_NaNFriction(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType: phycomp.ShapeTypeCircle,
		Friction:  math.NaN(),
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "friction")
}

func TestValidate_ColliderShape_InfRestitution(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:   phycomp.ShapeTypeCircle,
		Restitution: math.Inf(1),
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "restitution")
}

func TestValidate_ColliderShape_InfDensity(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType: phycomp.ShapeTypeCircle,
		Density:   math.Inf(-1),
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "density")
}

func TestValidate_ColliderShape_NaNLocalOffset(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:   phycomp.ShapeTypeCircle,
		LocalOffset: phycomp.Vec2{X: math.NaN(), Y: 0},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "local_offset")
}

func TestValidate_ColliderShape_InfLocalRotation(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:     phycomp.ShapeTypeCircle,
		LocalRotation: math.Inf(1),
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "local_rotation")
}

func TestValidate_ColliderShape_NaNVertex(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType: phycomp.ShapeTypeConvexPolygon,
		Vertices: []phycomp.Vec2{
			{X: 0, Y: 0}, {X: math.NaN(), Y: 0}, {X: 0, Y: 1},
		},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "vertices[1]")
}

func TestValidate_ChainGeometry_NaNPoint(t *testing.T) {
	t.Parallel()
	err := phycomp.ChainGeometry2D{
		Points: []phycomp.Vec2{{X: 0, Y: 0}, {X: 0, Y: math.Inf(1)}},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "points[1]")
}

func TestValidate_ColliderShape_NaNEdgeVertex(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeEdge,
		EdgeVertices: [2]phycomp.Vec2{{X: math.NaN(), Y: 0}, {X: 1, Y: 0}},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "edge_vertices[0]")
}

func TestValidate_ColliderShape_NaNHalfExtents(t *testing.T) {
	t.Parallel()
	err := phycomp.ColliderShape{
		ShapeType:   phycomp.ShapeTypeBox,
		HalfExtents: phycomp.Vec2{X: math.Inf(1), Y: 1},
	}.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "half_extents")
}

// ---------------------------------------------------------------------------
// PhysicsBody2D.Validate
// ---------------------------------------------------------------------------

func TestValidate_PhysicsBody2D_Valid(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       0.5,
		Density:      1,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	})
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
		Shapes: []phycomp.ColliderShape{{
			ShapeType:    phycomp.ShapeTypeCircle,
			Radius:       1,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}},
	}
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "body_type")
}

func TestValidate_PhysicsBody2D_NaNLinearDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       0.5,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	})
	pb.LinearDamping = math.NaN()
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "linear_damping")
}

func TestValidate_PhysicsBody2D_InfAngularDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       0.5,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	})
	pb.AngularDamping = math.Inf(1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "angular_damping")
}

func TestValidate_PhysicsBody2D_InfGravityScale(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       0.5,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	})
	pb.GravityScale = math.Inf(-1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "gravity_scale")
}

func TestValidate_PhysicsBody2D_InvalidShape(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.ColliderShape{
		ShapeType: 99,
	})
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "shapes[0]")
}

func TestValidate_PhysicsBody2D_AllBodyTypes(t *testing.T) {
	t.Parallel()
	shape := phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       0.5,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}
	for _, bt := range []phycomp.BodyType{
		phycomp.BodyTypeStatic,
		phycomp.BodyTypeDynamic,
		phycomp.BodyTypeKinematic,
		phycomp.BodyTypeManual,
	} {
		require.NoError(t, phycomp.NewPhysicsBody2D(bt, shape).Validate(), "body type %d", bt)
	}
}

// ---------------------------------------------------------------------------
// NewPhysicsBody2D constructor defaults
// ---------------------------------------------------------------------------

func TestNewPhysicsBody2D_Defaults(t *testing.T) {
	t.Parallel()
	shape := phycomp.ColliderShape{
		ShapeType:    phycomp.ShapeTypeCircle,
		Radius:       1,
		CategoryBits: 0xFFFF,
		MaskBits:     0xFFFF,
	}
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, shape)
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
	shapes := []phycomp.ColliderShape{
		{ShapeType: phycomp.ShapeTypeCircle, Radius: 0.5, CategoryBits: 0xFFFF, MaskBits: 0xFFFF},
		{ShapeType: phycomp.ShapeTypeBox, HalfExtents: phycomp.Vec2{X: 1, Y: 1}, CategoryBits: 0xFFFF, MaskBits: 0xFFFF},
	}
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeStatic, shapes...)
	require.Len(t, pb.Shapes, 2)
}

// ---------------------------------------------------------------------------
// PhysicsBody2D JSON unmarshal (defaults for missing fields)
// ---------------------------------------------------------------------------

func TestUnmarshalPhysicsBody2D_MissingFieldsGetDefaults(t *testing.T) {
	t.Parallel()
	// Minimal JSON: only body_type and shapes
	data := `{
		"body_type": 2,
		"shapes": [{"shape_type": 1, "radius": 0.5, "category_bits": 65535, "mask_bits": 65535}]
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
}

func TestUnmarshalPhysicsBody2D_ExplicitFalsePreserved(t *testing.T) {
	t.Parallel()
	data := `{
		"body_type": 2,
		"active": false,
		"awake": false,
		"sleeping_allowed": false,
		"gravity_scale": 0,
		"shapes": [{"shape_type": 1, "radius": 0.5, "category_bits": 65535, "mask_bits": 65535}]
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
			{"shape_type": 1, "radius": 1.0, "density": 2.0, "friction": 0.5, "category_bits": 1, "mask_bits": 65535}
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
	require.InDelta(t, 2.0, pb.Shapes[0].Density, 1e-12)
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
}
