package internal

import (
	"errors"
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// CreateBody creates a body in this runtime's Box2D world. It does not attach shapes;
// use AttachColliderFixtures next.
func (rt *Runtime) CreateBody(
	entityID cardinal.EntityID,
	transform component.Transform2D,
	velocity component.Velocity2D,
	pb component.PhysicsBody2D,
) error {
	if rt.World == nil {
		return errors.New("physics2d: world does not exist")
	}
	if _, exists := rt.Bodies[entityID]; exists {
		return errors.New("physics2d: CreateBody failed (entity already has a body)")
	}
	if err := transform.Validate(); err != nil {
		return fmt.Errorf("physics2d: transform: %w", err)
	}
	if err := velocity.Validate(); err != nil {
		return fmt.Errorf("physics2d: velocity: %w", err)
	}
	if err := pb.Validate(); err != nil {
		return fmt.Errorf("physics2d: physics_body: %w", err)
	}

	// Manual bodies have zero velocity in Box2D; ECS Velocity2D is a gameplay concept for them.
	// FixedRotation bodies have zero angular velocity in Box2D; Box2D's FixedRotation flag
	// only prevents torques from generating angular velocity but still integrates any explicit
	// value. Zeroing matches Box2D's own SetFixedRotation() behavior and standard engine
	// practice (Unity freezeRotation, Godot lock_rotation). ECS Velocity2D.Angular is
	// preserved as a gameplay concept; if FixedRotation is later disabled, the ECS angular
	// velocity is naturally applied via the reconciler.
	vx, vy, av := velocity.Linear.X, velocity.Linear.Y, velocity.Angular
	if pb.BodyType == component.BodyTypeManual {
		vx, vy, av = 0, 0, 0
	} else if pb.FixedRotation {
		av = 0
	}

	def := box2d.DefaultBodyDef()
	def.Type = mapBodyType(pb.BodyType)
	def.Position = box2d.Vec2{X: transform.Position.X, Y: transform.Position.Y}
	def.Rotation = box2d.MakeRot(transform.Rotation)
	def.LinearVelocity = box2d.Vec2{X: vx, Y: vy}
	def.AngularVelocity = av
	def.LinearDamping = pb.LinearDamping
	def.AngularDamping = pb.AngularDamping
	def.GravityScale = pb.GravityScale
	def.UserData = uint64(entityID)
	def.EnableSleep = pb.SleepingAllowed
	def.IsAwake = pb.Awake
	def.MotionLocks.AngularZ = pb.FixedRotation
	def.IsBullet = pb.Bullet
	def.IsEnabled = pb.Active

	rt.Bodies[entityID] = rt.World.CreateBody(&def)
	return nil
}

// AttachColliderFixtures creates one Box2D shape per entry of shapes on the body identified
// by entityID. Shape i becomes fixture i. Every shape is validated first, and one the engine
// still refuses rolls back the fixtures this call made, so a failed attach leaves the body as
// it was found.
func (rt *Runtime) AttachColliderFixtures(
	entityID cardinal.EntityID, shapes immutable.Slice[component.Shape],
) error {
	if shapes.Len() == 0 {
		return errors.New("physics2d: collider has no shapes")
	}
	for i, sh := range shapes.All() {
		if err := sh.Validate(); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	shapeMark, chainMark := len(rt.Shapes[entityID]), len(rt.Chains[entityID])
	for i, sh := range shapes.All() {
		if err := rt.attachShape(entityID, i, sh); err != nil {
			rt.rollbackFixtures(entityID, shapeMark, chainMark)
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	return nil
}

// rollbackFixtures destroys the fixtures an attach pass added past the marks. Validation
// does not catch everything the engine refuses — a polygon of collinear points passes every
// component check and then yields an empty hull — so a later slot can fail after earlier
// ones are already on the body. Chains go first: destroying one frees its segment shapes.
func (rt *Runtime) rollbackFixtures(entityID cardinal.EntityID, shapeMark, chainMark int) {
	chains := rt.Chains[entityID]
	for _, ch := range chains[chainMark:] {
		rt.World.DestroyChain(ch.ID)
	}
	shapes := rt.Shapes[entityID]
	for _, sid := range shapes[shapeMark:] {
		if !sid.IsNull() {
			rt.World.DestroyShape(sid, false)
		}
	}
	rt.Chains[entityID] = chains[:chainMark]
	rt.Shapes[entityID] = shapes[:shapeMark]
	if chainMark == 0 {
		delete(rt.Chains, entityID)
	}
	if shapeMark == 0 {
		delete(rt.Shapes, entityID)
	}
	if bodyID, ok := rt.Bodies[entityID]; ok {
		rt.World.ApplyBodyMassFromShapes(bodyID)
	}
}

// CreateBodyWithCollider creates a body and attaches all shapes. If shape attachment
// fails, the body is destroyed and an error is returned.
func (rt *Runtime) CreateBodyWithCollider(
	entityID cardinal.EntityID,
	transform component.Transform2D,
	velocity component.Velocity2D,
	pb component.PhysicsBody2D,
) error {
	if err := rt.CreateBody(entityID, transform, velocity, pb); err != nil {
		return err
	}
	if err := rt.AttachColliderFixtures(entityID, pb.Shapes); err != nil {
		rt.DestroyEntityBody(entityID)
		return err
	}
	return nil
}

// ChainSlot is the Box2D chain built for one chain-type slot of a body.
type ChainSlot struct {
	Index int
	ID    box2d.ChainID
}

// DestroyEntityBody destroys the Box2D body for entityID (with all attached shapes and
// chains) and clears the runtime's body/shape/chain tracking for it. No-op when the entity
// has no body.
func (rt *Runtime) DestroyEntityBody(entityID cardinal.EntityID) {
	bodyID, ok := rt.Bodies[entityID]
	if !ok || rt.World == nil {
		return
	}
	rt.World.DestroyBody(bodyID)
	delete(rt.Bodies, entityID)
	delete(rt.Shapes, entityID)
	delete(rt.Chains, entityID)
}

// mapBodyType maps ECS BodyType to Box2D body types. Manual maps to kinematic
// (same historical mapping as the CGO bridge).
func mapBodyType(t component.BodyType) box2d.BodyType {
	switch t {
	case component.BodyTypeStatic:
		return box2d.StaticBody
	case component.BodyTypeDynamic:
		return box2d.DynamicBody
	case component.BodyTypeKinematic, component.BodyTypeManual:
		return box2d.KinematicBody
	default:
		return box2d.StaticBody // static fallback
	}
}

// makeShapeDef builds the common shape definition. Mirrors the CGO bridge, which enabled
// sensor and contact events on every shape (Box2D ignores EnableContactEvents on sensors,
// and requires EnableSensorEvents on both the sensor and the visitor shape).
func makeShapeDef(shapeIndex int, s component.Shape) box2d.ShapeDef {
	def := box2d.DefaultShapeDef()
	def.UserData = uint64(uint32(shapeIndex)) //nolint:gosec // shape index is small and non-negative
	def.Material.Friction = s.Friction
	def.Material.Restitution = s.Restitution
	def.Density = s.Density
	def.IsSensor = s.IsSensor
	def.EnableSensorEvents = true
	def.EnableContactEvents = true
	def.Filter = shapeFilter(s)
	return def
}

// shapeFilter is the shape's collision filter as Box2D takes it.
func shapeFilter(s component.Shape) box2d.Filter {
	return box2d.Filter{CategoryBits: s.CategoryBits, MaskBits: s.MaskBits, GroupIndex: int(s.GroupIndex)}
}

// registerShape stores a shape id at collider slot shapeIndex, growing the per-entity slot
// slice with null sentinels as needed (chain slots stay null).
func (rt *Runtime) registerShape(entityID cardinal.EntityID, shapeIndex int, sid box2d.ShapeID) {
	slots := rt.Shapes[entityID]
	for len(slots) <= shapeIndex {
		slots = append(slots, box2d.ShapeID{})
	}
	slots[shapeIndex] = sid
	rt.Shapes[entityID] = slots
}

// attachShape creates the Box2D shape for fixture shapeIndex from s, applying its local
// offset and rotation.
//
//nolint:funlen // Keep all shape kinds in one function.
func (rt *Runtime) attachShape(entityID cardinal.EntityID, shapeIndex int, s component.Shape) error {
	bodyID, ok := rt.Bodies[entityID]
	if !ok {
		return errors.New("physics2d: body does not exist")
	}

	switch s.Kind() {
	case component.ShapeKindCircle:
		def := makeShapeDef(shapeIndex, s)
		circle := box2d.Circle{
			Center: box2d.Vec2{X: s.LocalOffset.X, Y: s.LocalOffset.Y},
			Radius: s.Radius(),
		}
		rt.registerShape(entityID, shapeIndex, rt.World.CreateCircleShape(bodyID, &def, &circle))

	case component.ShapeKindBox:
		def := makeShapeDef(shapeIndex, s)
		center := box2d.Vec2{X: s.LocalOffset.X, Y: s.LocalOffset.Y}
		rot := box2d.MakeRot(s.LocalRotation)
		half := s.HalfExtents()
		polygon := box2d.MakeOffsetBox(half.X, half.Y, center, rot)
		rt.registerShape(entityID, shapeIndex, rt.World.CreatePolygonShape(bodyID, &def, &polygon))

	case component.ShapeKindPolygon:
		vertices := s.Vertices()
		n := len(vertices)
		if n < 3 || n > box2d.MaxPolygonVertices {
			return errors.New("AddPolygonShape failed")
		}
		verts := make([]box2d.Vec2, n)
		for i, p := range vertices {
			v := shapePointToBodySpace(p, s.LocalOffset, s.LocalRotation)
			verts[i] = box2d.Vec2{X: v.X, Y: v.Y}
		}
		hull := box2d.ComputeHull(verts)
		if hull.Count == 0 {
			return errors.New("AddPolygonShape failed") // degenerate polygon
		}
		def := makeShapeDef(shapeIndex, s)
		polygon := box2d.MakePolygon(&hull, 0)
		rt.registerShape(entityID, shapeIndex, rt.World.CreatePolygonShape(bodyID, &def, &polygon))

	case component.ShapeKindChain, component.ShapeKindChainLoop:
		points := s.Points()
		pts := make([]box2d.Vec2, len(points))
		for i, p := range points {
			v := shapePointToBodySpace(p, s.LocalOffset, s.LocalRotation)
			pts[i] = box2d.Vec2{X: v.X, Y: v.Y}
		}
		def := box2d.DefaultChainDef()
		def.UserData = uint64(uint32(shapeIndex)) //nolint:gosec // shape index is small and non-negative
		def.Points = pts
		def.IsLoop = s.Kind() == component.ShapeKindChainLoop
		material := box2d.DefaultSurfaceMaterial()
		material.Friction = s.Friction
		material.Restitution = s.Restitution
		def.Materials = []box2d.SurfaceMaterial{material}
		def.Filter = shapeFilter(s)
		chainID := rt.World.CreateChain(bodyID, &def)
		rt.Chains[entityID] = append(rt.Chains[entityID], ChainSlot{Index: shapeIndex, ID: chainID})
		// Chain slots keep a null ShapeID; in-place edits go through the chain id instead.
		rt.registerShape(entityID, shapeIndex, box2d.ShapeID{})

	case component.ShapeKindEdge:
		a, b := s.Endpoints()
		v1 := shapePointToBodySpace(a, s.LocalOffset, s.LocalRotation)
		v2 := shapePointToBodySpace(b, s.LocalOffset, s.LocalRotation)
		def := makeShapeDef(shapeIndex, s)
		segment := box2d.Segment{
			Point1: box2d.Vec2{X: v1.X, Y: v1.Y},
			Point2: box2d.Vec2{X: v2.X, Y: v2.Y},
		}
		rt.registerShape(entityID, shapeIndex, rt.World.CreateSegmentShape(bodyID, &def, &segment))

	case component.ShapeKindCapsule:
		a, b := s.Endpoints()
		c1 := shapePointToBodySpace(a, s.LocalOffset, s.LocalRotation)
		c2 := shapePointToBodySpace(b, s.LocalOffset, s.LocalRotation)
		def := makeShapeDef(shapeIndex, s)
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: c1.X, Y: c1.Y},
			Center2: box2d.Vec2{X: c2.X, Y: c2.Y},
			Radius:  s.Radius(),
		}
		rt.registerShape(entityID, shapeIndex, rt.World.CreateCapsuleShape(bodyID, &def, &capsule))

	default:
		return fmt.Errorf("shape %d has no geometry", shapeIndex)
	}

	return nil
}

// shapePointToBodySpace maps a point from shape-local space into body-local space using the
// slot's LocalOffset and LocalRotation (radians, CCW +Y up). Each product is rounded through
// float64(...) so the compiler cannot fuse it with the add: a fused multiply-add rounds
// once, not twice, and would give different vertices on arm64 than on amd64. This is the
// one path shape points take into the engine, which promises bit-identical results.
func shapePointToBodySpace(p, offset component.Vec2, localRot float64) component.Vec2 {
	c, s := math.Cos(localRot), math.Sin(localRot)
	rx := float64(p.X*c) - float64(p.Y*s)
	ry := float64(p.X*s) + float64(p.Y*c)
	return component.Vec2{X: rx + offset.X, Y: ry + offset.Y}
}

// destroyAllShapesForEntity destroys every non-chain shape and every chain on the entity's
// body, then recomputes mass. Mirrors the CGO bridge's bridge_destroy_all_shapes: chains are
// destroyed first (chain destruction frees its segment shapes), then remaining shapes are
// enumerated from the body itself.
func (rt *Runtime) destroyAllShapesForEntity(entityID cardinal.EntityID) {
	bodyID, ok := rt.Bodies[entityID]
	if !ok {
		return
	}
	for _, ch := range rt.Chains[entityID] {
		rt.World.DestroyChain(ch.ID)
	}
	delete(rt.Chains, entityID)

	count := rt.World.BodyShapeCount(bodyID)
	if count > 0 {
		sids := make([]box2d.ShapeID, count)
		got := rt.World.BodyShapes(bodyID, sids)
		for i := range got {
			rt.World.DestroyShape(sids[i], false)
		}
	}
	delete(rt.Shapes, entityID)

	// Recalculate mass after removing all shapes.
	rt.World.ApplyBodyMassFromShapes(bodyID)
}
