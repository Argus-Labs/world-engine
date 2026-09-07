package internal

import (
	"errors"
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
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

// AttachColliderFixtures creates one Box2D shape per slot on the body identified by
// entityID. Slot i becomes fixture i. Each slot's shape entity is resolved through the
// runtime's ShapeMirror; a slot whose shape entity is missing fails the whole attach.
func (rt *Runtime) AttachColliderFixtures(entityID cardinal.EntityID, slots []component.ShapeSlot) error {
	if len(slots) == 0 {
		return errors.New("physics2d: collider has no shapes")
	}
	for i := range slots {
		if err := rt.validateSlot(slots[i]); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	for i := range slots {
		sh, err := rt.resolveSlot(slots[i])
		if err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
		if err := rt.attachShape(entityID, i, slots[i], sh); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	return nil
}

// validateSlot checks the slot's own fields and, once resolved, the shape entity's components.
func (rt *Runtime) validateSlot(slot component.ShapeSlot) error {
	if err := slot.Validate(); err != nil {
		return err
	}
	sh, err := rt.resolveSlot(slot)
	if err != nil {
		return err
	}
	return sh.validate()
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
func makeShapeDef(shapeIndex int, c component.ShapeCommon) box2d.ShapeDef {
	def := box2d.DefaultShapeDef()
	def.UserData = uint64(uint32(shapeIndex)) //nolint:gosec // shape index is small and non-negative
	def.Material.Friction = c.Friction
	def.Material.Restitution = c.Restitution
	def.Density = c.Density
	def.IsSensor = c.IsSensor
	def.EnableSensorEvents = true
	def.EnableContactEvents = true
	def.Filter.CategoryBits = c.CategoryBits
	def.Filter.MaskBits = c.MaskBits
	def.Filter.GroupIndex = int(c.GroupIndex)
	return def
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

// attachShape dispatches on the resolved shape's kind and creates the Box2D shape for slot
// shapeIndex, applying the slot's local offset and rotation.
//
//nolint:funlen // Keep all shape kinds in one function.
func (rt *Runtime) attachShape(
	entityID cardinal.EntityID,
	shapeIndex int,
	slot component.ShapeSlot,
	sh ResolvedShape,
) error {
	bodyID, ok := rt.Bodies[entityID]
	if !ok {
		return errors.New("physics2d: body does not exist")
	}

	switch sh.Kind {
	case ShapeKindCircle:
		def := makeShapeDef(shapeIndex, sh.Common)
		circle := box2d.Circle{
			Center: box2d.Vec2{X: slot.LocalOffset.X, Y: slot.LocalOffset.Y},
			Radius: sh.Circle.Radius,
		}
		rt.registerShape(entityID, shapeIndex, rt.World.CreateCircleShape(bodyID, &def, &circle))

	case ShapeKindBox:
		def := makeShapeDef(shapeIndex, sh.Common)
		center := box2d.Vec2{X: slot.LocalOffset.X, Y: slot.LocalOffset.Y}
		rot := box2d.MakeRot(slot.LocalRotation)
		polygon := box2d.MakeOffsetBox(sh.Box.HalfExtents.X, sh.Box.HalfExtents.Y, center, rot)
		rt.registerShape(entityID, shapeIndex, rt.World.CreatePolygonShape(bodyID, &def, &polygon))

	case ShapeKindPolygon:
		n := int(sh.Polygon.Count)
		if n < 3 || n > box2d.MaxPolygonVertices {
			return errors.New("AddPolygonShape failed")
		}
		verts := make([]box2d.Vec2, n)
		for i := range n {
			v := shapePointToBodySpace(sh.Polygon.Vertices[i], slot.LocalOffset, slot.LocalRotation)
			verts[i] = box2d.Vec2{X: v.X, Y: v.Y}
		}
		hull := box2d.ComputeHull(verts)
		if hull.Count == 0 {
			return errors.New("AddPolygonShape failed") // degenerate polygon
		}
		def := makeShapeDef(shapeIndex, sh.Common)
		polygon := box2d.MakePolygon(&hull, 0)
		rt.registerShape(entityID, shapeIndex, rt.World.CreatePolygonShape(bodyID, &def, &polygon))

	case ShapeKindChain:
		src := sh.Chain.Points
		pts := make([]box2d.Vec2, len(src))
		for i := range src {
			v := shapePointToBodySpace(src[i], slot.LocalOffset, slot.LocalRotation)
			pts[i] = box2d.Vec2{X: v.X, Y: v.Y}
		}
		def := box2d.DefaultChainDef()
		def.UserData = uint64(uint32(shapeIndex)) //nolint:gosec // shape index is small and non-negative
		def.Points = pts
		def.IsLoop = sh.Chain.Loop
		material := box2d.DefaultSurfaceMaterial()
		material.Friction = sh.Common.Friction
		material.Restitution = sh.Common.Restitution
		def.Materials = []box2d.SurfaceMaterial{material}
		def.Filter.CategoryBits = sh.Common.CategoryBits
		def.Filter.MaskBits = sh.Common.MaskBits
		def.Filter.GroupIndex = int(sh.Common.GroupIndex)
		chainID := rt.World.CreateChain(bodyID, &def)
		rt.Chains[entityID] = append(rt.Chains[entityID], chainID)
		// Chain slots keep a null ShapeID: mutable per-shape setters skip them, matching
		// the CGO bridge (chains were not registered in its shapes[] array either).
		rt.registerShape(entityID, shapeIndex, box2d.ShapeID{})

	case ShapeKindEdge:
		v1 := shapePointToBodySpace(sh.Edge.A, slot.LocalOffset, slot.LocalRotation)
		v2 := shapePointToBodySpace(sh.Edge.B, slot.LocalOffset, slot.LocalRotation)
		def := makeShapeDef(shapeIndex, sh.Common)
		segment := box2d.Segment{
			Point1: box2d.Vec2{X: v1.X, Y: v1.Y},
			Point2: box2d.Vec2{X: v2.X, Y: v2.Y},
		}
		rt.registerShape(entityID, shapeIndex, rt.World.CreateSegmentShape(bodyID, &def, &segment))

	case ShapeKindCapsule:
		c1 := shapePointToBodySpace(sh.Capsule.A, slot.LocalOffset, slot.LocalRotation)
		c2 := shapePointToBodySpace(sh.Capsule.B, slot.LocalOffset, slot.LocalRotation)
		def := makeShapeDef(shapeIndex, sh.Common)
		capsule := box2d.Capsule{
			Center1: box2d.Vec2{X: c1.X, Y: c1.Y},
			Center2: box2d.Vec2{X: c2.X, Y: c2.Y},
			Radius:  sh.Capsule.Radius,
		}
		rt.registerShape(entityID, shapeIndex, rt.World.CreateCapsuleShape(bodyID, &def, &capsule))

	default:
		return fmt.Errorf("shape entity %d carries no geometry component", slot.Shape)
	}

	return nil
}

// shapePointToBodySpace maps a point from shape-local space into body-local space using the
// slot's LocalOffset and LocalRotation (radians, CCW +Y up).
func shapePointToBodySpace(p, offset component.Vec2, localRot float64) component.Vec2 {
	c, s := math.Cos(localRot), math.Sin(localRot)
	rx := p.X*c - p.Y*s
	ry := p.X*s + p.Y*c
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
	for _, chainID := range rt.Chains[entityID] {
		rt.World.DestroyChain(chainID)
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
