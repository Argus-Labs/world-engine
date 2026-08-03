package snapshot

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// goldenSnapshotBytes is the wire encoding of goldenSnapshot(), captured from the write path that
// marshaled the world state in cardinal.go and re-marshaled the envelope inside the backend. The
// envelope is a persisted format: these bytes must not change.
const goldenSnapshotBytes = "082a120b0880e2cfaa0610959aef3a1a750807120203051a0d00fffffffffffffffff" +
	"f01010022391201031a0c00ffffffffffffffffff0101220201042a110a08506f736974696f6e12020102120103" +
	"2a0f0a0856656c6f6369747912001201ff22230801120205001a0bffffffffffffffffff01002201022a0b0a064" +
	"865616c746812012a2001"

// goldenSnapshot is a fixed envelope covering every field of Snapshot, WorldState, Archetype and
// Column, including the edge cases that are easy to break: a negative sparse-set tombstone, an
// empty component blob, and a multi-byte bitmap.
func goldenSnapshot() *cardinalv1.Snapshot {
	return &cardinalv1.Snapshot{
		TickHeight: 42,
		Timestamp:  timestamppb.New(time.Unix(1700000000, 123456789).UTC()),
		Version:    CurrentVersion,
		WorldState: &cardinalv1.WorldState{
			NextId:     7,
			FreeIds:    []uint32{3, 5},
			EntityArch: []int64{0, -1, 1, 0},
			Archetypes: []*cardinalv1.Archetype{
				{
					Id:               0,
					ComponentsBitmap: []byte{0x03},
					Rows:             []int64{0, -1, 1},
					Entities:         []uint32{1, 4},
					Columns: []*cardinalv1.Column{
						{ComponentName: "Position", Components: [][]byte{{0x01, 0x02}, {0x03}}},
						{ComponentName: "Velocity", Components: [][]byte{{}, {0xff}}},
					},
				},
				{
					Id:               1,
					ComponentsBitmap: []byte{0x05, 0x00},
					Rows:             []int64{-1, 0},
					Entities:         []uint32{2},
					Columns: []*cardinalv1.Column{
						{ComponentName: "Health", Components: [][]byte{{0x2a}}},
					},
				},
			},
		},
	}
}

// TestMarshalSnapshotByteStability pins the on-disk snapshot format. Every backend writes exactly
// what marshalSnapshot produces, so a change here is a change to persisted data that no reader
// would reject: snapshot.CurrentVersion is written but never validated on load.
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

	mutations := map[string]func(*cardinalv1.Snapshot){
		"tick height": func(s *cardinalv1.Snapshot) { s.TickHeight++ },
		"timestamp":   func(s *cardinalv1.Snapshot) { s.Timestamp = timestamppb.New(time.Unix(1, 0)) },
		"version":     func(s *cardinalv1.Snapshot) { s.Version++ },
		"column order": func(s *cardinalv1.Snapshot) {
			cols := s.GetWorldState().GetArchetypes()[0].GetColumns()
			cols[0], cols[1] = cols[1], cols[0]
		},
		"entity order": func(s *cardinalv1.Snapshot) {
			ents := s.GetWorldState().GetArchetypes()[0].GetEntities()
			ents[0], ents[1] = ents[1], ents[0]
		},
		"sparse set length": func(s *cardinalv1.Snapshot) {
			ws := s.GetWorldState()
			ws.EntityArch = append(ws.GetEntityArch(), -1)
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
	snap.GetWorldState().GetArchetypes()[0].GetColumns()[0].ComponentName = ""
	data, err := marshalSnapshot(snap)
	require.NoError(t, err)

	_, err = unmarshalSnapshot(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to validate snapshot")
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
