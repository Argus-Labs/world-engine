package physics2d_test

import (
	"encoding/json"
	"math"
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
// PhysicsBody2D.Validate
// ---------------------------------------------------------------------------

func TestValidate_PhysicsBody2D_Valid(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Circle(0.3))
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
		Shapes:   immutable.SliceOf(phycomp.Circle(0.3)),
	}
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "body_type")
}

func TestValidate_PhysicsBody2D_NaNLinearDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Circle(0.3))
	pb.LinearDamping = math.NaN()
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "linear_damping")
}

func TestValidate_PhysicsBody2D_InfAngularDamping(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Circle(0.3))
	pb.AngularDamping = math.Inf(1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "angular_damping")
}

func TestValidate_PhysicsBody2D_InfGravityScale(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Circle(0.3))
	pb.GravityScale = math.Inf(-1)
	err := pb.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "gravity_scale")
}

func TestValidate_PhysicsBody2D_InvalidSlot(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic,
		phycomp.Circle(0.3).At(phycomp.Vec2{X: math.NaN()}, 0))
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
		require.NoError(t, phycomp.NewPhysicsBody2D(bt, phycomp.Circle(0.3)).Validate(), "body type %d", bt)
	}
}

// ---------------------------------------------------------------------------
// NewPhysicsBody2D constructor defaults
// ---------------------------------------------------------------------------

func TestNewPhysicsBody2D_Defaults(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeDynamic, phycomp.Circle(0.3))
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

func TestNewPhysicsBody2D_MultipleShapes(t *testing.T) {
	t.Parallel()
	pb := phycomp.NewPhysicsBody2D(phycomp.BodyTypeStatic,
		phycomp.Circle(0.3), phycomp.Circle(0.4).At(phycomp.Vec2{X: 1}, 0))
	require.Equal(t, 2, pb.Shapes.Len())
	require.Equal(t, phycomp.Circle(0.4).At(phycomp.Vec2{X: 1}, 0), pb.Shapes.At(1))
}

// ---------------------------------------------------------------------------
// PhysicsBody2D JSON unmarshal (defaults for missing fields)
// ---------------------------------------------------------------------------

func TestUnmarshalPhysicsBody2D_MissingFieldsGetDefaults(t *testing.T) {
	t.Parallel()
	// Minimal JSON: only body_type and shapes
	data := `{
		"body_type": 2,
		"shapes": [{"kind": 1, "radius": 0.7}]
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
	require.Equal(t, immutable.SliceOf(phycomp.Circle(0.7)), pb.Shapes)
}

func TestUnmarshalPhysicsBody2D_ExplicitFalsePreserved(t *testing.T) {
	t.Parallel()
	data := `{
		"body_type": 2,
		"active": false,
		"awake": false,
		"sleeping_allowed": false,
		"gravity_scale": 0,
		"shapes": [{"kind": 1, "radius": 0.7}]
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
			{"kind": 1, "radius": 0.7, "local_offset": {"x": 1, "y": 2}, "local_rotation": 0.5}
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
	require.Equal(t, phycomp.Circle(0.7).At(phycomp.Vec2{X: 1, Y: 2}, 0.5), pb.Shapes.At(0))
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

// ---------------------------------------------------------------------------
// Wire round trip
// ---------------------------------------------------------------------------

// The body's wire code is hand-written until the generator runs; every field of every shape
// kind has to come back.
func TestWire_PhysicsBody2D_RoundTrip(t *testing.T) {
	t.Parallel()
	pts := []phycomp.Vec2{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 1}, {X: 3, Y: 0}}
	body := phycomp.NewPhysicsBody2D(phycomp.BodyTypeKinematic,
		phycomp.Circle(0.5).At(phycomp.Vec2{X: 1.5, Y: -2}, 0.25).
			Filter(0x0F, 0xF0).Group(-3).Material(0.7, 0.4, 2).Sensor(true),
		phycomp.Box(1, 2),
		phycomp.ChainLoop(pts...),
		phycomp.Polygon(pts[:3]...),
		phycomp.Edge(phycomp.Vec2{X: 0, Y: 0}, phycomp.Vec2{X: 1, Y: 1}),
		phycomp.Capsule(phycomp.Vec2{X: 0, Y: 0}, phycomp.Vec2{X: 0, Y: 1}, 0.3))
	body.Bullet, body.FixedRotation, body.Active = true, true, false
	body.LinearDamping, body.AngularDamping, body.GravityScale = 0.25, 0.5, 2.5

	raw := body.MarshalWire()
	require.Len(t, raw, body.SizeWire(), "SizeWire must match the encoding")
	back, err := phycomp.PhysicsBody2D{}.UnmarshalWire(raw)
	require.NoError(t, err)
	got, ok := back.(phycomp.PhysicsBody2D)
	require.True(t, ok)
	require.True(t, got.Shapes.EqualFunc(body.Shapes, phycomp.Shape.Equal),
		"shapes differ:\n got %+v\nwant %+v", got.Shapes, body.Shapes)
	got.Shapes, body.Shapes = immutable.Slice[phycomp.Shape]{}, immutable.Slice[phycomp.Shape]{}
	require.Equal(t, body, got, "body fields")
}
