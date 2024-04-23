package physics

import (
	"errors"
	"fmt"
	"math"

	"github.com/ByteArena/box2d"

	"pkg.world.dev/world-engine/cardinal/types"
)

const playerGroupIndex int16 = 1
const pickupGroupIndex int16 = 2

//const playerCategory uint16 = 1 << 1
//const pickupCategory uint16 = 1 << 2
//const playerMask uint16 = ^playerCategory
//const pickupMask uint16 = 0xFFFF

type EntityContactPair struct {
	EntityID0 types.EntityID
	EntityID1 types.EntityID
}

type Physics2D struct {
	fixedDeltaTime        float64
	numVelocityIterations int
	numPositionIterations int
	world2D               *box2d.B2World
	rigidBodies           map[types.EntityID]*box2d.B2Body
	ContactsListener      *CustomContactListener
	Contacts              chan EntityContactPair
}

type CustomContactListener struct {
	Physics2DInstance *Physics2D
}

// Could do this the "right" way too with categories and masks.
func (m *CustomContactListener) ShouldCollide(fixtureA *box2d.B2Fixture, fixtureB *box2d.B2Fixture) bool {
	if fixtureA.GetFilterData().GroupIndex == playerGroupIndex && fixtureB.GetFilterData().GroupIndex == playerGroupIndex {
		fmt.Println("Players DO NOT collide with other players.")
		return false
	}

	if fixtureA.GetFilterData().GroupIndex == pickupGroupIndex || fixtureB.GetFilterData().GroupIndex == pickupGroupIndex {
		fmt.Println("Players collide with pickups.")
		return true
	}

	if fixtureA.IsSensor() || fixtureB.IsSensor() {
		fmt.Println("Stuff collides with sensors.")
		return true
	}

	fmt.Println("Custom Contact Listener encountered an unexpected situation.")
	return false
}

func (m *CustomContactListener) BeginContact(contact box2d.B2ContactInterface) {
	fmt.Println("BeginContact")
	pair := EntityContactPair{}

	fixtureA := contact.GetFixtureA()
	if fixtureA == nil {
		fmt.Println("BeginContact: FixtureA doesn't exist.")
		return
	}
	userDataA := fixtureA.GetUserData()
	if userDataA == nil {
		fmt.Println("BeginContact: FixtureA.UserData doesn't exist.")
		return
	}
	entityID0, ok := userDataA.(types.EntityID)
	if !ok {
		fmt.Println("BeginContact: userDataA was not type EntityID")
		return
	}
	pair.EntityID0 = entityID0

	fixtureB := contact.GetFixtureB()
	if fixtureB == nil {
		fmt.Println("BeginContact: FixtureB doesn't exist.")
		return
	}
	userDataB := fixtureB.GetUserData()
	if userDataB == nil {
		fmt.Println("BeginContact: FixtureB.UserData doesn't exist.")
		return
	}
	entityID1, ok := userDataB.(types.EntityID)
	if !ok {
		fmt.Println("BeginContact: userDataB was not type EntityID")
		return
	}
	pair.EntityID1 = entityID1

	select {
	case m.Physics2DInstance.Contacts <- pair:
		fmt.Println("Sent entity pair to Contacts channel.")
	default:
		fmt.Println("Contacts Channel: ", len(m.Physics2DInstance.Contacts), "of", cap(m.Physics2DInstance.Contacts))
		if len(m.Physics2DInstance.Contacts) == 0 {
			panic("We can't send to the Contacts channel even though it's empty.")
		} else if len(m.Physics2DInstance.Contacts) < 4096 {
			panic("We can't send to the Contacts channel.")
		} else {
			panic("WARNING: We can't send to the Contacts channel because it's full.")
		}
	}
}
func (m *CustomContactListener) EndContact(_ box2d.B2ContactInterface) {}
func (m *CustomContactListener) PreSolve(_ box2d.B2ContactInterface, _ box2d.B2Manifold) {
	// handle pre-solve
}
func (m *CustomContactListener) PostSolve(_ box2d.B2ContactInterface, _ *box2d.B2ContactImpulse) {
	// handle post-solve
}

// It's a Singleton for now but at least we still have options.
var instance *Physics2D

func Instance() *Physics2D {
	if instance == nil {
		initialize()
	}
	return instance
}

func initialize() {
	// TODO: Set stepsPerSecond based on the server's tick rate.
	const stepsPerSecond = 20
	const desiredFrameRate = 60
	const desiredVelocityIterations = 4
	const desiredPositionIterations = 2

	gravity := box2d.MakeB2Vec2(0, 0)
	world2D := box2d.MakeB2World(gravity)

	instance = newPhysics2D(
		stepsPerSecond,
		desiredFrameRate,
		desiredVelocityIterations,
		desiredPositionIterations,
		&world2D)

	contactsListener := &CustomContactListener{Physics2DInstance: instance}
	world2D.SetContactFilter(box2d.B2ContactFilterInterface(contactsListener))
	world2D.SetContactListener(box2d.B2ContactListenerInterface(contactsListener))
}

func newPhysics2D(
	stepsPerSecond float64,
	desiredFrameRate float64,
	desiredVelocityIterations int,
	desiredPositionIterations int,
	world2D *box2d.B2World,
) *Physics2D {
	// 60 FPS is recommended. I typically use 30 + interpolation as needed.
	// We're attempting to make up for lost precision by increasing the iterations.
	iterScale := desiredFrameRate / stepsPerSecond

	var physics2D = &Physics2D{
		fixedDeltaTime:        1.0 / stepsPerSecond,
		numVelocityIterations: int(float64(desiredVelocityIterations) * iterScale),
		numPositionIterations: int(float64(desiredPositionIterations) * iterScale),
		world2D:               world2D,
		rigidBodies:           map[types.EntityID]*box2d.B2Body{},
	}

	physics2D.Contacts = make(chan EntityContactPair, 4096)

	return physics2D
}

func (physics2D *Physics2D) Update() {
	physics2D.world2D.Step(
		physics2D.fixedDeltaTime,
		physics2D.numVelocityIterations,
		physics2D.numPositionIterations)
}

func (physics2D *Physics2D) GetFixedDeltaTime() float64 {
	return physics2D.fixedDeltaTime
}

func (physics2D *Physics2D) GetWorld2D() *box2d.B2World {
	return physics2D.world2D
}

func (physics2D *Physics2D) TryGetBody(entityID types.EntityID) (*box2d.B2Body, bool) {
	body, ok := physics2D.rigidBodies[entityID]
	if !ok {
		//panic(fmt.Sprintf("Physics2D: Body not found for entityID %d", entityID))
		fmt.Printf("Physics2D.TryGetBody: Body not found for entityID %d", entityID)
		return nil, false
	}
	if body == nil {
		panic(fmt.Sprintf("Physics2D.TryGetBody: Body is nil for entityID %d", entityID))
	}
	return body, true
}

func (physics2D *Physics2D) DestroyBody(entityID types.EntityID) error {
	var body, ok = physics2D.rigidBodies[entityID]
	if !ok {
		return fmt.Errorf("Physics2D.DestroyBody: Failed to get body")
	}
	if body == nil {
		return errors.New("Physics2D.DestroyBody: Body is nil")
	}
	physics2D.world2D.DestroyBody(body)
	delete(physics2D.rigidBodies, entityID)
	return nil
}

// Note: UserData is reserved and set to the entityID.
func (physics2D *Physics2D) LinkEntityToBody(entityID types.EntityID, body *box2d.B2Body) {
	body.SetUserData(entityID)
	physics2D.rigidBodies[entityID] = body
}

func (physics2D *Physics2D) CreatePlayerBody(entityID types.EntityID, worldPosition box2d.B2Vec2, radius float64) *box2d.B2Body {
	bd := box2d.MakeB2BodyDef()
	bd.Type = box2d.B2BodyType.B2_dynamicBody
	bd.FixedRotation = true
	bd.Position = worldPosition
	bd.AllowSleep = true
	bd.Active = true
	// There could be a performance hit here when creating a large number of entities all at once.
	bd.Awake = true
	bd.LinearDamping = 0
	bd.UserData = entityID
	body := physics2D.world2D.CreateBody(&bd)

	shape := box2d.MakeB2CircleShape()
	shape.M_radius = radius

	fd := box2d.MakeB2FixtureDef()
	fd.Shape = &shape
	fd.Density = 1.0
	fd.Filter.GroupIndex = playerGroupIndex
	fd.UserData = entityID
	body.CreateFixtureFromDef(&fd)
	body.GetFixtureList().SetFilterData(fd.Filter)

	physics2D.LinkEntityToBody(entityID, body)
	return body
}

func (physics2D *Physics2D) CreatePickupBody(entityID types.EntityID, worldPosition box2d.B2Vec2, radius float64) *box2d.B2Body {
	bd := box2d.MakeB2BodyDef()
	bd.Type = box2d.B2BodyType.B2_staticBody
	bd.Position = worldPosition
	bd.FixedRotation = true
	bd.AllowSleep = true
	bd.Active = true
	bd.Awake = true
	bd.UserData = entityID

	body := physics2D.GetWorld2D().CreateBody(&bd)

	shape := box2d.MakeB2CircleShape()
	shape.M_radius = radius

	fd := box2d.MakeB2FixtureDef()
	fd.Shape = &shape
	fd.IsSensor = true
	fd.Filter.GroupIndex = pickupGroupIndex
	fd.UserData = entityID
	body.CreateFixtureFromDef(&fd)
	body.GetFixtureList().SetFilterData(fd.Filter)

	physics2D.LinkEntityToBody(entityID, body)

	return body
}

func (physics2D *Physics2D) FindClosestNeighbor(
	position box2d.B2Vec2,
	maxRadius float64,
	callback func(entityID types.EntityID) bool,
) (types.EntityID, bool) {
	aabb := box2d.MakeB2AABB()
	aabb.LowerBound = box2d.MakeB2Vec2(position.X-maxRadius, position.Y-maxRadius)
	aabb.UpperBound = box2d.MakeB2Vec2(position.X+maxRadius, position.Y+maxRadius)
	radiusSquared := maxRadius * maxRadius

	wasEntityFound := false
	var closestEntityID types.EntityID
	closestSqrDist := math.MaxFloat64

	physics2D.world2D.QueryAABB(func(fixture *box2d.B2Fixture) bool {
		body := fixture.GetBody()
		sqrDist := box2d.B2Vec2DistanceSquared(body.GetPosition(), position)
		if sqrDist > radiusSquared {
			return true
		}
		if sqrDist < closestSqrDist {
			targetEntityID, ok := body.GetUserData().(types.EntityID)
			if !ok {
				fmt.Println("FindClosestNeighbor: targetEntityID could not convert to type EntityID")
				return true
			}
			if !callback(targetEntityID) {
				return true
			}
			closestSqrDist = sqrDist
			closestEntityID = targetEntityID
			wasEntityFound = true
		}
		return true
	}, aabb)
	return closestEntityID, wasEntityFound
}

func (physics2D *Physics2D) FindNearestNeighbors(
	position box2d.B2Vec2,
	maxRadius float64,
	maxResults int,
	results []types.EntityID,
) {

	aabb := box2d.MakeB2AABB()
	aabb.LowerBound = box2d.MakeB2Vec2(-maxRadius, -maxRadius)
	aabb.UpperBound = box2d.MakeB2Vec2(maxRadius, maxRadius)
	radiusSquared := maxRadius * maxRadius

	physics2D.world2D.QueryAABB(func(fixture *box2d.B2Fixture) bool {
		body := fixture.GetBody()
		sqrDist := box2d.B2Vec2DistanceSquared(body.GetPosition(), position)
		if sqrDist > radiusSquared {
			return true
		}
		entityID, ok := body.GetUserData().(types.EntityID)
		if !ok {
			fmt.Println("FindNearestNeighbors: user data was not type EntityID")
			return true
		}
		results = append(results, entityID)
		return len(results) < maxResults
	}, aabb)
}
