package snapshot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
)

// testWorldState is a small entity-major world state exercising every field.
func testWorldState() *cardinalv1.WorldState {
	return &cardinalv1.WorldState{
		NextId:     7,
		Components: []string{"health", "position"},
		Entities: []*cardinalv1.Entity{
			// MessagePack maps: {"Value": 42} and {"X": 1, "Y": 2}.
			{Id: 0, Components: []uint32{0, 1}, Payloads: [][]byte{
				{0x81, 0xa5, 'V', 'a', 'l', 'u', 'e', 0x2a},
				{0x82, 0xa1, 'X', 0x01, 0xa1, 'Y', 0x02},
			}},
			{Id: 2}, // zero components
			{Id: 4, Components: []uint32{1}, Payloads: [][]byte{{0x80}}}, // empty MessagePack map
		},
	}
}

func TestEncodeMetadata(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		tick uint64
		ts   time.Time
	}{
		"typical":        {tick: 42, ts: time.Unix(1_700_000_000, 123_456_789).UTC()},
		"zero tick":      {tick: 0, ts: time.Unix(1_700_000_000, 0).UTC()},
		"zero timestamp": {tick: 9, ts: time.Time{}}, // Unix() is very negative, so still encoded
		"nanos only":     {tick: 1, ts: time.Unix(0, 5).UTC()},
		// The only timestamp whose BODY is empty. The field is still present, so it must be written
		// with a zero length; skipping it decodes as no timestamp at all.
		"unix epoch": {tick: 3, ts: time.Unix(0, 0).UTC()},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := Encode(tc.tick, tc.ts, testWorldState())
			require.NoError(t, err)
			snap, err := Decode(got)
			require.NoError(t, err)
			assert.Equal(t, tc.tick, snap.GetTickHeight())
			require.NotNil(t, snap.GetTimestamp())
			assert.True(t, tc.ts.Equal(snap.GetTimestamp().AsTime()))
			assert.Equal(t, uint32(2), snap.GetVersion())
			assert.True(t, proto.Equal(testWorldState(), snap.GetWorldState()))
		})
	}
}

// TestEncodeDecodeRoundTrip: bytes out of Encode come back as the same message through Decode.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Unix(1_700_000_000, 42).UTC()

	data, err := Encode(11, ts, testWorldState())
	require.NoError(t, err)

	snap, err := Decode(data)
	require.NoError(t, err)
	assert.Equal(t, uint64(11), snap.GetTickHeight())
	assert.Equal(t, ts, snap.GetTimestamp().AsTime())
	assert.Equal(t, CurrentVersion, snap.GetVersion())
	assert.True(t, proto.Equal(testWorldState(), snap.GetWorldState()))
}

// TestDecodeRejects: malformed bytes, schema violations, and foreign versions are refused — never
// decoded into a world state.
func TestDecodeRejects(t *testing.T) {
	t.Parallel()

	encode := func(snap *cardinalv1.Snapshot) []byte {
		data, err := proto.Marshal(snap)
		require.NoError(t, err)
		return data
	}

	t.Run("malformed wire data", func(t *testing.T) {
		t.Parallel()
		data := encode(&cardinalv1.Snapshot{
			WorldState: testWorldState(), Version: CurrentVersion,
		})
		data = append(data, 0x80) // unterminated varint

		snap, err := Decode(data)
		assert.Nil(t, snap)
		require.Error(t, err)
	})

	t.Run("missing world state", func(t *testing.T) {
		t.Parallel()
		snap, err := Decode(encode(&cardinalv1.Snapshot{Version: CurrentVersion}))
		assert.Nil(t, snap)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to validate snapshot")
	})

	t.Run("empty component name", func(t *testing.T) {
		t.Parallel()
		snap, err := Decode(encode(&cardinalv1.Snapshot{
			WorldState: &cardinalv1.WorldState{Components: []string{""}},
			Version:    CurrentVersion,
		}))
		assert.Nil(t, snap)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to validate snapshot")
	})

	for name, version := range map[string]uint32{
		"old protobuf payload version": 1,
		"newer version":                CurrentVersion + 1,
		"unset version":                0,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap, err := Decode(encode(&cardinalv1.Snapshot{
				WorldState: testWorldState(), Version: version,
			}))
			assert.Nil(t, snap)
			require.Error(t, err)
			assert.True(t, eris.Is(err, ErrUnsupportedVersion))
		})
	}

	// A format this build cannot read is unlikely to satisfy this build's schema, which is the
	// whole point of the version field. The refusal must still name the version, not describe how
	// the file differs from today's shape.
	t.Run("wrong version that also fails the current schema", func(t *testing.T) {
		t.Parallel()
		snap, err := Decode(encode(&cardinalv1.Snapshot{
			Version: CurrentVersion + 1, // WorldState omitted: required under the current schema
		}))
		assert.Nil(t, snap)
		require.Error(t, err)
		assert.True(t, eris.Is(err, ErrUnsupportedVersion), "got %v", err)
	})
}

// TestValidateVersion pins the acceptance policy: exactly the version this build writes is
// readable; everything else is refused instead of mis-read.
func TestValidateVersion(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateVersion(CurrentVersion))
	assert.Equal(t, uint32(2), CurrentVersion)
	for _, v := range []uint32{0, 1, CurrentVersion + 1, 99} {
		err := ValidateVersion(v)
		require.Error(t, err)
		assert.True(t, eris.Is(err, ErrUnsupportedVersion))
	}
}
