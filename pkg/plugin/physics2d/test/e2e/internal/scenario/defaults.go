package scenario

import (
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/pkg/immutable"

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
				body(c, physics.BodyTypeDynamic, circle(0.5)))

			// Built with a bare struct literal: Active/Awake/SleepingAllowed are
			// false and GravityScale is 0, so Box2D should never simulate it.
			s.literal = c.Spawn("struct-literal", 0, 20, physics.PhysicsBody2D{
				BodyType: physics.BodyTypeDynamic,
				Shapes:   immutable.SliceOf(circle(0.5).Spawn(c)),
			})

			// Built by decoding a payload that omits every flag, the way an old
			// snapshot would. UnmarshalJSON must fill in the Box2D defaults, so
			// this body must behave exactly like the constructed one. The shape is
			// an entity of its own, so the payload only names it.
			shape := circle(0.5).Spawn(c)
			s.roundTrip = c.Spawn("json-defaulted", 6, 20, decodeBody(
				fmt.Sprintf(`{"body_type":2,"shapes":[{"shape":%d}]}`, shape.Shape)))
		},
		Steps: []harness.Step{
			{Tick: 2, Do: checkConstructorDefaults},
			{Tick: 2, Do: checkZeroValueDefaults},
			{Tick: 2, Do: checkJSONDefaults},
			{Tick: 2, Do: checkValidation},
			{Tick: 2, Do: checkShapeConstructors},
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
	pb := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, physics.Slot(1))

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
	c.Int("NewPhysicsBody2D keeps the shapes it was given", pb.Shapes.Len(), 1)

	for _, kind := range []physics.BodyType{
		physics.BodyTypeStatic, physics.BodyTypeDynamic,
		physics.BodyTypeKinematic, physics.BodyTypeManual,
	} {
		got := physcomp.NewPhysicsBody2D(kind, physics.Slot(1))
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
		`{"body_type":2,"shapes":[{"shape":1}]}`)
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
			`"shapes":[{"shape":1}]}`)
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
		physics.Slot(7).At(vec(1.5, -2), 0.25))
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

	if !c.Int("wire round-trip preserves shape count", got.Shapes.Len(), original.Shapes.Len()) {
		return
	}
	o, g := original.Shapes.At(0), got.Shapes.At(0)
	c.True("wire round-trip preserves the slot's shape id", g.Shape == o.Shape,
		"got %d, want %d", g.Shape, o.Shape)
	c.NearVec("wire round-trip preserves LocalOffset", g.LocalOffset, o.LocalOffset, 0)
	c.Near("wire round-trip preserves LocalRotation", g.LocalRotation, o.LocalRotation, 0)

	// The shape entity's own components round-trip separately.
	common := withFilter(withRestitution(withFriction(circle(1.25), 0.7), 0.4), 0x0F, 0xF0, -3).Common
	commonAny, err := physics.ShapeCommon{}.UnmarshalWire(common.MarshalWire())
	if c.NoError("wire: ShapeCommon.UnmarshalWire succeeds", err) {
		gotCommon, isCommon := commonAny.(physics.ShapeCommon)
		c.True("wire round-trip preserves ShapeCommon", isCommon && gotCommon == common,
			"got %+v, want %+v", commonAny, common)
	}
	circleAny, err := physics.CircleGeom{}.UnmarshalWire(physics.CircleGeom{Radius: 1.25}.MarshalWire())
	if c.NoError("wire: CircleGeom.UnmarshalWire succeeds", err) {
		gotCircle, isCircle := circleAny.(physics.CircleGeom)
		c.True("wire round-trip preserves CircleGeom", isCircle && gotCircle == physics.CircleGeom{Radius: 1.25},
			"got %+v", circleAny)
	}
}

func checkValidation(c *harness.Ctx) {
	slot := physics.Slot(1)
	valid := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, slot)
	c.NoError("Validate accepts a well-formed body", valid.Validate())

	noShapes := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic)
	c.HasError("Validate rejects a body with no shapes", noShapes.Validate())

	badKind := physcomp.NewPhysicsBody2D(physics.BodyType(0), slot)
	c.HasError("Validate rejects body type 0", badKind.Validate())

	badKind2 := physcomp.NewPhysicsBody2D(physics.BodyType(99), slot)
	c.HasError("Validate rejects an out-of-range body type", badKind2.Validate())

	nanGravity := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, slot)
	nanGravity.GravityScale = math.NaN()
	c.HasError("Validate rejects NaN GravityScale", nanGravity.Validate())

	infDamping := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, slot)
	infDamping.LinearDamping = math.Inf(1)
	c.HasError("Validate rejects infinite LinearDamping", infDamping.Validate())

	c.HasError("Validate rejects NaN LocalOffset",
		physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, slot.At(vec(math.NaN(), 0), 0)).Validate())
	c.HasError("Validate rejects Inf LocalRotation",
		physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, slot.At(vec(0, 0), math.Inf(1))).Validate())

	// The shape's own components validate themselves; the plugin runs these at
	// fixture attach, since a body only names its shape entity.
	c.HasError("CircleGeom.Validate rejects NaN radius", physics.CircleGeom{Radius: math.NaN()}.Validate())
	c.HasError("ShapeCommon.Validate rejects NaN friction", physics.ShapeCommon{Friction: math.NaN()}.Validate())
	c.HasError("PolygonGeom.Validate rejects two vertices", physics.PolygonGeom{Count: 2}.Validate())
	c.HasError("PolygonGeom.Validate rejects nine vertices",
		physics.PolygonGeom{Count: physics.MaxPolygonVertices + 1}.Validate())
	c.NoError("ChainGeom.Validate accepts finite points",
		physics.ChainGeom{Points: immutable.SliceOf(vec(0, 0), vec(1, 0))}.Validate())
	c.HasError("CircleGeom.Validate rejects a zero radius", physics.CircleGeom{Radius: 0}.Validate())
	c.HasError("CircleGeom.Validate rejects a negative radius", physics.CircleGeom{Radius: -1}.Validate())
	c.HasError("BoxGeom.Validate rejects a zero half-extent",
		physics.BoxGeom{HalfExtents: vec(0, 1)}.Validate())
	c.HasError("BoxGeom.Validate rejects a negative half-extent",
		physics.BoxGeom{HalfExtents: vec(1, -1)}.Validate())
	c.HasError("CapsuleGeom.Validate rejects a zero radius",
		physics.CapsuleGeom{A: vec(0, 0), B: vec(0, 1), Radius: 0}.Validate())

	checkJSONRoundTrip(c)
}

// checkJSONRoundTrip pins the JSON codec on the three components holding an immutable.Slice.
// The elements live in an unexported field, so without a codec on the slice each of these
// encodes as {} and silently loses its list, while the surrounding struct still looks fine.
func checkJSONRoundTrip(c *harness.Ctx) {
	body := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic,
		physcomp.Slot(7), physcomp.Slot(9).At(vec(1, 2), 0.5))
	if data, err := json.Marshal(body); c.NoError("a body with slots marshals", err) {
		var back physics.PhysicsBody2D
		if c.NoError("a body with slots unmarshals", json.Unmarshal(data, &back)) {
			c.True("a body keeps its slot list through JSON",
				immutable.Equal(body.Shapes, back.Shapes),
				"%d slots became %d: %s", body.Shapes.Len(), back.Shapes.Len(), data)
		}
	}

	chain := physcomp.ChainGeom{Points: immutable.SliceOf(vec(0, 0), vec(1, 0), vec(2, 1)), Loop: true}
	if data, err := json.Marshal(chain); c.NoError("a chain marshals", err) {
		var back physcomp.ChainGeom
		if c.NoError("a chain unmarshals", json.Unmarshal(data, &back)) {
			c.True("a chain keeps its points through JSON",
				immutable.Equal(chain.Points, back.Points) && chain.Loop == back.Loop,
				"%d points became %d: %s", chain.Points.Len(), back.Points.Len(), data)
		}
	}

	contacts := physcomp.ActiveContacts{Pairs: immutable.SliceOf(
		physcomp.ContactPairEntry{EntityA: 1, ShapeIndexA: 0, EntityB: 2, ShapeIndexB: 1})}
	if data, err := json.Marshal(contacts); c.NoError("active contacts marshal", err) {
		var back physcomp.ActiveContacts
		if c.NoError("active contacts unmarshal", json.Unmarshal(data, &back)) {
			c.True("active contacts keep their pairs through JSON",
				immutable.Equal(contacts.Pairs, back.Pairs),
				"%d pairs became %d: %s", contacts.Pairs.Len(), back.Pairs.Len(), data)
		}
	}

	var emptyBody physics.PhysicsBody2D
	if data, err := json.Marshal(emptyBody); c.NoError("an empty body marshals", err) {
		var back physics.PhysicsBody2D
		c.NoError("an empty slot list decodes rather than erroring", json.Unmarshal(data, &back))
	}

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

// checkShapeConstructors pins the shape constructors: each carries Box2D's default material
// and filter, the options set exactly what they say, and geometry lands in the right fields.
func checkShapeConstructors(c *harness.Ctx) {
	defaults := physics.ShapeCommon{Friction: 0.6, Density: 1, CategoryBits: 1, MaskBits: ^uint64(0)}
	commons := map[string]physics.ShapeCommon{
		"Circle":    physics.Circle(0.5).Common,
		"Box":       physics.Box(1, 2).Common,
		"Polygon":   physics.Polygon(vec(0, 0), vec(1, 0), vec(0, 1)).Common,
		"Chain":     physics.Chain(vec(0, 0), vec(1, 0)).Common,
		"ChainLoop": physics.ChainLoop(vec(0, 0), vec(1, 0)).Common,
		"Edge":      physics.Edge(vec(0, 0), vec(1, 0)).Common,
		"Capsule":   physics.Capsule(vec(0, 0), vec(1, 0), 0.25).Common,
	}
	for name, common := range commons {
		c.True(name+" carries Box2D's default material and filter", common == defaults,
			"got %+v, want %+v", common, defaults)
	}

	d := physics.Box(1, 1).AsSensor().Material(0.1, 0.2, 0.3).Filter(0x2, 0x4).Group(-1)
	c.True("the options set exactly what they say", d.Common == physics.ShapeCommon{
		IsSensor: true, Friction: 0.1, Restitution: 0.2, Density: 0.3,
		CategoryBits: 0x2, MaskBits: 0x4, GroupIndex: -1,
	}, "got %+v", d.Common)
	c.True("Box stores its half extents", d.Geom == physics.BoxGeom{HalfExtents: vec(1, 1)}, "got %+v", d.Geom)
	c.True("Circle stores its radius", physics.Circle(0.5).Geom == physics.CircleGeom{Radius: 0.5}, "")
	c.True("Edge stores its endpoints",
		physics.Edge(vec(0, 0), vec(1, 0)).Geom == physics.EdgeGeom{A: vec(0, 0), B: vec(1, 0)}, "")
	c.True("Capsule stores its endpoints and radius",
		physics.Capsule(vec(0, 0), vec(1, 0), 0.25).Geom == physics.CapsuleGeom{A: vec(0, 0), B: vec(1, 0), Radius: 0.25}, "")

	line := []physics.Vec2{vec(0, 0), vec(1, 0)}
	chain := physics.Chain(line...).Geom
	c.True("Chain copies its points", immutable.Equal(chain.Points, immutable.SliceOf(line...)) && !chain.Loop, "")
	c.True("ChainLoop closes the polyline", physics.ChainLoop(line...).Geom.Loop, "Loop is false")

	tri := physics.Polygon(vec(0, 0), vec(1, 0), vec(0, 1)).Geom
	c.Int("Polygon counts its vertices", int(tri.Count), 3)
	c.NoError("Polygon of three vertices validates", tri.Validate())
	nine := make([]physics.Vec2, 9)
	for i := range nine {
		nine[i] = vec(float64(i), 0)
	}
	c.HasError("Polygon of nine vertices fails validation", physics.Polygon(nine...).Geom.Validate())

	slot := physics.Slot(9).At(vec(2, 3), 0.5)
	c.True("Slot.At places the slot",
		slot == physics.ShapeSlot{Shape: 9, LocalOffset: vec(2, 3), LocalRotation: 0.5}, "got %+v", slot)
}
