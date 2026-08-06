package snapshot

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestMarshalSnapshotByteStability verifies that repeatedly marshaling the same snapshot produces
// stable bytes.
func TestMarshalSnapshotByteStability(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		snapshotCountMax = 10
		marshalCountMax  = 10
	)

	snapshotCount := 2 + prng.IntN(snapshotCountMax-1)
	for range snapshotCount {
		snapshot := randomSnapshot(prng)
		expected, err := marshalSnapshot(snapshot)
		require.NoError(t, err)

		marshalCount := 2 + prng.IntN(marshalCountMax-1)
		for range marshalCount - 1 {
			actual, err := marshalSnapshot(snapshot)
			require.NoError(t, err)

			// Property: serialize(a) == serialize(a).
			assert.Equal(t, expected, actual)
		}
	}
}

// TestSnapshotWireRoundTrip covers the marshal/unmarshal pair the backends share.
func TestSnapshotWireRoundTrip(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		snapshotCountMax  = 10
		roundTripCountMax = 10
	)

	snapshotCount := 2 + prng.IntN(snapshotCountMax-1)
	for range snapshotCount {
		expected := randomSnapshot(prng)

		roundTripCount := 2 + prng.IntN(roundTripCountMax-1)
		for range roundTripCount {
			data, err := marshalSnapshot(expected)
			require.NoError(t, err)

			actual, err := unmarshalSnapshot(data)
			require.NoError(t, err)

			// Property: deserialize(serialize(x)) == x.
			assert.True(t, proto.Equal(expected, actual), "snapshot did not survive a wire roundtrip")
		}
	}
}

func TestUnmarshalSnapshotNegative(t *testing.T) {
	t.Parallel()

	t.Run("rejects malformed wire data", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		const snapshotCountMax = 10

		snapshotCount := 2 + prng.IntN(snapshotCountMax-1)
		for range snapshotCount {
			data, err := marshalSnapshot(randomSnapshot(prng))
			require.NoError(t, err)

			// Append an unterminated, randomly sized varint to corrupt the protobuf wire data.
			for range 1 + prng.IntN(10) {
				data = append(data, 0x80)
			}

			actual, err := unmarshalSnapshot(data)
			assert.Nil(t, actual)
			assert.Error(t, err)
		}
	})

	t.Run("rejects empty component name", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		snapshot := randomSnapshot(prng)
		data, err := marshalSnapshot(snapshot)
		require.NoError(t, err)

		invalidWorldState, err := proto.Marshal(&cardinalv1.WorldState{
			Archetypes: []*cardinalv1.Archetype{{
				Columns: []*cardinalv1.Column{{ComponentName: ""}},
			}},
		})
		require.NoError(t, err)
		data = protowire.AppendTag(data, 3, protowire.BytesType)
		data = protowire.AppendBytes(data, invalidWorldState)

		actual, err := unmarshalSnapshot(data)
		assert.Nil(t, actual)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to validate snapshot")
	})

	t.Run("rejects wrong version", func(t *testing.T) {
		t.Parallel()
		prng := testutils.NewRand(t)

		version := prng.Uint32()
		for version == CurrentVersion {
			version = prng.Uint32()
		}
		snapshot := randomSnapshot(prng)
		data, err := marshalSnapshot(snapshot)
		require.NoError(t, err)
		data = protowire.AppendTag(data, 4, protowire.VarintType)
		data = protowire.AppendVarint(data, uint64(version))

		actual, err := unmarshalSnapshot(data)
		assert.Nil(t, actual)
		require.Error(t, err)
		assert.True(t, eris.Is(err, ErrUnsupportedVersion))
	})
}

func randomSnapshot(prng *rand.Rand) *cardinalv1.Snapshot {
	randomBytes := func(length int) []byte {
		data := make([]byte, length)
		for i := range data {
			data[i] = byte(prng.Uint32())
		}
		return data
	}

	worldState := &cardinalv1.WorldState{
		NextId:     prng.Uint32(),
		FreeIds:    make([]uint32, prng.IntN(32)),
		Archetypes: make([]*cardinalv1.Archetype, prng.IntN(16)),
	}
	for i := range worldState.GetFreeIds() {
		worldState.FreeIds[i] = prng.Uint32()
	}
	for i := range worldState.GetArchetypes() {
		archetype := &cardinalv1.Archetype{
			Id:               prng.Int32(),
			ComponentsBitmap: randomBytes(prng.IntN(32)),
			Entities:         make([]uint32, prng.IntN(32)),
			Columns:          make([]*cardinalv1.Column, prng.IntN(16)),
		}
		for j := range archetype.GetEntities() {
			archetype.Entities[j] = prng.Uint32()
		}
		for j := range archetype.GetColumns() {
			components := make([][]byte, prng.IntN(32))
			for k := range components {
				components[k] = randomBytes(prng.IntN(256))
			}
			archetype.Columns[j] = &cardinalv1.Column{
				ComponentName: fmt.Sprintf("component-%d", prng.Uint64()),
				Components:    components,
			}
		}
		worldState.Archetypes[i] = archetype
	}

	return &cardinalv1.Snapshot{
		TickHeight: prng.Uint64(),
		Timestamp: timestamppb.New(time.Unix(
			prng.Int64N(4_102_444_800),
			prng.Int64N(int64(time.Second)),
		).UTC()),
		WorldState: worldState,
		Version:    CurrentVersion,
	}
}
