package internal

import (
	"errors"
	"fmt"
	"math"

	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/query"
)

// CreateBody builds a Box2D body from ECS rigidbody/transform/velocity. It does not add
// fixtures; use AttachColliderFixtures next. Body.UserData is set to *query.BodyUserData.
func CreateBody(
	world *box2d.B2World,
	entityID cardinal.EntityID,
	transform component.Transform2D,
	velocity component.Velocity2D,
	rigid component.Rigidbody2D,
) (*box2d.B2Body, error) {
	if world == nil {
		return nil, errors.New("physics2d: nil world")
	}
	if err := transform.Validate(); err != nil {
		return nil, fmt.Errorf("physics2d: transform: %w", err)
	}
	if err := velocity.Validate(); err != nil {
		return nil, fmt.Errorf("physics2d: velocity: %w", err)
	}
	if err := rigid.Validate(); err != nil {
		return nil, fmt.Errorf("physics2d: rigidbody: %w", err)
	}
	def := box2d.MakeB2BodyDef()
	def.Type = mapBodyType(rigid.BodyType)
	def.Position = box2d.MakeB2Vec2(transform.Position.X, transform.Position.Y)
	def.Angle = transform.Rotation
	// Manual bodies have zero velocity in Box2D; ECS Velocity2D is a gameplay concept for them.
	if rigid.BodyType != component.BodyTypeManual {
		def.LinearVelocity = box2d.MakeB2Vec2(velocity.Linear.X, velocity.Linear.Y)
		def.AngularVelocity = velocity.Angular
	}
	def.LinearDamping = rigid.LinearDamping
	def.AngularDamping = rigid.AngularDamping
	def.GravityScale = rigid.GravityScale
	def.Active = rigid.Active
	def.Awake = rigid.Awake
	def.AllowSleep = rigid.SleepingAllowed
	def.Bullet = rigid.Bullet
	def.FixedRotation = rigid.FixedRotation
	def.UserData = &query.BodyUserData{EntityID: entityID}

	body := world.CreateBody(&def)
	if body == nil {
		return nil, errors.New("physics2d: CreateBody returned nil (world may be locked)")
	}
	return body, nil
}

// AttachColliderFixtures creates one fixture per ColliderShape in order. ShapeIndex in
// query.FixtureUserData is the slice index i in collider.Shapes. Local offsets and rotations are
// applied so geometry defined in shape space is placed correctly in body space.
func AttachColliderFixtures(body *box2d.B2Body, entityID cardinal.EntityID, collider component.Collider2D) error {
	if body == nil {
		return errors.New("physics2d: nil body")
	}
	if len(collider.Shapes) == 0 {
		return errors.New("physics2d: collider has no shapes")
	}
	if err := collider.Validate(); err != nil {
		return fmt.Errorf("physics2d: collider: %w", err)
	}
	bodyType := body.GetType()
	for i := range collider.Shapes {
		if err := attachFixture(body, entityID, i, collider.Shapes[i], bodyType); err != nil {
			return fmt.Errorf("physics2d: shapes[%d]: %w", i, err)
		}
	}
	return nil
}

// CreateBodyWithCollider creates a body and attaches all fixtures. If fixture attachment
// fails, the body is destroyed and an error is returned.
func CreateBodyWithCollider(
	world *box2d.B2World,
	entityID cardinal.EntityID,
	transform component.Transform2D,
	velocity component.Velocity2D,
	rigid component.Rigidbody2D,
	collider component.Collider2D,
) (*box2d.B2Body, error) {
	body, err := CreateBody(world, entityID, transform, velocity, rigid)
	if err != nil {
		return nil, err
	}
	if err := AttachColliderFixtures(body, entityID, collider); err != nil {
		world.DestroyBody(body)
		return nil, err
	}
	return body, nil
}

func mapBodyType(t component.BodyType) uint8 {
	switch t {
	case component.BodyTypeStatic:
		return box2d.B2BodyType.B2_staticBody
	case component.BodyTypeKinematic, component.BodyTypeManual:
		return box2d.B2BodyType.B2_kinematicBody
	case component.BodyTypeDynamic:
		return box2d.B2BodyType.B2_dynamicBody
	default:
		return box2d.B2BodyType.B2_staticBody
	}
}

func attachFixture(
	body *box2d.B2Body,
	entityID cardinal.EntityID,
	shapeIndex int,
	sh component.ColliderShape,
	bodyType uint8,
) error {
	//nolint:exhaustive // We only care about static chain, static chain loop, and edge shapes
	switch sh.ShapeType {
	case component.ShapeTypeStaticChain, component.ShapeTypeStaticChainLoop, component.ShapeTypeEdge:
		if bodyType == box2d.B2BodyType.B2_dynamicBody {
			return fmt.Errorf("%d shape type cannot be used on dynamic bodies (zero mass)", sh.ShapeType)
		}
	}

	shape, err := buildShape(sh)
	if err != nil {
		return err
	}

	def := box2d.MakeB2FixtureDef()
	def.Shape = shape
	def.UserData = &query.FixtureUserData{EntityID: entityID, ShapeIndex: shapeIndex}
	def.Friction = sh.Friction
	def.Restitution = sh.Restitution
	def.Density = sh.Density
	def.IsSensor = sh.IsSensor
	def.Filter.CategoryBits = sh.CategoryBits
	def.Filter.MaskBits = sh.MaskBits
	def.Filter.GroupIndex = sh.GroupIndex

	body.CreateFixtureFromDef(&def)
	return nil
}

func buildShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	switch sh.ShapeType {
	case component.ShapeTypeCircle:
		return buildCircleShape(sh)
	case component.ShapeTypeBox:
		return buildBoxShape(sh)
	case component.ShapeTypeConvexPolygon:
		return buildPolygonShape(sh)
	case component.ShapeTypeStaticChain:
		return buildChainShape(sh)
	case component.ShapeTypeStaticChainLoop:
		return buildChainLoopShape(sh)
	case component.ShapeTypeEdge:
		return buildEdgeShape(sh)
	default:
		return nil, fmt.Errorf("unknown shape_type %d", sh.ShapeType)
	}
}

func buildCircleShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	c := box2d.NewB2CircleShape()
	c.M_radius = sh.Radius
	c.M_p = box2d.MakeB2Vec2(sh.LocalOffset.X, sh.LocalOffset.Y)
	return c, nil
}

func buildBoxShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	poly := box2d.NewB2PolygonShape()
	center := box2d.MakeB2Vec2(sh.LocalOffset.X, sh.LocalOffset.Y)
	poly.SetAsBoxFromCenterAndAngle(sh.HalfExtents.X, sh.HalfExtents.Y, center, sh.LocalRotation)
	return poly, nil
}

func buildPolygonShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	verts := make([]box2d.B2Vec2, len(sh.Vertices))
	for i := range sh.Vertices {
		verts[i] = shapePointToBodySpace(sh.Vertices[i], sh.LocalOffset, sh.LocalRotation)
	}
	poly := box2d.NewB2PolygonShape()
	poly.Set(verts, len(sh.Vertices))
	return poly, nil
}

func buildChainShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	pts := make([]box2d.B2Vec2, len(sh.ChainPoints))
	for i := range sh.ChainPoints {
		pts[i] = shapePointToBodySpace(sh.ChainPoints[i], sh.LocalOffset, sh.LocalRotation)
	}
	chain := box2d.MakeB2ChainShape()
	chain.CreateChain(pts, len(sh.ChainPoints))
	return &chain, nil
}

func buildChainLoopShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	pts := make([]box2d.B2Vec2, len(sh.ChainPoints))
	for i := range sh.ChainPoints {
		pts[i] = shapePointToBodySpace(sh.ChainPoints[i], sh.LocalOffset, sh.LocalRotation)
	}
	chain := box2d.MakeB2ChainShape()
	chain.CreateLoop(pts, len(sh.ChainPoints))
	return &chain, nil
}

func buildEdgeShape(sh component.ColliderShape) (box2d.B2ShapeInterface, error) {
	v1 := shapePointToBodySpace(sh.EdgeVertices[0], sh.LocalOffset, sh.LocalRotation)
	v2 := shapePointToBodySpace(sh.EdgeVertices[1], sh.LocalOffset, sh.LocalRotation)
	edge := box2d.MakeB2EdgeShape()
	edge.Set(v1, v2)
	return &edge, nil
}

// shapePointToBodySpace maps a point from shape-local space into body-local space using
// LocalOffset and LocalRotation (radians, CCW +Y up) on the ColliderShape.
func shapePointToBodySpace(p, offset component.Vec2, localRot float64) box2d.B2Vec2 {
	c, s := math.Cos(localRot), math.Sin(localRot)
	rx := p.X*c - p.Y*s
	ry := p.X*s + p.Y*c
	return box2d.MakeB2Vec2(rx+offset.X, ry+offset.Y)
}
