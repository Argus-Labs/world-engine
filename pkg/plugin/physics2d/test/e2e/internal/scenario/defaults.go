package scenario

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/argus-labs/world-engine/pkg/immutable"

	"github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
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
			// this body must behave exactly like the constructed one.
			shape, err := json.Marshal(circle(0.5).Spawn(c))
			if err != nil {
				panic(err)
			}
			s.roundTrip = c.Spawn("json-defaulted", 6, 20, decodeBody(
				fmt.Sprintf(`{"body_type":2,"shapes":[%s]}`, shape)))
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
	pb := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, physcomp.Circle(0.1))

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
		got := physcomp.NewPhysicsBody2D(kind, physcomp.Circle(0.1))
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

	// Full round-trip through the component's own wire encoding, with every field of a
	// shape set to something other than its default, plus a chain for its points.
	shape := physcomp.Circle(0.7).At(vec(1.5, -2), 0.25).Filter(0x0F, 0xF0).Group(-3).Material(0.7, 0.4, 2)
	original := physcomp.NewPhysicsBody2D(physics.BodyTypeKinematic, shape,
		physcomp.ChainLoop(vec(0, 0), vec(1, 0), vec(2, 1), vec(3, 0)))
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
	c.True("wire round-trip preserves the shape's kind and geometry", g.Kind() == o.Kind() && g.Radius() == o.Radius(),
		"got %v %v, want %v %v", g.Kind(), g.Radius(), o.Kind(), o.Radius())
	c.NearVec("wire round-trip preserves LocalOffset", g.LocalOffset, o.LocalOffset, 0)
	c.Near("wire round-trip preserves LocalRotation", g.LocalRotation, o.LocalRotation, 0)
	c.True("wire round-trip preserves the filter bits",
		g.CategoryBits == o.CategoryBits && g.MaskBits == o.MaskBits,
		"got %#x/%#x, want %#x/%#x", g.CategoryBits, g.MaskBits, o.CategoryBits, o.MaskBits)
	c.Int("wire round-trip preserves GroupIndex", int(g.GroupIndex), int(o.GroupIndex))
	c.True("wire round-trip preserves the material",
		g.Friction == o.Friction && g.Restitution == o.Restitution && g.Density == o.Density && g.IsSensor == o.IsSensor,
		"got %+v, want %+v", g, o)
	c.True("wire round-trip preserves a chain's kind and points", got.Shapes.At(1).Equal(original.Shapes.At(1)),
		"got %+v, want %+v", got.Shapes.At(1), original.Shapes.At(1))
}

func checkValidation(c *harness.Ctx) {
	slot := physcomp.Circle(0.1)
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
	c.NoError("Validate allows many shapes, identical ones too",
		physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, slot, slot, physcomp.Circle(0.2)).Validate())

	nan := math.NaN()
	c.HasError("Validate rejects a NaN radius", physcomp.Circle(nan).Validate())
	c.HasError("Validate rejects NaN friction", physcomp.Circle(0.5).Material(nan, 0, 1).Validate())
	c.HasError("Validate rejects negative friction", physcomp.Circle(0.5).Material(-0.1, 0, 1).Validate())
	c.HasError("Validate rejects negative restitution", physcomp.Circle(0.5).Material(0, -1, 1).Validate())
	c.HasError("Validate rejects negative density", physcomp.Circle(0.5).Material(0, 0, -1).Validate())
	c.HasError("Validate rejects a polygon of two vertices", physcomp.Polygon(vec(0, 0), vec(1, 0)).Validate())
	nine := make([]physics.Vec2, physics.MaxPolygonVertices+1)
	for i := range nine {
		nine[i] = vec(float64(i), float64(i*i))
	}
	c.HasError("Validate rejects a polygon of nine vertices", physcomp.Polygon(nine...).Validate())
	c.NoError("Validate accepts a chain of four finite points",
		physcomp.Chain(vec(0, 0), vec(1, 0), vec(2, 0), vec(3, 0)).Validate())
	c.HasError("Validate rejects a chain of three points", physcomp.Chain(vec(0, 0), vec(1, 0), vec(2, 0)).Validate())
	c.HasError("Validate rejects a zero radius", physcomp.Circle(0).Validate())
	c.HasError("Validate rejects a negative radius", physcomp.Circle(-1).Validate())
	c.HasError("Validate rejects a zero half-extent", physcomp.Box(0, 1).Validate())
	c.HasError("Validate rejects a negative half-extent", physcomp.Box(1, -1).Validate())
	c.HasError("Validate rejects an edge whose endpoints meet", physcomp.Edge(vec(1, 1), vec(1, 1)).Validate())
	c.HasError("Validate rejects a capsule whose endpoints meet", physcomp.Capsule(vec(1, 1), vec(1, 1), 0.5).Validate())
	c.HasError("Validate rejects a capsule of zero radius", physcomp.Capsule(vec(0, 0), vec(0, 1), 0).Validate())

	checkJSONRoundTrip(c)
}

// checkJSONRoundTrip pins the JSON codec on the components holding an immutable.Slice.
// The elements live in an unexported field, so without a codec on the slice each of these
// encodes as {} and silently loses its list, while the surrounding struct still looks fine.
func checkJSONRoundTrip(c *harness.Ctx) {
	body := physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic,
		physcomp.Circle(0.7), physcomp.Circle(0.9).At(vec(1, 2), 0.5))
	back, err := (physcomp.PhysicsBody2D{}).UnmarshalWire(body.MarshalWire())
	if c.NoError("a body round-trips the wire", err) {
		got, isBody := back.(physcomp.PhysicsBody2D)
		c.True("a body keeps its shapes through the wire", isBody && body.Shapes.EqualFunc(got.Shapes, physcomp.Shape.Equal),
			"got %+v", back)
	}
	if data, err := json.Marshal(body); c.NoError("a body with slots marshals", err) {
		var back physics.PhysicsBody2D
		if c.NoError("a body with slots unmarshals", json.Unmarshal(data, &back)) {
			c.True("a body keeps its shape list through JSON",
				body.Shapes.EqualFunc(back.Shapes, physcomp.Shape.Equal),
				"%d shapes became %d: %s", body.Shapes.Len(), back.Shapes.Len(), data)
		}
	}

	chain := physcomp.ChainLoop(vec(0, 0), vec(1, 0), vec(2, 1), vec(3, 0))
	if data, err := json.Marshal(chain); c.NoError("a chain marshals", err) {
		var back physcomp.Shape
		if c.NoError("a chain unmarshals", json.Unmarshal(data, &back)) {
			c.True("a chain keeps its points and kind through JSON", chain.Equal(back),
				"%d points became %d: %s", len(chain.Points()), len(back.Points()), data)
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
	shapes := map[string]physics.Shape{
		"Circle":    physics.Circle(0.5),
		"Box":       physics.Box(1, 2),
		"Polygon":   physics.Polygon(vec(0, 0), vec(1, 0), vec(0, 1)),
		"Chain":     physics.Chain(line4()...),
		"ChainLoop": physics.ChainLoop(line4()...),
		"Edge":      physics.Edge(vec(0, 0), vec(1, 0)),
		"Capsule":   physics.Capsule(vec(0, 0), vec(1, 0), 0.25),
	}
	for name, sh := range shapes {
		c.True(name+" carries Box2D's default material and filter",
			sh.Friction == 0.6 && sh.Restitution == 0 && sh.Density == 1 && !sh.IsSensor &&
				sh.CategoryBits == 1 && sh.MaskBits == ^uint64(0) && sh.GroupIndex == 0,
			"got %+v", sh)
		c.NoError(name+" validates", sh.Validate())
	}

	d := physics.Box(1, 1).Sensor(true).Material(0.1, 0.2, 0.3).Filter(0x2, 0x4).Group(-1)
	c.True("the options set exactly what they say",
		d.IsSensor && d.Friction == 0.1 && d.Restitution == 0.2 && d.Density == 0.3 &&
			d.CategoryBits == 0x2 && d.MaskBits == 0x4 && d.GroupIndex == -1,
		"got %+v", d)
	c.True("Box reports its half extents", d.Kind() == physics.ShapeKindBox && d.HalfExtents() == vec(1, 1), "got %+v", d)
	c.True("Circle reports its radius", physics.Circle(0.5).Radius() == 0.5, "got %v", physics.Circle(0.5).Radius())
	ea, eb := physics.Edge(vec(0, 0), vec(1, 0)).Endpoints()
	c.True("Edge reports its endpoints", ea == vec(0, 0) && eb == vec(1, 0), "got %v %v", ea, eb)
	capsule := physics.Capsule(vec(0, 0), vec(1, 0), 0.25)
	ca, cb := capsule.Endpoints()
	c.True("Capsule reports its endpoints and radius",
		ca == vec(0, 0) && cb == vec(1, 0) && capsule.Radius() == 0.25, "got %+v", capsule)
	c.True("getters of another kind read zero", d.Radius() == 0 && d.Points() == nil && d.Vertices() == nil &&
		physics.Circle(0.5).HalfExtents() == vec(0, 0), "got %+v", d)

	chain := physics.Chain(line4()...)
	c.True("Chain copies its points", chain.Kind() == physics.ShapeKindChain &&
		slices.Equal(chain.Points(), line4()), "got %+v", chain)
	c.True("ChainLoop is its own kind", physics.ChainLoop(line4()...).Kind() == physics.ShapeKindChainLoop,
		"got %v", physics.ChainLoop(line4()...).Kind())

	tri := physics.Polygon(vec(0, 0), vec(1, 0), vec(0, 1))
	c.Int("Polygon counts its vertices", len(tri.Vertices()), 3)
	c.HasError("the zero Shape fails validation", physics.Shape{}.Validate())
	for _, n := range []int{9, 300} {
		err := physics.Polygon(make([]physics.Vec2, n)...).Validate()
		c.True(fmt.Sprintf("a polygon of %d vertices is refused, naming %d", n, n),
			err != nil && strings.Contains(err.Error(), strconv.Itoa(n)), "got %v", err)
	}
	c.HasError("a three-point Chain fails validation", physics.Chain(vec(0, 0), vec(1, 0), vec(2, 0)).Validate())
	c.HasError("a Chain marked as a sensor fails validation", physics.Chain(line4()...).Sensor(true).Validate())
	c.HasError("a ChainLoop marked as a sensor fails validation", physics.ChainLoop(line4()...).Sensor(true).Validate())
	c.NoError("a solid Chain still validates", physics.Chain(line4()...).Validate())
	c.NoError("Polygon of three vertices validates", tri.Validate())

	placed := physics.Circle(0.9).At(vec(2, 3), 0.5)
	c.True("At places the shape", placed.LocalOffset == vec(2, 3) && placed.LocalRotation == 0.5, "got %+v", placed)
	reshaped := placed.Material(0.1, 0.2, 0.3).Filter(0x2, 0x4).Reshape(physics.Box(1, 2))
	c.True("Reshape swaps the geometry and keeps the rest",
		reshaped.Kind() == physics.ShapeKindBox && reshaped.HalfExtents() == vec(1, 2) && reshaped.Radius() == 0 &&
			reshaped.LocalOffset == vec(2, 3) && reshaped.Friction == 0.1 && reshaped.CategoryBits == 0x2,
		"got %+v", reshaped)
}

// line4 is the shortest polyline Box2D accepts.
func line4() []physics.Vec2 {
	return []physics.Vec2{vec(0, 0), vec(1, 0), vec(2, 0), vec(3, 0)}
}
