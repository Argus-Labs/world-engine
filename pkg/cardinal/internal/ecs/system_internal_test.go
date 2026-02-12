package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: test system registration to make sure scheduler deps are correct.

// -------------------------------------------------------------------------------------------------
// Search, Contains, Exact, smoke tests
// -------------------------------------------------------------------------------------------------
// The search fields and Ref are just light wrappers over the world state operations, Which is
// already tested. Here, we just check if the regular search operations work. Most of the
// complicated logic in initialization where the cached result is created using reflection. We can
// verify it's working if the operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestSearch_Smoke(t *testing.T) {
	t.Parallel()

	t.Run("iter contains", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		// Create random movers and singles; only movers should appear in Iter.
		var expectedIDs []EntityID
		for range prng.IntN(100) {
			if prng.IntN(2) == 0 {
				eid, _ := fixture.Movers.Create()
				expectedIDs = append(expectedIDs, eid)
			} else {
				fixture.Singles.Create()
			}
		}

		var moverIDs []EntityID
		for eid := range fixture.Movers.Iter() {
			moverIDs = append(moverIDs, eid)
		}
		assert.Equal(t, expectedIDs, moverIDs)
	})

	t.Run("iter exact", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		// Create random singles and movers; only singles should appear in Iter.
		var expectedIDs []EntityID
		for range prng.IntN(100) {
			if prng.IntN(2) == 0 {
				eid, _ := fixture.Singles.Create()
				expectedIDs = append(expectedIDs, eid)
			} else {
				fixture.Movers.Create()
			}
		}

		var singleIDs []EntityID
		for eid := range fixture.Singles.Iter() {
			singleIDs = append(singleIDs, eid)
		}
		assert.Equal(t, expectedIDs, singleIDs)
	})

	t.Run("get by id", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		compB := testutils.ComponentB{ID: prng.Uint64(), Label: testutils.RandString(prng, 8), Enabled: prng.IntN(2) == 1}
		moverID, mover := fixture.Movers.Create()
		mover.A.Set(testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()})
		mover.B.Set(compB)

		singleID, single := fixture.Singles.Create()
		single.A.Set(testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()})

		// Success: correct archetype.
		moverResult, err := fixture.Movers.GetByID(moverID)
		require.NoError(t, err)
		assert.Equal(t, compB, moverResult.B.Get())

		// Wrong archetype.
		_, err = fixture.Movers.GetByID(singleID)
		require.ErrorIs(t, err, ErrArchetypeMismatch)

		_, err = fixture.Singles.GetByID(moverID)
		require.ErrorIs(t, err, ErrArchetypeMismatch)

		// Nonexistent entity.
		_, err = fixture.Movers.GetByID(999)
		require.ErrorIs(t, err, ErrEntityNotFound)
	})

	t.Run("get set remove", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSearchFixture(t)

		eid, mover := fixture.Movers.Create()

		// Get returns zero value before Set.
		assert.Equal(t, testutils.ComponentA{}, mover.A.Get())

		// Set then Get round-trips the value.
		compA := testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		mover.A.Set(compA)
		assert.Equal(t, compA, mover.A.Get())

		// Overwrite with a new value.
		compA2 := testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
		mover.A.Set(compA2)
		assert.Equal(t, compA2, mover.A.Get())

		// Remove changes the archetype, so the entity no longer matches Movers.
		mover.A.Remove()
		_, err := fixture.Movers.GetByID(eid)
		require.ErrorIs(t, err, ErrArchetypeMismatch)
	})

	t.Run("destroy", func(t *testing.T) {
		t.Parallel()
		fixture := newSearchFixture(t)

		eid, _ := fixture.Movers.Create()

		// Destroy succeeds once, then fails on the same ID.
		assert.True(t, fixture.Movers.Destroy(eid))
		assert.False(t, fixture.Movers.Destroy(eid))
	})
}

type searchFixture struct {
	// These are the fields under test. They must be public/exported.
	Movers Contains[struct {
		A Ref[testutils.ComponentA]
		B Ref[testutils.ComponentB]
	}]
	Singles Exact[struct {
		A Ref[testutils.ComponentA]
	}]
}

func newSearchFixture(t *testing.T) *searchFixture {
	t.Helper()

	world := NewWorld()

	fixture := &searchFixture{}
	_, err := initSystemFields(fixture, world)
	require.NoError(t, err)

	return fixture
}

// -------------------------------------------------------------------------------------------------
// WithSystemEvent smoke tests
// -------------------------------------------------------------------------------------------------
// WithSystemEventEmitter and WithSystemEventReceiver are just light wrappers over the
// systemEventManager, which is already tested. Here, we just check if the regular system event
// operations work correctly.
// -------------------------------------------------------------------------------------------------

func TestSystem_WithSystemEvent(t *testing.T) {
	t.Parallel()

	t.Run("round trip", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)
		fixture := newSystemEventFixture(t)

		count := prng.IntN(10_000)
		model := make([]testutils.SimpleSystemEvent, count)
		for i := range count {
			model[i] = testutils.SimpleSystemEvent{Value: prng.Int()}
		}

		for _, event := range model {
			fixture.Emitter.Emit(event)
		}

		var results []testutils.SimpleSystemEvent
		for event := range fixture.Receiver.Iter() {
			results = append(results, event)
		}

		assert.Len(t, results, len(model), "completeness: expected %d events, got %d", len(model), len(results))
		for i, result := range results {
			assert.Equal(t, model[i], result, "round-trip integrity: event mismatch at index %d", i)
		}
	})

	t.Run("empty iteration", func(t *testing.T) {
		t.Parallel()

		fixture := newSystemEventFixture(t)

		count := 0
		for range fixture.Receiver.Iter() {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		fixture := newSystemEventFixture(t)

		for i := range 10 {
			fixture.Emitter.Emit(testutils.SimpleSystemEvent{Value: i})
		}

		count := 0
		for range fixture.Receiver.Iter() {
			count++
			break
		}
		assert.Equal(t, 1, count)
	})
}

type systemEventFixture struct {
	Emitter  WithSystemEventEmitter[testutils.SimpleSystemEvent]
	Receiver WithSystemEventReceiver[testutils.SimpleSystemEvent]
}

func newSystemEventFixture(t *testing.T) *systemEventFixture {
	t.Helper()

	world := NewWorld()
	fixture := &systemEventFixture{}

	meta := &systemInitMetadata{world: world, systemEvents: make(map[string]struct{})}
	err := fixture.Emitter.init(meta)
	require.NoError(t, err)

	meta = &systemInitMetadata{world: world, systemEvents: make(map[string]struct{})}
	err = fixture.Receiver.init(meta)
	require.NoError(t, err)

	return fixture
}
