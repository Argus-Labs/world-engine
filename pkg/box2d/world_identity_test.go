// Cross-world id ownership tests.
//
// This port has no b2_worlds registry, so an id cannot be resolved back to the
// world that minted it (see the DESIGN DEVIATION header in world.go). Instead
// every World takes a distinct owner token at creation and stamps it into the
// world0 field of every id it hands out. These tests pin that behavior: two
// worlds built from the same script allocate the same slots and generations,
// so the owner token is the only thing that keeps world B from accepting —
// and destroying — world A's objects.

package box2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// worldTokenMask covers the world0 field inside a packed id (bits 16..31 of
// the Pack*ID layout: index1<<32 | world0<<16 | generation).
const worldTokenMask = uint64(0xFFFF) << 16

// identityScene is one world plus one id of every kind it can hand out.
type identityScene struct {
	world *box2d.World
	body  box2d.BodyID
	shape box2d.ShapeID
	chain box2d.ChainID
	joint box2d.JointID
}

// buildIdentityScene builds a fixed scene. Two calls produce two worlds whose
// bodies, shapes, chains and joints occupy identical slots with identical
// generations, so any id from one is index/generation-compatible with the
// other.
func buildIdentityScene(t *testing.T) identityScene {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	groundDef := box2d.DefaultBodyDef()
	ground := world.CreateBody(&groundDef)

	chainDef := box2d.DefaultChainDef()
	chainDef.Points = []box2d.Vec2{
		{X: -4.0, Y: 0.0},
		{X: -2.0, Y: 0.0},
		{X: 2.0, Y: 0.0},
		{X: 4.0, Y: 0.0},
	}
	chain := world.CreateChain(ground, &chainDef)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyDef.Position = box2d.Vec2{X: 1.0, Y: 4.0}
	body := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.5, 0.5)
	shape := world.CreatePolygonShape(body, &shapeDef, &box)

	jointDef := box2d.DefaultDistanceJointDef()
	jointDef.Base.BodyIDA = ground
	jointDef.Base.BodyIDB = body
	joint := world.CreateDistanceJoint(&jointDef)

	return identityScene{world: world, body: body, shape: shape, chain: chain, joint: joint}
}

func TestIDsAreBoundToTheirOwningWorld(t *testing.T) {
	t.Parallel()

	a := buildIdentityScene(t)
	b := buildIdentityScene(t)

	// Each world accepts the ids it minted.
	require.True(t, a.world.IsBodyValid(a.body))
	require.True(t, a.world.IsShapeValid(a.shape))
	require.True(t, a.world.IsChainValid(a.chain))
	require.True(t, a.world.IsJointValid(a.joint))
	require.True(t, b.world.IsBodyValid(b.body))
	require.True(t, b.world.IsShapeValid(b.shape))
	require.True(t, b.world.IsChainValid(b.chain))
	require.True(t, b.world.IsJointValid(b.joint))

	// The two scenes really are slot-for-slot identical: the packed ids agree
	// once the owner token is masked out. Without the token, every assertion
	// below would pass in the wrong direction.
	tassert.Equal(t,
		box2d.PackBodyID(a.body)&^worldTokenMask,
		box2d.PackBodyID(b.body)&^worldTokenMask)
	tassert.Equal(t,
		box2d.PackShapeID(a.shape)&^worldTokenMask,
		box2d.PackShapeID(b.shape)&^worldTokenMask)
	tassert.Equal(t,
		box2d.PackChainID(a.chain)&^worldTokenMask,
		box2d.PackChainID(b.chain)&^worldTokenMask)
	tassert.Equal(t,
		box2d.PackJointID(a.joint)&^worldTokenMask,
		box2d.PackJointID(b.joint)&^worldTokenMask)

	// ...and the tokens themselves differ.
	tassert.NotEqual(t, box2d.PackBodyID(a.body), box2d.PackBodyID(b.body))

	// Neither world accepts the other's ids.
	tassert.False(t, b.world.IsBodyValid(a.body))
	tassert.False(t, b.world.IsShapeValid(a.shape))
	tassert.False(t, b.world.IsChainValid(a.chain))
	tassert.False(t, b.world.IsJointValid(a.joint))
	tassert.False(t, a.world.IsBodyValid(b.body))
	tassert.False(t, a.world.IsShapeValid(b.shape))
	tassert.False(t, a.world.IsChainValid(b.chain))
	tassert.False(t, a.world.IsJointValid(b.joint))
}

func TestContactIDsAreBoundToTheirOwningWorld(t *testing.T) {
	t.Parallel()

	// buildContactScene drops a box onto a static box and steps until the two
	// touch, so the world holds at least one contact.
	buildContactScene := func() (*box2d.World, box2d.BodyID) {
		worldDef := box2d.DefaultWorldDef()
		world := box2d.NewWorld(&worldDef)
		t.Cleanup(world.Destroy)

		groundDef := box2d.DefaultBodyDef()
		ground := world.CreateBody(&groundDef)
		shapeDef := box2d.DefaultShapeDef()
		groundBox := box2d.MakeBox(10.0, 1.0)
		world.CreatePolygonShape(ground, &shapeDef, &groundBox)

		bodyDef := box2d.DefaultBodyDef()
		bodyDef.Type = box2d.DynamicBody
		bodyDef.Position = box2d.Vec2{X: 0.0, Y: 1.4}
		body := world.CreateBody(&bodyDef)
		box := box2d.MakeBox(0.5, 0.5)
		world.CreatePolygonShape(body, &shapeDef, &box)

		for range 20 {
			world.Step(1.0/60.0, 4)
		}

		return world, body
	}

	contactID := func(world *box2d.World, body box2d.BodyID) box2d.ContactID {
		data := make([]box2d.ContactData, world.BodyContactCapacity(body))
		count := world.BodyContactData(body, data)
		require.Positive(t, count, "scene produced no contacts")
		return data[0].ContactID
	}

	worldA, bodyA := buildContactScene()
	worldB, bodyB := buildContactScene()

	idA := contactID(worldA, bodyA)
	idB := contactID(worldB, bodyB)

	require.True(t, worldA.IsContactValid(idA))
	require.True(t, worldB.IsContactValid(idB))

	// Same slot and generation in both worlds, distinguished only by the token.
	tassert.Equal(t, box2d.PackContactID(idA)[0], box2d.PackContactID(idB)[0])
	tassert.Equal(t, box2d.PackContactID(idA)[2], box2d.PackContactID(idB)[2])
	tassert.NotEqual(t, box2d.PackContactID(idA)[1], box2d.PackContactID(idB)[1])

	tassert.False(t, worldB.IsContactValid(idA))
	tassert.False(t, worldA.IsContactValid(idB))
}

func TestGenerationCheckSurvivesDestroyAndRecreate(t *testing.T) {
	t.Parallel()

	a := buildIdentityScene(t)
	b := buildIdentityScene(t)

	stale := a.body
	a.world.DestroyBody(stale)
	require.False(t, a.world.IsBodyValid(stale))

	// The recycled slot comes back with a bumped generation, so the stale id
	// stays rejected while the fresh one validates.
	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyDef.Position = box2d.Vec2{X: 1.0, Y: 4.0}
	fresh := a.world.CreateBody(&bodyDef)

	require.True(t, a.world.IsBodyValid(fresh))
	tassert.False(t, a.world.IsBodyValid(stale))
	tassert.Equal(t,
		box2d.PackBodyID(stale)>>32,
		box2d.PackBodyID(fresh)>>32,
		"expected the same slot index to be recycled")

	// The owner token is orthogonal to the generation check: the fresh id is
	// still foreign to world b, and b's own body is still foreign to a.
	tassert.False(t, b.world.IsBodyValid(fresh))
	tassert.False(t, b.world.IsBodyValid(stale))
	tassert.False(t, a.world.IsBodyValid(b.body))
}

func TestDestroyedWorldRejectsItsOwnIDs(t *testing.T) {
	t.Parallel()

	// Built without buildIdentityScene's t.Cleanup because this test destroys
	// the world itself and Destroy is not idempotent.
	worldDef := box2d.DefaultWorldDef()
	world := box2d.NewWorld(&worldDef)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	body := world.CreateBody(&bodyDef)
	shapeDef := box2d.DefaultShapeDef()
	box := box2d.MakeBox(0.5, 0.5)
	shape := world.CreatePolygonShape(body, &shapeDef, &box)

	require.True(t, world.IsBodyValid(body))
	world.Destroy()

	// Destroy keeps the owner token (so the ids still match on world0) and
	// bumps the world generation; inUse going false is what rejects them.
	tassert.False(t, world.IsBodyValid(body))
	tassert.False(t, world.IsShapeValid(shape))
}
