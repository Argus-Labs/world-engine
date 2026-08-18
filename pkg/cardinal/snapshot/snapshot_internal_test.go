package snapshot

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// goldenSnapshotBytes is the wire encoding of goldenSnapshot() at format version 2, which
// describes the world (entities and their components by name) instead of mirroring the runtime
// layout. The envelope is a persisted format: these bytes must not change.
const goldenSnapshotBytes = "082a120b0880e2cfaa0610959aef3a1a4408071204010204061a0e0a064865616c" +
	"74681201021a012a1a150a08506f736974696f6e120201041a0201021a01031a130a0856656c6f63697479120201" +
	"041a001a01ff2002"

// goldenSnapshot is a fixed envelope covering every field of Snapshot, WorldState and Column,
// including the edge cases that are easy to break: an empty payload, a column spanning
// non-contiguous entities, and a live entity that appears in no column (entity 6 has zero
// components). Entities 1, 2, 4, 6 are alive of next_id 7, so 0, 3 and 5 are implied free.
func goldenSnapshot() *cardinalv1.Snapshot {
	return &cardinalv1.Snapshot{
		TickHeight: 42,
		Timestamp:  timestamppb.New(time.Unix(1700000000, 123456789).UTC()),
		Version:    CurrentVersion,
		WorldState: &cardinalv1.WorldState{
			NextId:        7,
			LiveEntityIds: []uint32{1, 2, 4, 6},
			Columns: []*cardinalv1.Column{
				{Name: "Health", EntityIds: []uint32{2}, Payloads: [][]byte{{0x2a}}},
				{Name: "Position", EntityIds: []uint32{1, 4}, Payloads: [][]byte{{0x01, 0x02}, {0x03}}},
				{Name: "Velocity", EntityIds: []uint32{1, 4}, Payloads: [][]byte{{}, {0xff}}},
			},
		},
	}
}

// TestMarshalSnapshotByteStability pins the on-disk snapshot format. Every backend writes exactly
// what marshalSnapshot produces, so a change here is a change to persisted data. Existing
// snapshots still carry CurrentVersion, so a layout change that keeps the version constant is
// accepted by ValidateVersion and mis-read: bump CurrentVersion with the layout.
func TestMarshalSnapshotByteStability(t *testing.T) {
	t.Parallel()

	data, err := marshalSnapshot(goldenSnapshot())
	require.NoError(t, err)
	assert.Equal(t, goldenSnapshotBytes, hex.EncodeToString(data),
		"snapshot envelope encoding changed; existing snapshots would be read differently")
}

// TestMarshalSnapshotDetectsEnvelopeChange proves the byte-stability assertion above is load
// bearing: any envelope field or ordering change moves the bytes.
func TestMarshalSnapshotDetectsEnvelopeChange(t *testing.T) {
	t.Parallel()

	golden, err := marshalSnapshot(goldenSnapshot())
	require.NoError(t, err)

	// No "version" mutation: marshalSnapshot asserts the version it writes, and the version byte
	// is already pinned by goldenSnapshotBytes.
	mutations := map[string]func(*cardinalv1.Snapshot){
		"tick height": func(s *cardinalv1.Snapshot) { s.TickHeight++ },
		"timestamp":   func(s *cardinalv1.Snapshot) { s.Timestamp = timestamppb.New(time.Unix(1, 0)) },
		"column order": func(s *cardinalv1.Snapshot) {
			cols := s.GetWorldState().GetColumns()
			cols[0], cols[1] = cols[1], cols[0]
		},
		"entity order": func(s *cardinalv1.Snapshot) {
			ents := s.GetWorldState().GetColumns()[1].GetEntityIds()
			ents[0], ents[1] = ents[1], ents[0]
		},
		"live list": func(s *cardinalv1.Snapshot) {
			ws := s.GetWorldState()
			ws.LiveEntityIds = append(ws.GetLiveEntityIds(), 9)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			snap := goldenSnapshot()
			mutate(snap)
			data, err := marshalSnapshot(snap)
			require.NoError(t, err)
			assert.NotEqual(t, golden, data, "mutating the %s must change the encoded bytes", name)
		})
	}
}

// TestSnapshotWireRoundTrip covers the marshal/unmarshal pair the backends share.
func TestSnapshotWireRoundTrip(t *testing.T) {
	t.Parallel()

	want := goldenSnapshot()
	data, err := marshalSnapshot(want)
	require.NoError(t, err)

	got, err := unmarshalSnapshot(data)
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, got), "snapshot did not survive a wire roundtrip")

	// Re-encoding the decoded envelope must reproduce the same bytes.
	again, err := marshalSnapshot(got)
	require.NoError(t, err)
	assert.Equal(t, data, again)
}

// TestUnmarshalSnapshotValidates guards the one protovalidate rule the schema carries: a column
// must name its component, otherwise a restore cannot match it back to a registered type.
func TestUnmarshalSnapshotValidates(t *testing.T) {
	t.Parallel()

	snap := goldenSnapshot()
	snap.GetWorldState().GetColumns()[0].Name = ""
	data, err := marshalSnapshot(snap)
	require.NoError(t, err)

	_, err = unmarshalSnapshot(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to validate snapshot")
}

// TestUnmarshalSnapshotRejectsMalformedBytes: corrupt wire data must error, never decode into a
// world state.
func TestUnmarshalSnapshotRejectsMalformedBytes(t *testing.T) {
	t.Parallel()

	data, err := marshalSnapshot(goldenSnapshot())
	require.NoError(t, err)
	data = append(data, 0x80) // unterminated varint

	snap, err := unmarshalSnapshot(data)
	assert.Nil(t, snap)
	require.Error(t, err)
}

// TestValidateVersion pins the acceptance policy: exactly the version this build writes is
// readable, and every other value — unset, older, newer — is refused instead of mis-read.
func TestValidateVersion(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateVersion(CurrentVersion))

	// An older version must fail rather than be decoded into the new layout: version 1 is still
	// decodable by the current schema, so this check is all that stands between it and a silently
	// wrong world. Newer and unset are refused for the same reason.
	for name, version := range map[string]uint32{
		"unset":     0,
		"older":     CurrentVersion - 1,
		"newer":     CurrentVersion + 1,
		"far newer": 99,
	} {
		t.Run(name+" is rejected", func(t *testing.T) {
			t.Parallel()

			err := ValidateVersion(version)
			require.Error(t, err)
			assert.True(t, eris.Is(err, ErrUnsupportedVersion), "must be identifiable as a version error")
		})
	}
}

// TestUnmarshalSnapshotChecksVersion covers the decode boundary both real backends share: bytes
// from a build with a newer format must be refused, not decoded into a world state.
func TestUnmarshalSnapshotChecksVersion(t *testing.T) {
	t.Parallel()

	t.Run("current version loads", func(t *testing.T) {
		t.Parallel()

		data, err := marshalSnapshot(goldenSnapshot())
		require.NoError(t, err)

		got, err := unmarshalSnapshot(data)
		require.NoError(t, err)
		assert.Equal(t, CurrentVersion, got.GetVersion())
	})

	for name, version := range map[string]uint32{"newer": CurrentVersion + 1, "unset": 0} {
		t.Run(name+" version is rejected", func(t *testing.T) {
			t.Parallel()

			// Raw proto.Marshal, not marshalSnapshot: the writer path asserts the version it
			// writes, and this test is about the decode boundary refusing foreign bytes.
			snap := goldenSnapshot()
			snap.Version = version
			data, err := proto.MarshalOptions{Deterministic: true}.Marshal(snap)
			require.NoError(t, err)

			got, err := unmarshalSnapshot(data)
			assert.Nil(t, got, "a snapshot this build cannot read must not be handed back")
			require.Error(t, err)
			assert.True(t, eris.Is(err, ErrUnsupportedVersion))
		})
	}
}

// TestNopStorage documents the default storage type: Store does no work and Load always reports
// that no snapshot exists.
func TestNopStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := NewNopStorage()
	require.NoError(t, store.Store(ctx, goldenSnapshot()))

	got, err := store.Load(ctx)
	assert.Nil(t, got)
	require.Error(t, err)
	assert.True(t, eris.Is(err, ErrSnapshotNotFound))
}

// TestNopStorageIsFree pins the reason Store takes a proto instead of pre-marshaled bytes: with
// the default storage type a snapshot must cost nothing.
func TestNopStorageIsFree(t *testing.T) {
	ctx := context.Background()
	store := NewNopStorage()
	snap := goldenSnapshot()

	allocs := testing.AllocsPerRun(100, func() {
		if err := store.Store(ctx, snap); err != nil {
			t.Fatal(err)
		}
	})
	assert.Zero(t, allocs, "NopStorage.Store must not allocate")
}

// randomSnapshot builds a valid flat-format snapshot with randomized contents, for tests that need
// distinct snapshots (the writer's ordering tests) rather than pinned bytes.
func randomSnapshot(prng *rand.Rand) *cardinalv1.Snapshot {
	randomBytes := func(length int) []byte {
		data := make([]byte, length)
		for i := range data {
			data[i] = byte(prng.Uint32())
		}
		return data
	}

	live := make([]uint32, prng.IntN(32))
	for i := range live {
		live[i] = prng.Uint32()
	}

	columns := make([]*cardinalv1.Column, prng.IntN(8))
	for i := range columns {
		ids := make([]uint32, prng.IntN(16))
		payloads := make([][]byte, len(ids))
		for j := range ids {
			ids[j] = prng.Uint32()
			payloads[j] = randomBytes(prng.IntN(64))
		}
		columns[i] = &cardinalv1.Column{
			Name:      fmt.Sprintf("component_%d", i),
			EntityIds: ids,
			Payloads:  payloads,
		}
	}

	return &cardinalv1.Snapshot{
		TickHeight: prng.Uint64(),
		Timestamp: timestamppb.New(time.Unix(
			prng.Int64N(4_102_444_800),
			prng.Int64N(int64(time.Second)),
		).UTC()),
		Version: CurrentVersion,
		WorldState: &cardinalv1.WorldState{
			NextId:        prng.Uint32(),
			LiveEntityIds: live,
			Columns:       columns,
		},
	}
}
