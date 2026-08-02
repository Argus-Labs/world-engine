// Tests for the float64 port of b2World_Explode
// (src/physics_world.c, pkg/box2d/world_explode.go).

package box2d_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/box2d"
)

const (
	explodeCategoryA uint64 = 0x0001
	explodeCategoryB uint64 = 0x0002

	// The explosion scene uses unit-density circles of this radius, so the
	// hand calculations below only need pi.
	explodeCircleRadius = 0.5
)

// explodeCircleMass is the mass of a unit-density circle of
// explodeCircleRadius. Used by the hand calculations.
func explodeCircleMass() float64 {
	return math.Pi * explodeCircleRadius * explodeCircleRadius
}

// newExplodeWorld returns a gravity-free world so an explosion impulse is the
// only thing that ever changes a velocity.
func newExplodeWorld(t *testing.T) *box2d.World {
	t.Helper()

	worldDef := box2d.DefaultWorldDef()
	worldDef.Gravity = box2d.Vec2{X: 0.0, Y: 0.0}
	world := box2d.NewWorld(&worldDef)
	t.Cleanup(world.Destroy)

	return world
}

// addExplodeCircle adds a body carrying one unit-density circle at position
// with the supplied body type and collision category.
func addExplodeCircle(t *testing.T, world *box2d.World, bodyType box2d.BodyType, position box2d.Vec2,
	category uint64,
) box2d.BodyID {
	t.Helper()

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = bodyType
	bodyDef.Position = position
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	shapeDef.Filter.CategoryBits = category
	circle := box2d.Circle{Center: box2d.Vec2{X: 0.0, Y: 0.0}, Radius: explodeCircleRadius}
	world.CreateCircleShape(bodyID, &shapeDef, &circle)

	return bodyID
}

func TestExplodeDefaultDef(t *testing.T) {
	t.Parallel()

	def := box2d.DefaultExplosionDef()

	assert.Equal(t, box2d.DefaultMaskBits, def.MaskBits)
	assert.Equal(t, box2d.Vec2{X: 0.0, Y: 0.0}, def.Position)
	assert.InDelta(t, 0.0, def.Radius, 0.0)
	assert.InDelta(t, 0.0, def.Falloff, 0.0)
	assert.InDelta(t, 0.0, def.ImpulsePerLength, 0.0)
}

// TestExplodeInsideRadius checks the impulse of a body well inside the
// explosion radius against a hand calculation.
//
// The body is a unit-density circle of radius 0.5 centred at (2, 0); the
// explosion sits at the origin. The closest point of the circle to the origin
// is (1.5, 0), so the distance is 1.5 and the outward direction is +x. The
// projected perimeter of a circle is 2*radius = 1 regardless of direction, and
// the falloff scale is 1 because the distance is inside the radius. The
// impulse is therefore impulsePerLength * 1 * 1 = 10 along +x, and the
// resulting speed is 10 / (pi * 0.25).
//
// The impulse line passes through the centre of mass, so angular velocity must
// stay exactly zero.
func TestExplodeInsideRadius(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)
	bodyID := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.DefaultCategoryBits)

	def := box2d.DefaultExplosionDef()
	def.Position = box2d.Vec2{X: 0.0, Y: 0.0}
	def.Radius = 5.0
	def.Falloff = 0.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	velocity := world.BodyLinearVelocity(bodyID)
	expectedSpeed := def.ImpulsePerLength * (2.0 * explodeCircleRadius) / explodeCircleMass()

	assert.InDelta(t, expectedSpeed, velocity.X, 1e-12)
	assert.InDelta(t, 0.0, velocity.Y, 1e-12)
	assert.InDelta(t, 0.0, world.BodyAngularVelocity(bodyID), 1e-12)
	assert.Greater(t, velocity.X, 0.0, "impulse must point away from the explosion")
}

// TestExplodeOutwardDirection checks that four bodies arranged around the
// explosion each move directly away from it with the same speed.
func TestExplodeOutwardDirection(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)

	offsets := []box2d.Vec2{
		{X: 2.0, Y: 0.0},
		{X: -2.0, Y: 0.0},
		{X: 0.0, Y: 2.0},
		{X: 0.0, Y: -2.0},
	}
	bodyIDs := make([]box2d.BodyID, 0, len(offsets))
	for _, offset := range offsets {
		bodyIDs = append(bodyIDs, addExplodeCircle(t, world, box2d.DynamicBody, offset, box2d.DefaultCategoryBits))
	}

	def := box2d.DefaultExplosionDef()
	def.Radius = 5.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	expectedSpeed := def.ImpulsePerLength * (2.0 * explodeCircleRadius) / explodeCircleMass()

	for i, bodyID := range bodyIDs {
		velocity := world.BodyLinearVelocity(bodyID)
		outward := box2d.Normalize(offsets[i])

		assert.InDelta(t, expectedSpeed*outward.X, velocity.X, 1e-12)
		assert.InDelta(t, expectedSpeed*outward.Y, velocity.Y, 1e-12)
	}
}

// TestExplodeImplosion checks that a negative impulse per length pulls bodies
// toward the explosion centre.
func TestExplodeImplosion(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)
	bodyID := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.DefaultCategoryBits)

	def := box2d.DefaultExplosionDef()
	def.Radius = 5.0
	def.ImpulsePerLength = -10.0
	world.Explode(&def)

	velocity := world.BodyLinearVelocity(bodyID)

	assert.Less(t, velocity.X, 0.0, "a negative impulse per length must implode")
}

// TestExplodeOutsideRadiusNoFalloff checks that a body beyond the radius is
// untouched when the falloff is zero.
func TestExplodeOutsideRadiusNoFalloff(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)
	inside := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.DefaultCategoryBits)
	outside := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 20.0, Y: 0.0}, box2d.DefaultCategoryBits)

	def := box2d.DefaultExplosionDef()
	def.Radius = 5.0
	def.Falloff = 0.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	assert.Greater(t, world.BodyLinearVelocity(inside).X, 0.0)

	outsideVelocity := world.BodyLinearVelocity(outside)
	assert.InDelta(t, 0.0, outsideVelocity.X, 0.0)
	assert.InDelta(t, 0.0, outsideVelocity.Y, 0.0)
	assert.InDelta(t, 0.0, world.BodyAngularVelocity(outside), 0.0)
}

// TestExplodeFalloffBand checks the linear falloff ramp. The body centre sits
// at x = 6 with radius 0.5, so its closest point is at distance 5.5, which is
// 0.5 into a falloff band of 2 starting at radius 5. The expected scale is
// (5 + 2 - 5.5) / 2 = 0.75. A second body beyond radius + falloff is untouched.
func TestExplodeFalloffBand(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)
	inBand := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 6.0, Y: 0.0}, box2d.DefaultCategoryBits)
	beyond := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 9.0, Y: 0.0}, box2d.DefaultCategoryBits)

	def := box2d.DefaultExplosionDef()
	def.Radius = 5.0
	def.Falloff = 2.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	const expectedScale = 0.75
	fullSpeed := def.ImpulsePerLength * (2.0 * explodeCircleRadius) / explodeCircleMass()

	assert.InDelta(t, expectedScale*fullSpeed, world.BodyLinearVelocity(inBand).X, 1e-12)
	assert.InDelta(t, 0.0, world.BodyLinearVelocity(beyond).X, 0.0)
}

// TestExplodeMaskBits checks that the explosion mask bits filter shapes by
// their category bits.
func TestExplodeMaskBits(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)
	selected := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 2.0, Y: 0.0}, explodeCategoryA)
	ignored := addExplodeCircle(t, world, box2d.DynamicBody, box2d.Vec2{X: 0.0, Y: 2.0}, explodeCategoryB)

	def := box2d.DefaultExplosionDef()
	def.MaskBits = explodeCategoryA
	def.Radius = 5.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	assert.Greater(t, world.BodyLinearVelocity(selected).X, 0.0)

	ignoredVelocity := world.BodyLinearVelocity(ignored)
	assert.InDelta(t, 0.0, ignoredVelocity.X, 0.0)
	assert.InDelta(t, 0.0, ignoredVelocity.Y, 0.0)
}

// TestExplodeIgnoresNonDynamicBodies checks that static and kinematic bodies
// are never touched: the explosion only queries the dynamic broad-phase tree.
func TestExplodeIgnoresNonDynamicBodies(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)
	staticID := addExplodeCircle(t, world, box2d.StaticBody, box2d.Vec2{X: 2.0, Y: 0.0}, box2d.DefaultCategoryBits)
	kinematicID := addExplodeCircle(t, world, box2d.KinematicBody, box2d.Vec2{X: 0.0, Y: 2.0}, box2d.DefaultCategoryBits)

	def := box2d.DefaultExplosionDef()
	def.Radius = 5.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	staticVelocity := world.BodyLinearVelocity(staticID)
	assert.InDelta(t, 0.0, staticVelocity.X, 0.0)
	assert.InDelta(t, 0.0, staticVelocity.Y, 0.0)

	kinematicVelocity := world.BodyLinearVelocity(kinematicID)
	assert.InDelta(t, 0.0, kinematicVelocity.X, 0.0)
	assert.InDelta(t, 0.0, kinematicVelocity.Y, 0.0)
}

// TestExplodeAppliesTorqueOffCenter checks that an impulse whose line of action
// misses the centre of mass produces angular velocity. The box is twice as tall
// as it is wide and sits diagonally from the explosion, so the closest point is
// a corner and the impulse arm is non-zero.
func TestExplodeAppliesTorqueOffCenter(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyDef.Position = box2d.Vec2{X: 3.0, Y: 3.0}
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	polygon := box2d.MakeBox(0.5, 1.0)
	world.CreatePolygonShape(bodyID, &shapeDef, &polygon)

	def := box2d.DefaultExplosionDef()
	def.Radius = 10.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	velocity := world.BodyLinearVelocity(bodyID)
	require.Greater(t, velocity.X, 0.0)
	require.Greater(t, velocity.Y, 0.0)
	assert.Greater(t, math.Abs(world.BodyAngularVelocity(bodyID)), 1e-9)
}

// TestExplodeWakesSleepingBody checks that a sleeping body is woken and then
// receives the impulse in the same call.
func TestExplodeWakesSleepingBody(t *testing.T) {
	t.Parallel()

	world := newExplodeWorld(t)

	bodyDef := box2d.DefaultBodyDef()
	bodyDef.Type = box2d.DynamicBody
	bodyDef.Position = box2d.Vec2{X: 2.0, Y: 0.0}
	bodyDef.IsAwake = false
	bodyID := world.CreateBody(&bodyDef)

	shapeDef := box2d.DefaultShapeDef()
	circle := box2d.Circle{Center: box2d.Vec2{X: 0.0, Y: 0.0}, Radius: explodeCircleRadius}
	world.CreateCircleShape(bodyID, &shapeDef, &circle)

	require.False(t, world.IsBodyAwake(bodyID))

	def := box2d.DefaultExplosionDef()
	def.Radius = 5.0
	def.ImpulsePerLength = 10.0
	world.Explode(&def)

	assert.True(t, world.IsBodyAwake(bodyID))
	assert.Greater(t, world.BodyLinearVelocity(bodyID).X, 0.0)
}
