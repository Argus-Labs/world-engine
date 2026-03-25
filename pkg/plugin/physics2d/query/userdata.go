package query

import "github.com/argus-labs/world-engine/pkg/cardinal"

// BodyUserData is stored on each B2Body (B2BodyDef.UserData) so callbacks and queries can
// recover the Cardinal entity without scanning ECS.
type BodyUserData struct {
	EntityID cardinal.EntityID
}

// FixtureUserData is stored on each B2Fixture (B2FixtureDef.UserData). ShapeIndex is the
// index into Collider2D.Shapes for that entity (v1 identity: fixture order creation does not
// match Box2D’s linked list order; always use this index, not fixture iteration order).
type FixtureUserData struct {
	EntityID   cardinal.EntityID
	ShapeIndex int
}

// FixtureUserDataFrom returns entity and shape index if UserData is *FixtureUserData.
func FixtureUserDataFrom(data any) (cardinal.EntityID, int, bool) {
	if data == nil {
		return 0, 0, false
	}
	u, ok := data.(*FixtureUserData)
	if !ok {
		return 0, 0, false
	}
	return u.EntityID, u.ShapeIndex, true
}

// BodyUserDataFrom returns the entity ID if UserData is *BodyUserData.
func BodyUserDataFrom(data any) (cardinal.EntityID, bool) {
	if data == nil {
		return 0, false
	}
	u, ok := data.(*BodyUserData)
	if !ok {
		return 0, false
	}
	return u.EntityID, true
}
