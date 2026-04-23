package internal

import (
	"errors"
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
)

// CreateBody creates a body on the C-side Box2D world via cbridge. It does not attach shapes;
// use AttachColliderFixtures next.
func CreateBody(
	entityID cardinal.EntityID,
	transform component.Transform2D,
	velocity component.Velocity2D,
	pb component.PhysicsBody2D,
) error {
	if !cbridge.WorldExists() {
		return errors.New("physics2d: world does not exist")
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

	bodyType := mapBodyType(pb.BodyType)

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

	ok := cbridge.CreateBody(
		uint32(entityID), bodyType,
		transform.Position.X, transform.Position.Y, transform.Rotation,
		vx, vy, av,
		pb.LinearDamping, pb.AngularDamping, pb.GravityScale,
		pb.Active, pb.Awake, pb.SleepingAllowed,
		pb.Bullet, pb.FixedRotation,
	)
	if !ok {
		return errors.New("physics2d: CreateBody failed on C side (world may be locked or entity already exists)")
	}

	return nil
}

// AttachColliderFixtures creates one shape per ColliderShape on the C-side body identified
// by entityID. shapeIndex is the slice index i in shapes. Local offsets and rotations are
// applied so geometry defined in shape space is placed correctly in body space.
func AttachColliderFixtures(entityID cardinal.EntityID, shapes []component.ColliderShape) error {
	if len(shapes) == 0 {
		return errors.New("physics2d: collider has no shapes")
	}
	for i := range shapes {
		if err := shapes[i].Validate(); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	for i := range shapes {
		if err := attachShape(entityID, i, shapes[i]); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	return nil
}

// CreateBodyWithCollider creates a body and attaches all shapes. If shape attachment
// fails, the body is destroyed and an error is returned.
func CreateBodyWithCollider(
	entityID cardinal.EntityID,
	transform component.Transform2D,
	velocity component.Velocity2D,
	pb component.PhysicsBody2D,
) error {
	if err := CreateBody(entityID, transform, velocity, pb); err != nil {
		return err
	}
	if err := AttachColliderFixtures(entityID, pb.Shapes); err != nil {
		cbridge.DestroyBody(uint32(entityID))
		return err
	}
	return nil
}

// mapBodyType maps ECS BodyType to cbridge body type constants.
// Bridge: 1=static, 2=dynamic, 3=kinematic. Manual maps to kinematic.
func mapBodyType(t component.BodyType) uint8 {
	switch t {
	case component.BodyTypeStatic:
		return 1 // static
	case component.BodyTypeDynamic:
		return 2 // dynamic
	case component.BodyTypeKinematic, component.BodyTypeManual:
		return 3 // kinematic
	default:
		return 1 // static fallback
	}
}

// attachShape dispatches to the appropriate cbridge.AddXxxShape based on shape type.
//
//nolint:gocognit,funlen // Keep all shape types in one function.
func attachShape(
	entityID cardinal.EntityID,
	shapeIndex int,
	sh component.ColliderShape,
) error {
	eid := uint32(entityID)

	switch sh.ShapeType {
	case component.ShapeTypeCircle:
		ok := cbridge.AddCircleShape(
			eid, shapeIndex,
			sh.LocalOffset.X, sh.LocalOffset.Y, sh.Radius,
			sh.IsSensor, sh.Friction, sh.Restitution, sh.Density,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddCircleShape failed")
		}

	case component.ShapeTypeBox:
		ok := cbridge.AddBoxShape(
			eid, shapeIndex,
			sh.LocalOffset.X, sh.LocalOffset.Y,
			sh.HalfExtents.X, sh.HalfExtents.Y, sh.LocalRotation,
			sh.IsSensor, sh.Friction, sh.Restitution, sh.Density,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddBoxShape failed")
		}

	case component.ShapeTypeConvexPolygon:
		verts := make([]cbridge.Vec2, len(sh.Vertices))
		for i := range sh.Vertices {
			v := shapePointToBodySpace(sh.Vertices[i], sh.LocalOffset, sh.LocalRotation)
			verts[i] = cbridge.Vec2{X: v.X, Y: v.Y}
		}
		ok := cbridge.AddPolygonShape(
			eid, shapeIndex,
			verts,
			0, 0, 0, // offset/rotation already baked into verts
			sh.IsSensor, sh.Friction, sh.Restitution, sh.Density,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddPolygonShape failed")
		}

	case component.ShapeTypeStaticChain:
		pts := make([]cbridge.Vec2, len(sh.ChainPoints))
		for i := range sh.ChainPoints {
			v := shapePointToBodySpace(sh.ChainPoints[i], sh.LocalOffset, sh.LocalRotation)
			pts[i] = cbridge.Vec2{X: v.X, Y: v.Y}
		}
		ok := cbridge.AddChainShape(
			eid, shapeIndex,
			pts, false,
			sh.Friction, sh.Restitution,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddChainShape failed")
		}

	case component.ShapeTypeStaticChainLoop:
		pts := make([]cbridge.Vec2, len(sh.ChainPoints))
		for i := range sh.ChainPoints {
			v := shapePointToBodySpace(sh.ChainPoints[i], sh.LocalOffset, sh.LocalRotation)
			pts[i] = cbridge.Vec2{X: v.X, Y: v.Y}
		}
		ok := cbridge.AddChainShape(
			eid, shapeIndex,
			pts, true,
			sh.Friction, sh.Restitution,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddChainLoopShape failed")
		}

	case component.ShapeTypeEdge:
		v1 := shapePointToBodySpace(sh.EdgeVertices[0], sh.LocalOffset, sh.LocalRotation)
		v2 := shapePointToBodySpace(sh.EdgeVertices[1], sh.LocalOffset, sh.LocalRotation)
		ok := cbridge.AddSegmentShape(
			eid, shapeIndex,
			v1.X, v1.Y, v2.X, v2.Y,
			sh.IsSensor, sh.Friction, sh.Restitution, sh.Density,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddSegmentShape failed")
		}

	case component.ShapeTypeCapsule:
		c1 := shapePointToBodySpace(sh.CapsuleCenter1, sh.LocalOffset, sh.LocalRotation)
		c2 := shapePointToBodySpace(sh.CapsuleCenter2, sh.LocalOffset, sh.LocalRotation)
		ok := cbridge.AddCapsuleShape(
			eid, shapeIndex,
			c1.X, c1.Y, c2.X, c2.Y, sh.Radius,
			sh.IsSensor, sh.Friction, sh.Restitution, sh.Density,
			sh.CategoryBits, sh.MaskBits, sh.GroupIndex,
		)
		if !ok {
			return errors.New("AddCapsuleShape failed")
		}

	default:
		return fmt.Errorf("unknown shape_type %d", sh.ShapeType)
	}

	return nil
}

// shapePointToBodySpace maps a point from shape-local space into body-local space using
// LocalOffset and LocalRotation (radians, CCW +Y up) on the ColliderShape.
func shapePointToBodySpace(p, offset component.Vec2, localRot float64) component.Vec2 {
	c, s := math.Cos(localRot), math.Sin(localRot)
	rx := p.X*c - p.Y*s
	ry := p.X*s + p.Y*c
	return component.Vec2{X: rx + offset.X, Y: ry + offset.Y}
}
