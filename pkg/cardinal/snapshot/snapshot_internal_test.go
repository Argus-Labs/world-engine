package snapshot

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
)

// testWorldState is a small entity-major world state exercising every field.
func testWorldState() *cardinalv1.WorldState {
	return &cardinalv1.WorldState{
		NextId:     7,
		Components: []string{"health", "position"},
		Entities: []*cardinalv1.Entity{
			{Id: 0, Components: []uint32{0, 1}, Payloads: [][]byte{{0x2a}, {0x01, 0x02}}},
			{Id: 2}, // zero components
			{Id: 4, Components: []uint32{1}, Payloads: [][]byte{{}}}, // empty payload is legal
		},
	}
}

// TestEnvelopeCanonical: the hand-encoded envelope must be byte-identical to proto.Marshal of the
// same Snapshot message. The body is opaque to the envelope, so any valid WorldState bytes work as
// the fixture.
func TestEnvelopeCanonical(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		tick uint64
		ts   time.Time
	}{
		"typical":        {tick: 42, ts: time.Unix(1_700_000_000, 123_456_789).UTC()},
		"zero tick":      {tick: 0, ts: time.Unix(1_700_000_000, 0).UTC()},
		"zero timestamp": {tick: 9, ts: time.Time{}},
		"nanos only":     {tick: 1, ts: time.Unix(0, 5).UTC()},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			body, err := proto.Marshal(testWorldState())
			require.NoError(t, err)

			got := Encode(tc.tick, tc.ts, len(body), func(b []byte) []byte {
				return append(b, body...)
			})

			want, err := proto.MarshalOptions{Deterministic: true}.Marshal(&cardinalv1.Snapshot{
				TickHeight: tc.tick,
				Timestamp:  timestamppb.New(tc.ts),
				WorldState: testWorldState(),
				Version:    CurrentVersion,
			})
			require.NoError(t, err)

			assert.Equal(t, want, got, "hand-encoded envelope must match proto.Marshal exactly")
			assert.Len(t, got, EnvelopeSize(tc.tick, tc.ts, len(body)), "EnvelopeSize must be exact")
		})
	}
}

// TestEncodeDecodeRoundTrip: bytes out of Encode come back as the same message through Decode.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	body, err := proto.Marshal(testWorldState())
	require.NoError(t, err)
	ts := time.Unix(1_700_000_000, 42).UTC()

	data := Encode(11, ts, len(body), func(b []byte) []byte { return append(b, body...) })

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
		"older version": CurrentVersion - 1,
		"newer version": CurrentVersion + 1,
		"unset version": 0,
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
}

// TestValidateVersion pins the acceptance policy: exactly the version this build writes is
// readable; everything else is refused instead of mis-read.
func TestValidateVersion(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateVersion(CurrentVersion))
	for _, v := range []uint32{0, CurrentVersion - 1, CurrentVersion + 1, 99} {
		err := ValidateVersion(v)
		require.Error(t, err)
		assert.True(t, eris.Is(err, ErrUnsupportedVersion))
	}
}
