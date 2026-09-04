package scenario

import (
	"math"

	"github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Defaults covers the single most likely class of porting bug: a Go zero value
// silently standing in for a C default. Box2D's b2DefaultBodyDef enables the
// body, wakes it, allows sleeping and sets gravityScale to 1, but Go bool zero
// values are false and float zero values are 0. The plugin bridges that gap in
// two places — NewPhysicsBody2D and PhysicsBody2D.UnmarshalJSON — and this
// scenario pins both, then proves the flags actually reach Box2D by watching a
// constructor-built body fall while a struct-literal body does not.
func Defaults() harness.Scenario {
	var s struct {
		constructed cardinal.EntityID
		literal     cardinal.EntityID
		roundTrip   cardinal.EntityID
	}

	return harness.Scenario{
		Name: "defaults",
		Setup: func(c *harness.Ctx) {
			// Built the documented way: should behave like a normal Box2D body.
			s.constructed = c.Spawn("constructed", -6, 20,
				body(physics.BodyTypeDynamic, circle(0.5)))

			// Built with a bare struct literal: Active/Awake/SleepingAllowed are
			// false and GravityScale is 0, so Box2D should never simulate it.
			s.literal = c.Spawn("struct-literal", 0, 20, physics.PhysicsBody2D{
				BodyType: physics.BodyTypeDynamic,
				Shapes:   []physics.ColliderShape{circle(0.5)},
			})

			// Built by decoding a payload that omits every flag, the way an old
			// snapshot would. UnmarshalJSON must fill in the Box2D defaults, so
			// this body must behave exactly like the constructed one.
			s.roundTrip = c.Spawn("json-defaulted", 6, 20, decodeBody(
				`{"body_type":2,"shapes":[{"shape_type":1,"radius":0.5,"density":1,`+
					`"friction":0.3,"category_bits":18446744073709551615,`+
					`"mask_bits":18446744073709551615}]}`))
		},
		Steps: []harness.Step{
			{Tick: 2, Do: checkConstructorDefaults},
			{Tick: 2, Do: checkZeroValueDefaults},
			{Tick: 2, Do: checkJSONDefaults},
			{Tick: 2, Do: checkValidation},
			{Tick: 90, Do: func(c *harness.Ctx) {
				// 90 ticks at 60 Hz is 1.5 s: free fall covers ~11 m, so a live
				// body is well clear of its spawn height and a disabled one has
				// not budged at all.
				c.Less("constructor-built body falls", c.Pos(s.constructed).Y, 12.0)
				c.Near("struct-literal body never simulates", c.Pos(s.literal).Y, 20.0, 1e-9)
				c.Near("struct-literal body keeps zero velocity", c.Vel(s.literal).Y, 0, 1e-9)
				c.Less("json-defaulted body falls", c.Pos(s.roundTrip).Y, 12.0)
				c.Near("json defaults match constructor defaults",
					c.Pos(s.roundTrip).Y, c.Pos(s.constructed).Y, 1e-6)
			}},
		},
	}
}

func checkConstructorDefaults(c *harness.Ctx) {
	pb := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, circle(1))

	c.True("NewPhysicsBody2D sets Active=true", pb.Active, "Active=false: body would never simulate")
	c.True("NewPhysicsBody2D sets Awake=true", pb.Awake, "Awake=false: body would spawn asleep")
	c.True("NewPhysicsBody2D sets SleepingAllowed=true", pb.SleepingAllowed,
		"SleepingAllowed=false: body would never sleep, unlike b2DefaultBodyDef")
	c.Near("NewPhysicsBody2D sets GravityScale=1", pb.GravityScale, 1, 0)

	// Box2D's own defaults for these two are false, so Go's zero value is correct
	// here and the constructor must not "helpfully" turn them on.
	c.False("NewPhysicsBody2D leaves Bullet=false", pb.Bullet,
		"Bullet defaults on: every body would pay for CCD")
	c.False("NewPhysicsBody2D leaves FixedRotation=false", pb.FixedRotation,
		"FixedRotation defaults on: nothing would ever rotate")

	c.Near("NewPhysicsBody2D leaves LinearDamping=0", pb.LinearDamping, 0, 0)
	c.Near("NewPhysicsBody2D leaves AngularDamping=0", pb.AngularDamping, 0, 0)
	c.Int("NewPhysicsBody2D keeps the shapes it was given", len(pb.Shapes), 1)

	for _, kind := range []physics.BodyType{
		physics.BodyTypeStatic, physics.BodyTypeDynamic,
		physics.BodyTypeKinematic, physics.BodyTypeManual,
	} {
		got := physcomp.NewPhysicsBody2D(kind, circle(1))
		c.True("NewPhysicsBody2D preserves the body type it was given",
			got.BodyType == kind, "asked for %d, got %d", kind, got.BodyType)
	}
}

func checkZeroValueDefaults(c *harness.Ctx) {
	// This is not a bug, it is the trap: the plugin documents that a bare literal
	// produces a disabled, sleeping, gravity-less body. Pinning it here means the
	// day someone "fixes" the zero value, this check tells you the docs went stale.
	var pb physics.PhysicsBody2D
	c.False("zero-value PhysicsBody2D has Active=false", pb.Active,
		"zero value now enables the body; NewPhysicsBody2D docs are stale")
	c.False("zero-value PhysicsBody2D has Awake=false", pb.Awake, "zero value changed")
	c.False("zero-value PhysicsBody2D has SleepingAllowed=false", pb.SleepingAllowed,
		"zero value changed")
	c.Near("zero-value PhysicsBody2D has GravityScale=0", pb.GravityScale, 0, 0)
}

func checkJSONDefaults(c *harness.Ctx) {
	// A payload with no flags at all: every defaulted field must come back as
	// Box2D's default, not Go's zero value.
	bare := decodeBodyInto(c, "json: minimal payload decodes",
		`{"body_type":2,"shapes":[{"shape_type":1,"radius":1}]}`)
	c.True("json: absent Active defaults to true", bare.Active, "got false")
	c.True("json: absent Awake defaults to true", bare.Awake, "got false")
	c.True("json: absent SleepingAllowed defaults to true", bare.SleepingAllowed, "got false")
	c.Near("json: absent GravityScale defaults to 1", bare.GravityScale, 1, 0)
	c.False("json: absent Bullet stays false", bare.Bullet, "got true")
	c.False("json: absent FixedRotation stays false", bare.FixedRotation, "got true")

	// Explicit falses and zeros must survive. If UnmarshalJSON used a plain bool
	// instead of *bool it would be unable to tell "absent" from "false" and would
	// overwrite these with true — the exact bug the pointer fields exist to avoid.
	explicit := decodeBodyInto(c, "json: explicit-flags payload decodes",
		`{"body_type":2,"active":false,"awake":false,"sleeping_allowed":false,`+
			`"gravity_scale":0,"bullet":true,"fixed_rotation":true,`+
			`"shapes":[{"shape_type":1,"radius":1}]}`)
	c.False("json: explicit Active=false is preserved", explicit.Active,
		"explicit false was overwritten with the default true")
	c.False("json: explicit Awake=false is preserved", explicit.Awake,
		"explicit false was overwritten with the default true")
	c.False("json: explicit SleepingAllowed=false is preserved", explicit.SleepingAllowed,
		"explicit false was overwritten with the default true")
	c.Near("json: explicit GravityScale=0 is preserved", explicit.GravityScale, 0, 0)
	c.True("json: explicit Bullet=true is preserved", explicit.Bullet, "got false")
	c.True("json: explicit FixedRotation=true is preserved", explicit.FixedRotation, "got false")

	// Full round-trip through the component's own wire encoding.
	original := physcomp.NewPhysicsBody2D(physics.BodyTypeKinematic,
		withFilter(withRestitution(withFriction(circle(1.25), 0.7), 0.4), 0x0F, 0xF0, -3))
	original.Bullet = true
	original.FixedRotation = true
	original.Active = false
	original.SleepingAllowed = false
	original.LinearDamping = 0.25
	original.AngularDamping = 0.5
	original.GravityScale = 2.5

	raw := original.MarshalWire()
	decodedAny, err := physics.PhysicsBody2D{}.UnmarshalWire(raw)
	if !c.NoError("wire: UnmarshalWire succeeds", err) {
		return
	}
	got, ok := decodedAny.(physics.PhysicsBody2D)
	if !c.True("wire: UnmarshalWire returns a PhysicsBody2D", ok,
		"got %T instead", decodedAny) {
		return
	}

	c.True("wire round-trip preserves BodyType", got.BodyType == original.BodyType,
		"got %d, want %d", got.BodyType, original.BodyType)
	c.True("wire round-trip preserves Active", got.Active == original.Active,
		"got %v, want %v", got.Active, original.Active)
	c.True("wire round-trip preserves Awake", got.Awake == original.Awake,
		"got %v, want %v", got.Awake, original.Awake)
	c.True("wire round-trip preserves SleepingAllowed",
		got.SleepingAllowed == original.SleepingAllowed,
		"got %v, want %v", got.SleepingAllowed, original.SleepingAllowed)
	c.True("wire round-trip preserves Bullet", got.Bullet == original.Bullet,
		"got %v, want %v", got.Bullet, original.Bullet)
	c.True("wire round-trip preserves FixedRotation",
		got.FixedRotation == original.FixedRotation,
		"got %v, want %v", got.FixedRotation, original.FixedRotation)
	c.Near("wire round-trip preserves GravityScale", got.GravityScale, original.GravityScale, 0)
	c.Near("wire round-trip preserves LinearDamping", got.LinearDamping, original.LinearDamping, 0)
	c.Near("wire round-trip preserves AngularDamping", got.AngularDamping, original.AngularDamping, 0)

	if !c.Int("wire round-trip preserves shape count", len(got.Shapes), len(original.Shapes)) {
		return
	}
	o, g := original.Shapes[0], got.Shapes[0]
	c.True("wire round-trip preserves ShapeType", g.ShapeType == o.ShapeType,
		"got %d, want %d", g.ShapeType, o.ShapeType)
	c.Near("wire round-trip preserves Radius", g.Radius, o.Radius, 0)
	c.Near("wire round-trip preserves Friction", g.Friction, o.Friction, 0)
	c.Near("wire round-trip preserves Restitution", g.Restitution, o.Restitution, 0)
	c.True("wire round-trip preserves CategoryBits", g.CategoryBits == o.CategoryBits,
		"got %#x, want %#x", g.CategoryBits, o.CategoryBits)
	c.True("wire round-trip preserves MaskBits", g.MaskBits == o.MaskBits,
		"got %#x, want %#x", g.MaskBits, o.MaskBits)
	c.True("wire round-trip preserves GroupIndex", g.GroupIndex == o.GroupIndex,
		"got %d, want %d", g.GroupIndex, o.GroupIndex)
}

func checkValidation(c *harness.Ctx) {
	valid := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, circle(1))
	c.NoError("Validate accepts a well-formed body", valid.Validate())

	noShapes := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic)
	c.HasError("Validate rejects a body with no shapes", noShapes.Validate())

	badKind := physcomp.NewPhysicsBody2D(physics.BodyType(0), circle(1))
	c.HasError("Validate rejects body type 0", badKind.Validate())

	badKind2 := physcomp.NewPhysicsBody2D(physics.BodyType(99), circle(1))
	c.HasError("Validate rejects an out-of-range body type", badKind2.Validate())

	nanGravity := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, circle(1))
	nanGravity.GravityScale = math.NaN()
	c.HasError("Validate rejects NaN GravityScale", nanGravity.Validate())

	infDamping := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, circle(1))
	infDamping.LinearDamping = math.Inf(1)
	c.HasError("Validate rejects infinite LinearDamping", infDamping.Validate())

	badShape := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, physics.ColliderShape{})
	c.HasError("Validate rejects shape type 0", badShape.Validate())

	unknownShape := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic,
		physics.ColliderShape{ShapeType: physics.ShapeType(99), Radius: 1})
	c.HasError("Validate rejects an out-of-range shape type", unknownShape.Validate())

	nanRadius := circle(math.NaN())
	c.HasError("Validate rejects NaN radius",
		physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, nanRadius).Validate())

	nanOffset := circle(1)
	nanOffset.LocalOffset = vec(math.NaN(), 0)
	c.HasError("Validate rejects NaN LocalOffset",
		physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, nanOffset).Validate())

	c.NoError("Transform2D.Validate accepts finite values",
		physics.Transform2D{Position: vec(1, 2), Rotation: 0.5}.Validate())
	c.HasError("Transform2D.Validate rejects NaN position",
		physics.Transform2D{Position: vec(math.NaN(), 0)}.Validate())
	c.HasError("Transform2D.Validate rejects Inf rotation",
		physics.Transform2D{Rotation: math.Inf(-1)}.Validate())

	c.NoError("Velocity2D.Validate accepts finite values",
		physics.Velocity2D{Linear: vec(1, 2), Angular: 3}.Validate())
	c.HasError("Velocity2D.Validate rejects NaN linear velocity",
		physics.Velocity2D{Linear: vec(0, math.NaN())}.Validate())
	c.HasError("Velocity2D.Validate rejects NaN angular velocity",
		physics.Velocity2D{Angular: math.NaN()}.Validate())

	// A shape carrying geometry for a different shape type is accepted: the tag
	// selects which fields Box2D reads, so this documents that Validate is a
	// finiteness guard, not a tagged-union consistency check.
	mixed := circle(1)
	mixed.HalfExtents = vec(5, 5)
	c.NoError("Validate ignores geometry that does not match ShapeType",
		physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, mixed).Validate())
}

// decodeBody decodes a PhysicsBody2D payload, panicking on malformed literals in
// this file (they are compile-time constants, so a failure is a typo, not data).
func decodeBody(payload string) physics.PhysicsBody2D {
	var pb physics.PhysicsBody2D
	if err := json.Unmarshal([]byte(payload), &pb); err != nil {
		panic("scenario/defaults: bad JSON literal: " + err.Error())
	}
	return pb
}

// decodeBodyInto decodes a payload and records the decode itself as a check.
func decodeBodyInto(c *harness.Ctx, check, payload string) physics.PhysicsBody2D {
	var pb physics.PhysicsBody2D
	err := json.Unmarshal([]byte(payload), &pb)
	c.NoError(check, err)
	return pb
}
