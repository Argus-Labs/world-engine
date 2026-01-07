package ecs

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Exhaustive search[T] iteration test
// -------------------------------------------------------------------------------------------------
// This test exhaustively enumerates all combinations of the following parameters against a fixed
// archetype pool ({}, {A}, {A,B}, {B,C}) to verify search[T].iter() correctness:
//
// - Entity count per archetype (0, 1, or 2): 0 tests empty archetypes yield nothing, 1 tests
//   single-entity iteration, 2 tests multi-entity iteration. Higher counts would cause factorial
//   explosion of the state space (n!).
// - Search components (all subsets of {A, B, C}): tests all 8 possible combinations.
// - Match type (exact vs contains): tests both archetype matching strategies.
// -------------------------------------------------------------------------------------------------

func TestSearch_IterExhaustive(t *testing.T) {
	t.Parallel()

	// Archetype component sets: {}, {A}, {A,B}, {B,C}.
	archComponents := [4][]uint32{{}, {cidA}, {cidA, cidB}, {cidB, cidC}}

	gen := testutils.NewGen()
	for !gen.Done() {
		world := NewWorld()
		ws := world.state

		var s search[struct {
			A Ref[testutils.ComponentA]
			B Ref[testutils.ComponentB]
			C Ref[testutils.ComponentC]
		}]
		_, err := s.init(world)
		require.NoError(t, err)

		// Create 0-2 entities per archetype.
		var model []entityRecord
		var entityCounts [4]int
		for i, cids := range archComponents {
			var bm bitmap.Bitmap
			for _, cid := range cids {
				bm.Set(cid)
			}
			entityCounts[i] = gen.Intn(2)
			for range entityCounts[i] {
				eid := ws.newEntityWithArchetype(bm)
				aid, ok := ws.entityArch.get(eid)
				assert.True(t, ok)
				rec := entityRecord{eid: eid, archID: aid, componentSet: cids}
				for _, cid := range cids {
					switch cid {
					case cidA:
						rec.compA = testutils.ComponentA{X: float64(eid), Y: float64(eid) * 2, Z: float64(eid) * 3}
						err = setComponent(ws, eid, rec.compA)
						require.NoError(t, err)
					case cidB:
						rec.compB = testutils.ComponentB{ID: uint64(eid), Label: "test", Enabled: true}
						err = setComponent(ws, eid, rec.compB)
						require.NoError(t, err)
					case cidC:
						rec.compC = testutils.ComponentC{Values: [8]int32{int32(eid)}, Counter: uint16(eid)}
						err = setComponent(ws, eid, rec.compC)
						require.NoError(t, err)
					}
				}
				model = append(model, rec)
			}
		}

		// Pick non-empty subset of {A,B,C} using 3-bit mask [1,7].
		mask := gen.Intn(7)
		var searchBm bitmap.Bitmap
		for i, cid := range []uint32{cidA, cidB, cidC} {
			if mask&(1<<i) != 0 {
				searchBm.Set(cid)
			}
		}

		// Get matching archetypes (exact or contains).
		exact := gen.Bool()
		var archetypes []archetypeID
		if exact {
			if aid, ok := ws.archExact(searchBm); ok {
				archetypes = []archetypeID{aid}
			}
		} else {
			archetypes = ws.archContains(searchBm)
		}

		// Collect results.
		var results []entityRecord
		for eid, comps := range s.iter(archetypes) {
			aid, ok := ws.entityArch.get(eid)
			assert.True(t, ok)
			arch := ws.archetypes[aid]

			rec := entityRecord{eid: eid}
			if arch.components.Contains(cidA) {
				rec.compA = comps.A.Get()
			}
			if arch.components.Contains(cidB) {
				rec.compB = comps.B.Get()
			}
			if arch.components.Contains(cidC) {
				rec.compC = comps.C.Get()
			}
			results = append(results, rec)
		}

		// Build expected set: entities whose archetype matches the query.
		var expected []entityRecord
		for _, rec := range model {
			if slices.Contains(archetypes, rec.archID) {
				expected = append(expected, rec)
			}
		}

		// Property: completeness (all expected entities yielded, no extras).
		assert.Len(t, results, len(expected), "entity count mismatch")

		for i, res := range results {
			exp := expected[i]

			// Property: correct entity ordering (iter yields entities in model order).
			assert.Equal(t, exp.eid, res.eid, "entity ID mismatch at index %d", i)

			// Property: data integrity (component values match what was set).
			for _, cid := range exp.componentSet {
				switch cid {
				case cidA:
					assert.Equal(t, exp.compA, res.compA, "compA mismatch for entity %d", exp.eid)
				case cidB:
					assert.Equal(t, exp.compB, res.compB, "compB mismatch for entity %d", exp.eid)
				case cidC:
					assert.Equal(t, exp.compC, res.compC, "compC mismatch for entity %d", exp.eid)
				}
			}
		}

		// Uncomment to see all cases checked.
		// t.Logf("search: %s", describeSearch(t,entityCounts, mask, exact))
	}
}

// entityRecord tracks which entities belong to which archetype and their component values.
type entityRecord struct {
	eid          EntityID
	archID       int      // Actual archetype ID from worldState
	componentSet []uint32 // Component IDs in this entity's archetype
	compA        testutils.ComponentA
	compB        testutils.ComponentB
	compC        testutils.ComponentC
}

// describeSearch returns a string describing the search scenario for debugging.
func describeSearch(entityCounts [4]int, mask int, exact bool) string { //nolint:unused // for testing only
	var query string
	if mask&1 != 0 {
		query += "A"
	}
	if mask&2 != 0 {
		query += "B"
	}
	if mask&4 != 0 {
		query += "C"
	}
	matchType := "contains"
	if exact {
		matchType = "exact"
	}
	return fmt.Sprintf("entities=%v, query={%s}, match=%s", entityCounts, query, matchType)
}

// -------------------------------------------------------------------------------------------------
// WithCommand property tests
// -------------------------------------------------------------------------------------------------
// This test verifies WithCommand correctness by comparing iteration results against a model
// (slice of submitted commands). We verify completeness (every submitted command is yielded
// exactly once), no duplicates (no command appears more than once), and data integrity
// (Payload() and Persona() return exactly what was submitted).
// -------------------------------------------------------------------------------------------------

func TestSystem_WithCommand_Properties(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	world := NewWorld()

	// Initialize WithCommand field (this registers the command type).
	var withCmd WithCommand[testutils.SimpleCommand]
	_, err := withCmd.init(world)
	require.NoError(t, err)

	// Generate random commands (model).
	count := prng.IntN(10_000)
	model := make([]micro.Command, count)
	for i := range count {
		model[i] = micro.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Persona: randString(prng, 8),
			Payload: testutils.SimpleCommand{Value: prng.Int()},
		}
	}

	// Submit commands to world.
	world.commands.receiveCommands(model)

	// Iterate and collect results.
	var results []micro.Command
	for ctx := range withCmd.Iter() {
		results = append(results, micro.Command{
			Name:    testutils.SimpleCommand{}.Name(),
			Persona: ctx.Persona(),
			Payload: ctx.Payload(),
		})
	}

	// Property: Iter returns all commands received this tick.
	assert.Len(t, results, len(model), "completeness: expected %d commands, got %d", len(model), len(results))

	// Property: data integrity, each result matches corresponding model entry.
	// This also implicitly verifies no duplicates (if counts match and all match, no dups).
	for i, result := range results {
		assert.Equal(t, model[i].Payload, result.Payload,
			"data integrity: payload mismatch at index %d", i)
		assert.Equal(t, model[i].Persona, result.Persona,
			"data integrity: persona mismatch at index %d", i)
	}
}

// -------------------------------------------------------------------------------------------------
// WithCommand edge case tests
// -------------------------------------------------------------------------------------------------
// Example-based tests for specific edge cases and error conditions.
// -------------------------------------------------------------------------------------------------

func TestSystem_WithCommand_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty iteration", func(t *testing.T) {
		t.Parallel()

		world := NewWorld()

		var withCmd WithCommand[testutils.SimpleCommand]
		_, err := withCmd.init(world)
		require.NoError(t, err)

		// No commands submitted - Iter should yield nothing.
		count := 0
		for range withCmd.Iter() {
			count++
		}
		assert.Equal(t, 0, count, "expected 0 commands, got %d", count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		world := NewWorld()

		var withCmd WithCommand[testutils.SimpleCommand]
		_, err := withCmd.init(world)
		require.NoError(t, err)

		// Submit 10 commands.
		commands := make([]micro.Command, 10)
		for i := range commands {
			commands[i] = micro.Command{
				Name:    testutils.SimpleCommand{}.Name(),
				Persona: "test",
				Payload: testutils.SimpleCommand{Value: i},
			}
		}
		world.commands.receiveCommands(commands)

		// Break after first command.
		count := 0
		for range withCmd.Iter() {
			count++
			break
		}
		assert.Equal(t, 1, count, "expected to process exactly 1 command before break")
	})
}

// -------------------------------------------------------------------------------------------------
// WithEvent property tests
// -------------------------------------------------------------------------------------------------
// This test verifies WithEvent correctness by comparing emitted events against a model
// (slice of emitted values). We verify round-trip integrity (the payload in the queue equals
// exactly what was passed to Emit()) and that init enables Emit without panic.
// -------------------------------------------------------------------------------------------------

func TestSystem_WithEvent_Properties(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	world := NewWorld()

	// Initialize WithEvent field (this registers the event type).
	var withEvent WithEvent[testutils.SimpleEvent]
	_, err := withEvent.init(world)
	require.NoError(t, err)

	// Generate random events (model).
	count := prng.IntN(10_000)
	model := make([]testutils.SimpleEvent, count)
	for i := range count {
		model[i] = testutils.SimpleEvent{Value: prng.Int()}
	}

	// Emit all events.
	for _, event := range model {
		withEvent.Emit(event)
	}

	// Drain queue.
	rawEvents := world.events.getEvents()

	// Property: All emitted events appear in the queue.
	assert.Len(t, rawEvents, len(model), "expected %d events, got %d", len(model), len(rawEvents))

	// Property: Round-trip integrity, each payload equals corresponding model entry.
	for i, raw := range rawEvents {
		payload, ok := raw.Payload.(testutils.SimpleEvent)
		require.True(t, ok, "payload type mismatch at index %d", i)
		assert.Equal(t, model[i], payload, "round-trip integrity: payload mismatch at index %d", i)
	}
}

// -------------------------------------------------------------------------------------------------
// WithEvent edge case tests
// -------------------------------------------------------------------------------------------------
// Example-based tests for specific edge cases and error conditions.
// -------------------------------------------------------------------------------------------------

func TestSystem_WithEvent_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty emission", func(t *testing.T) {
		t.Parallel()

		world := NewWorld()

		var withEvent WithEvent[testutils.SimpleEvent]
		_, err := withEvent.init(world)
		require.NoError(t, err)

		// No events emitted - getEvents should return empty slice.
		rawEvents := world.events.getEvents()
		assert.Empty(t, rawEvents, "expected 0 events, got %d", len(rawEvents))
	})
}

func randString(prng *rand.Rand, n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[prng.IntN(len(letters))]
	}
	return string(b)
}

// -------------------------------------------------------------------------------------------------
// WithSystemEvent property tests
// -------------------------------------------------------------------------------------------------
// This test verifies WithSystemEventEmitter and WithSystemEventReceiver correctness. We test
// them together because they form a producer-consumer pair — the emitter writes to a shared
// buffer and the receiver reads from it. The real properties are:
//
//  1. Wiring correctness (round-trip): Emit(x) on emitter → Iter() on receiver yields x.
//     This verifies both sides resolve to the same underlying buffer via T.Name().
//  2. Type-keyed isolation: Emitting EventA must not appear in a receiver for EventB.
//     This is the channel-separation guarantee.
//
// -------------------------------------------------------------------------------------------------

func TestSystem_WithSystemEvent_Properties(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	world := NewWorld()

	// Initialize emitter and receiver for the same type.
	var emitter WithSystemEventEmitter[testutils.SimpleSystemEvent]
	var receiver WithSystemEventReceiver[testutils.SimpleSystemEvent]
	_, err := emitter.init(world)
	require.NoError(t, err)
	_, err = receiver.init(world)
	require.NoError(t, err)

	// Generate random events (model).
	count := prng.IntN(10_000)
	model := make([]testutils.SimpleSystemEvent, count)
	for i := range count {
		model[i] = testutils.SimpleSystemEvent{Value: prng.Int()}
	}

	// Emit all events.
	for _, event := range model {
		emitter.Emit(event)
	}

	// Collect results via receiver.
	var results []testutils.SimpleSystemEvent
	for event := range receiver.Iter() {
		results = append(results, event)
	}

	// Property: All emitted events are received.
	assert.Len(t, results, len(model), "completeness: expected %d events, got %d", len(model), len(results))

	// Property: Round-trip integrity, each result equals corresponding model entry.
	for i, result := range results {
		assert.Equal(t, model[i], result, "round-trip integrity: event mismatch at index %d", i)
	}
}

// -------------------------------------------------------------------------------------------------
// WithSystemEvent edge case tests
// -------------------------------------------------------------------------------------------------
// Example-based tests for specific edge cases and error conditions.
// -------------------------------------------------------------------------------------------------

func TestSystem_WithSystemEvent_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty iteration", func(t *testing.T) {
		t.Parallel()

		world := NewWorld()

		var receiver WithSystemEventReceiver[testutils.SimpleSystemEvent]
		_, err := receiver.init(world)
		require.NoError(t, err)

		// No events emitted - Iter should yield nothing.
		count := 0
		for range receiver.Iter() {
			count++
		}
		assert.Equal(t, 0, count)
	})

	t.Run("early termination", func(t *testing.T) {
		t.Parallel()

		world := NewWorld()

		var emitter WithSystemEventEmitter[testutils.SimpleSystemEvent]
		var receiver WithSystemEventReceiver[testutils.SimpleSystemEvent]
		_, err := emitter.init(world)
		require.NoError(t, err)
		_, err = receiver.init(world)
		require.NoError(t, err)

		// Emit 10 events.
		for i := range 10 {
			emitter.Emit(testutils.SimpleSystemEvent{Value: i})
		}

		// Break after first event.
		count := 0
		for range receiver.Iter() {
			count++
			break
		}
		assert.Equal(t, 1, count)
	})
}
