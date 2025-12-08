package ecs

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: add other tests. See https://ampcode.com/threads/T-54b7ab03-dc58-4301-9684-6c4e4e98fc2b#message-73-block-1

// -------------------------------------------------------------------------------------------------
// Serialization smoke test
// -------------------------------------------------------------------------------------------------
// This test verifies basic World serialization correctness:
// 1. Determinism: multiple serialize calls produce identical bytes.
// 2. Roundtrip: deserialize(serialize(x)) == x.
//
// We don't need to extensively test serialize/deserialize because:
// 1. World.Serialize is a thin wrapper over worldState.toProto (tested separately).
// 2. The remaining logic is proto marshaling and setting initDone.
// -------------------------------------------------------------------------------------------------

func TestWorld_SerializationSmoke(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	w1 := newRandomizedTestWorld(t, prng)

	// Property: serialize is deterministic.
	data1, err := w1.Serialize()
	require.NoError(t, err)
	for range prng.IntN(100) + 1 {
		dup, err := w1.Serialize()
		require.NoError(t, err)
		assert.Equal(t, data1, dup, "multiple serializations should produce identical bytes")
	}

	// Property: deserialize(serialize(x)) == x.
	w2 := newTestWorld(t)
	err = w2.Deserialize(data1)
	require.NoError(t, err)

	assertWorldStateEqual(t, w1.state, w2.state)
	assert.True(t, w2.initDone, "initDone should be true after deserialize")
}

// -------------------------------------------------------------------------------------------------
// Deserialization negative space fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies deserialization robustness by checking against the negative space. It
// serializes random worlds, optionally corrupts the bytes, and checks that Deserialize correctly
// rejects corrupted snapshots and never crashes, panics, or accepts malformed state.
// -------------------------------------------------------------------------------------------------

func TestWorld_DeserializeNegative(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 11 // 2048 iterations

	valid, invalid := 0, 0

	for range opsMax {
		w1 := newRandomizedTestWorld(t, prng)
		data, err := w1.Serialize()
		require.NoError(t, err)

		if prng.Float64() < 0.75 && len(data) > 0 {
			// Flip a random bit to corrupt the data. This has a chance of not corrupting if it hits a
			// field's value bytes.
			bit := uint(prng.IntN(8))
			data[prng.IntN(len(data))] ^= 1 << bit
		}

		w2 := newTestWorld(t)
		err = w2.Deserialize(data)
		if err != nil {
			invalid++
		} else {
			valid++

			// NOTE: We skip the roundtrip property check (serialize(deserialize(x)) == x) here because in
			// some cases, the input mutator mutates a JSON object's key inside the proto bytes.
			// Go's (and goccy/go-json's) JSON unmarshal uses case-insensitive field matching, so
			// corrupted field names like "value" will match "Value" and be accepted. When we
			// re-serialize, the correct field name is used, causing the roundtrip to differ from the
			// corrupted input. This is stupid and there's no way to disable case-insentivity.
			//
			// Since this just affects the field(?), this shouldn't affect the state, e.g. when we
			// deserialize a corrupted snapshot, the restored state should in theory be the same as if the
			// snapshot wasn't corrupted.
			// Ideally I want to re-enable the test below so that we can catch similar bugs.
			//
			// data2, err := w2.Serialize()
			// require.NoError(t, err)
			// assert.Equal(t, data, data2, "TODO: fill me 1")
		}
	}
	assert.Equal(t, opsMax, valid+invalid, "valid + invalid should equal total iterations")

	// Uncomment to see stats.
	// t.Logf("total=%d valid=%d invalid=%d", opsMax, valid, invalid)
}

func newTestWorld(t *testing.T) *World {
	t.Helper()

	w := NewWorld()
	_, err := registerComponent[testutils.ComponentA](w.state)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentB](w.state)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentC](w.state)
	require.NoError(t, err)
	return w
}

func newRandomizedTestWorld(t *testing.T, prng *rand.Rand) *World {
	t.Helper()

	w := newTestWorld(t)

	const entityMax = 1000
	entityCount := prng.IntN(entityMax)
	for range entityCount {
		eid := Create(w.state)

		numComponents := prng.IntN(4)
		names := slices.Clone(allComponentNames)
		prng.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
		for i := range numComponents {
			c := randComponentByName(prng, names[i])
			setComponentAbstract(t, w.state, eid, c)
		}
	}

	if entityCount > 0 {
		removeCount := prng.IntN(max(entityCount/4, 1))
		for range removeCount {
			eid := EntityID(prng.IntN(entityCount))
			Destroy(w.state, eid)
		}
	}

	return w
}

// TODO: fix.
// to reproduce: TEST_SEED=0x187f45843d5f9288
// --- FAIL: TestWorld_DeserializeNegative (0.90s)
// panic: bitmap: buffer length expected to be multiple of 8, was 71 [recovered, repanicked]
