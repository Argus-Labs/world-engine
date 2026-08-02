// Tests for the float64 port of Box2D v3.2.0 src/physics_world.c world
// lifecycle, id validation and solver set bookkeeping. Internal package tests
// because they inspect solver set membership directly.

package box2d

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorldDefaults(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	require.NotNil(t, w)

	// The three permanent sets exist with their fixed indices.
	require.Len(t, w.solverSets, 3)
	tassert.Equal(t, staticSet, w.solverSets[staticSet].setIndex)
	tassert.Equal(t, disabledSet, w.solverSets[disabledSet].setIndex)
	tassert.Equal(t, awakeSet, w.solverSets[awakeSet].setIndex)

	tassert.Equal(t, Vec2{X: 0.0, Y: -10.0}, w.Gravity())
	tassert.True(t, w.IsSleepingEnabled())
	tassert.True(t, w.IsContinuousEnabled())
	tassert.True(t, w.IsWarmStartingEnabled())
	tassert.Equal(t, 0, w.AwakeBodyCount())

	counters := w.Counters()
	tassert.Equal(t, 0, counters.BodyCount)
	tassert.Equal(t, 0, counters.ShapeCount)
	tassert.Equal(t, 0, counters.ContactCount)
	tassert.Equal(t, 0, counters.JointCount)
	tassert.Equal(t, 0, counters.IslandCount)

	id := w.ID()
	tassert.True(t, id.IsNonNull())
	tassert.True(t, w.IsWorldValid(id))

	w.Destroy()
}

func TestWorldDestroyInvalidatesID(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	id := w.ID()
	require.True(t, w.IsWorldValid(id))

	w.Destroy()

	// Destroy preserves the generation counter and bumps it, so the stale id
	// no longer validates.
	tassert.False(t, w.IsWorldValid(id))
	tassert.False(t, w.inUse)
}

func TestWorldSettersAndGetters(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	w.SetGravity(Vec2{X: 1.0, Y: 2.0})
	tassert.Equal(t, Vec2{X: 1.0, Y: 2.0}, w.Gravity())

	w.SetRestitutionThreshold(2.5)
	tassert.InDelta(t, 2.5, w.RestitutionThreshold(), 0.0)

	w.SetHitEventThreshold(3.5)
	tassert.InDelta(t, 3.5, w.HitEventThreshold(), 0.0)

	w.SetContactRecycleDistance(0.25)
	tassert.InDelta(t, 0.25, w.ContactRecycleDistance(), 0.0)

	w.SetMaximumLinearSpeed(123.0)
	tassert.InDelta(t, 123.0, w.MaximumLinearSpeed(), 0.0)

	w.EnableContinuous(false)
	tassert.False(t, w.IsContinuousEnabled())

	w.EnableWarmStarting(false)
	tassert.False(t, w.IsWarmStartingEnabled())

	w.SetUserData(77)
	tassert.Equal(t, uint64(77), w.UserData())

	// Negative values clamp to zero.
	w.SetRestitutionThreshold(-1.0)
	tassert.InDelta(t, 0.0, w.RestitutionThreshold(), 0.0)
}

func TestWorldIDValidationRejectsGarbage(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	tassert.False(t, w.IsWorldValid(WorldID{}))
	tassert.False(t, w.IsBodyValid(BodyID{}))
	tassert.False(t, w.IsShapeValid(ShapeID{}))
	tassert.False(t, w.IsChainValid(ChainID{}))
	tassert.False(t, w.IsJointValid(JointID{}))
	tassert.False(t, w.IsContactValid(ContactID{}))

	// Out of range indices. These carry this world's own owner token so the
	// index check is what rejects them; a hard-coded world0 would now be
	// rejected by the token check first and stop exercising the index bound.
	tassert.False(t, w.IsBodyValid(BodyID{index1: 100, world0: w.worldID, generation: 0}))
	tassert.False(t, w.IsShapeValid(ShapeID{index1: 100, world0: w.worldID, generation: 0}))

	// Wrong world token. Derived from this world's token rather than a literal
	// because tokens are now handed out by a process-wide counter, so any
	// literal could legitimately belong to this world.
	tassert.False(t, w.IsBodyValid(BodyID{index1: 1, world0: w.worldID + 1, generation: 1}))
}

// sleepWakeCycle drives one body through a full sleep/wake transfer.
func sleepWakeCycle(t *testing.T, w *World, id BodyID) {
	t.Helper()

	require.True(t, w.IsBodyAwake(id))

	w.SetBodyAwake(id, false)
	require.False(t, w.IsBodyAwake(id))

	b := w.getBodyFullID(id)
	require.GreaterOrEqual(t, b.setIndex, firstSleepingSet)

	// The sleeping set owns the body sim and one island sim.
	sleepSet := &w.solverSets[b.setIndex]
	require.Len(t, sleepSet.bodySims, 1)
	require.Empty(t, sleepSet.bodyStates)
	require.Len(t, sleepSet.islandSims, 1)
	tassert.Equal(t, b.id, sleepSet.bodySims[b.localIndex].bodyID)
	tassert.Equal(t, b.islandID, sleepSet.islandSims[0].islandID)

	w.SetBodyAwake(id, true)
	require.True(t, w.IsBodyAwake(id))

	b = w.getBodyFullID(id)
	tassert.Equal(t, awakeSet, b.setIndex)
	awake := &w.solverSets[awakeSet]
	tassert.Equal(t, b.id, awake.bodySims[b.localIndex].bodyID)
	tassert.Len(t, awake.bodyStates, len(awake.bodySims))

	// Sleep timer was reset by the wake transfer.
	tassert.InDelta(t, 0.0, b.sleepTime, 0.0)
}

func TestWakeSleepTransfer(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.Position = Vec2{X: 1.0, Y: 2.0}
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	w.CreateCircleShape(id, &shapeDef, &circle)

	setCountBefore := getIDCount(&w.solverSetIDPool)
	sleepWakeCycle(t, w, id)

	// The temporary sleeping set was recycled by the wake transfer.
	tassert.Equal(t, setCountBefore, getIDCount(&w.solverSetIDPool))
}

func TestCreateSleepingBodyOwnSet(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.IsAwake = false
	id := w.CreateBody(&bodyDef)

	require.False(t, w.IsBodyAwake(id))
	b := w.getBodyFullID(id)
	tassert.GreaterOrEqual(t, b.setIndex, firstSleepingSet)
	tassert.Equal(t, b.setIndex, w.solverSets[b.setIndex].setIndex)

	// Waking moves it to the awake set and destroys the private set.
	w.SetBodyAwake(id, true)
	b = w.getBodyFullID(id)
	tassert.Equal(t, awakeSet, b.setIndex)
}

func TestDisableEnableBody(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)

	shapeDef := DefaultShapeDef()
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	shapeID := w.CreateCircleShape(id, &shapeDef, &circle)

	require.True(t, w.IsBodyEnabled(id))
	require.Equal(t, 1, w.broadPhase.trees[DynamicBody].GetProxyCount())

	w.DisableBody(id)
	tassert.False(t, w.IsBodyEnabled(id))
	tassert.Equal(t, 0, w.broadPhase.trees[DynamicBody].GetProxyCount())
	b := w.getBodyFullID(id)
	tassert.Equal(t, disabledSet, b.setIndex)
	tassert.Equal(t, NullIndex, b.islandID)
	tassert.Equal(t, NullIndex, w.shapes[int(shapeID.index1)-1].proxyKey)

	w.EnableBody(id)
	tassert.True(t, w.IsBodyEnabled(id))
	tassert.Equal(t, 1, w.broadPhase.trees[DynamicBody].GetProxyCount())
	b = w.getBodyFullID(id)
	tassert.Equal(t, awakeSet, b.setIndex)
	tassert.NotEqual(t, NullIndex, b.islandID)
}

func TestEnableSleepingFalseWakesAll(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	id := w.CreateBody(&bodyDef)
	w.SetBodyAwake(id, false)
	require.False(t, w.IsBodyAwake(id))

	w.EnableSleeping(false)
	tassert.False(t, w.IsSleepingEnabled())
	tassert.True(t, w.IsBodyAwake(id))
}

// buildDeterministicScene runs an identical operation sequence against a
// world: mixed body/shape creation, destruction, sleep and wake transfers.
func buildDeterministicScene(w *World) []BodyID {
	bodyDef := DefaultBodyDef()
	shapeDef := DefaultShapeDef()

	circle := Circle{Center: Vec2{X: 0.1, Y: 0.2}, Radius: 0.5}
	box := MakeBox(0.5, 0.75)
	capsule := Capsule{Center1: Vec2{X: -0.5, Y: 0.0}, Center2: Vec2{X: 0.5, Y: 0.0}, Radius: 0.25}
	segment := Segment{Point1: Vec2{X: -1.0, Y: 0.0}, Point2: Vec2{X: 1.0, Y: 0.0}}

	ids := make([]BodyID, 0, 60)
	for i := range 60 {
		switch i % 3 {
		case 0:
			bodyDef.Type = StaticBody
		case 1:
			bodyDef.Type = KinematicBody
		default:
			bodyDef.Type = DynamicBody
		}

		bodyDef.Position = Vec2{X: float64(i%10) * 2.0, Y: float64(i/10) * 2.0}
		id := w.CreateBody(&bodyDef)

		switch i % 4 {
		case 0:
			w.CreateCircleShape(id, &shapeDef, &circle)
		case 1:
			w.CreatePolygonShape(id, &shapeDef, &box)
		case 2:
			w.CreateCapsuleShape(id, &shapeDef, &capsule)
		default:
			w.CreateSegmentShape(id, &shapeDef, &segment)
		}

		ids = append(ids, id)
	}

	// Destroy every 7th body, then recreate a dynamic body in its place.
	for i := 3; i < len(ids); i += 7 {
		w.DestroyBody(ids[i])
		bodyDef.Type = DynamicBody
		bodyDef.Position = Vec2{X: -float64(i), Y: 3.0}
		ids[i] = w.CreateBody(&bodyDef)
		w.CreateCircleShape(ids[i], &shapeDef, &circle)
	}

	// Sleep some dynamic bodies, wake a subset again.
	for i := 2; i < len(ids); i += 6 {
		if w.BodyType(ids[i]) != StaticBody {
			w.SetBodyAwake(ids[i], false)
		}
	}
	for i := 2; i < len(ids); i += 12 {
		if w.BodyType(ids[i]) != StaticBody {
			w.SetBodyAwake(ids[i], true)
		}
	}

	return ids
}

func TestDeterministicBodyArrays(t *testing.T) {
	def1 := DefaultWorldDef()
	w1 := NewWorld(&def1)
	defer w1.Destroy()

	def2 := DefaultWorldDef()
	w2 := NewWorld(&def2)
	defer w2.Destroy()

	ids1 := buildDeterministicScene(w1)
	ids2 := buildDeterministicScene(w2)

	// The two worlds allocate the same slots with the same generations, but
	// each World stamps its own owner token into world0 (see the DESIGN
	// DEVIATION header in world.go), so the ids are no longer bit-identical
	// across worlds by design. Compare the parts determinism is about — the
	// slot index and the generation — and assert that world0 really is the
	// per-world part rather than dropping the field from the comparison.
	require.Len(t, ids2, len(ids1))
	require.NotEqual(t, w1.worldID, w2.worldID)
	for i := range ids1 {
		tassert.Equal(t, ids1[i].index1, ids2[i].index1, "body %d index", i)
		tassert.Equal(t, ids1[i].generation, ids2[i].generation, "body %d generation", i)
		tassert.Equal(t, w1.worldID, ids1[i].world0, "body %d owner token", i)
		tassert.Equal(t, w2.worldID, ids2[i].world0, "body %d owner token", i)
	}

	// Field-identical sparse body arrays including free slots.
	require.Equal(t, w1.bodies, w2.bodies)

	// Identical solver set layout: same sims in the same order.
	require.Len(t, w2.solverSets, len(w1.solverSets))
	for i := range w1.solverSets {
		tassert.Equal(t, w1.solverSets[i].setIndex, w2.solverSets[i].setIndex, "set %d", i)
		tassert.Equal(t, w1.solverSets[i].bodySims, w2.solverSets[i].bodySims, "set %d bodySims", i)
		tassert.Equal(t, w1.solverSets[i].bodyStates, w2.solverSets[i].bodyStates, "set %d bodyStates", i)
		tassert.Equal(t, w1.solverSets[i].islandSims, w2.solverSets[i].islandSims, "set %d islandSims", i)
	}

	// Identical shape arrays and id pools.
	require.Equal(t, w1.shapes, w2.shapes)
	require.Equal(t, w1.bodyIDPool, w2.bodyIDPool)
	require.Equal(t, w1.shapeIDPool, w2.shapeIDPool)
	require.Equal(t, w1.solverSetIDPool, w2.solverSetIDPool)
	require.Equal(t, w1.islands, w2.islands)
}

func TestCountersMatchScene(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	buildDeterministicScene(w)

	counters := w.Counters()
	tassert.Equal(t, 60, counters.BodyCount)
	tassert.Equal(t, 60, counters.ShapeCount)
	tassert.Equal(t, 0, counters.ContactCount)
	tassert.Equal(t, 0, counters.JointCount)

	// Every enabled dynamic/kinematic body has exactly one island (no
	// contacts or joints exist to merge them).
	islands := 0
	for i := range w.bodies {
		b := &w.bodies[i]
		if b.id != NullIndex && b.islandID != NullIndex {
			islands++
		}
	}
	tassert.Equal(t, islands, counters.IslandCount)

	tassert.GreaterOrEqual(t, counters.TreeHeight, 0)
}
