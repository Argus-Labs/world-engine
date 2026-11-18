package cardinal

import (
	"bytes"
	"math"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// newCommandIDFixture publishes an avatar that alice has authority over and bob can only view.
func newCommandIDFixture(t *testing.T) (*accessFixture, syncstate.ID) {
	t.Helper()
	f := newAccessFixture(t, "alice", "bob")
	avatar := f.Create[syncAvatar]()
	avatar.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	f.rejections(t, "alice")
	f.rejections(t, "bob")
	return f, avatar.SyncID()
}

// castWithID is a cast at target carrying a command ID.
func castWithID(target syncstate.ID, id uint64) accessCast {
	cmd := accessCast{Target: syncstate.TargetOf(target)}
	cmd.CommandID = id
	return cmd
}

// rejections returns the command_rejected events player received, and clears its queue.
func (f *accessFixture) rejections(t *testing.T, player string) []syncstate.CommandRejected {
	t.Helper()
	var got []syncstate.CommandRejected
	for _, q := range f.subs[player].queue {
		if q.response.GetEvent().GetName() != "command_rejected" {
			continue
		}
		var r syncstate.CommandRejected
		require.NoError(t, msgpack.Unmarshal(q.response.GetEvent().GetPayload(), &r))
		got = append(got, r)
	}
	f.subs[player].queue, f.subs[player].queuedBytes = nil, 0
	return got
}

func rejected(reason syncstate.Reason, id uint64) syncstate.CommandRejected {
	return syncstate.CommandRejected{Command: "access_cast", Reason: reason, CommandID: id}
}

// A retry of an admitted command, in a later tick or the same one, never runs the handler again.
func TestSyncCommandIDRepeatOfAdmittedRunsOnce(t *testing.T) {
	f, avatar := newCommandIDFixture(t)

	f.Command("alice", castWithID(avatar, 7))
	f.Tick()
	require.Len(t, *f.casts, 1)

	f.Command("alice", castWithID(avatar, 7)) // the client's SendCommand failed, so it retries
	f.Tick()
	assert.Len(t, *f.casts, 1, "a repeat of an admitted command is dropped")
	assert.Empty(t, f.rejections(t, "alice"), "and dropped silently")

	f.Command("alice", castWithID(avatar, 8))
	f.Command("alice", castWithID(avatar, 8)) // the first copy and the repeat in one tick
	f.Tick()
	assert.Len(t, *f.casts, 2, "a new ID is admitted once, even when it arrives twice in one tick")
	assert.Empty(t, f.rejections(t, "alice"))
}

// A retry of a rejected command is rejected again with the first verdict, even if the world now
// would admit it, so a lost rejection is repeated and the command never runs.
func TestSyncCommandIDRepeatOfRejectedIsRejectedAgain(t *testing.T) {
	f, avatar := newCommandIDFixture(t)

	f.Command("bob", castWithID(avatar, 9))
	f.Tick()
	assert.Equal(t, []syncstate.CommandRejected{rejected(syncstate.NoAuthority, 9)}, f.rejections(t, "bob"))

	f.Entity(EntityID(avatar.Entity)).Set(syncstate.Authorities{Players: players("alice", "bob")})
	f.Tick() // publishes bob's authority
	f.rejections(t, "bob")

	f.Command("bob", castWithID(avatar, 9))
	f.Command("bob", castWithID(avatar, 9))
	f.Tick()
	assert.Empty(t, *f.casts, "a repeat of a rejected command never reaches the handler")
	assert.Equal(t,
		[]syncstate.CommandRejected{rejected(syncstate.NoAuthority, 9), rejected(syncstate.NoAuthority, 9)},
		f.rejections(t, "bob"), "each copy gets the first rejection again")

	f.Command("bob", castWithID(avatar, 10))
	f.Tick()
	assert.Len(t, *f.casts, 1, "a new ID is judged against the world as published")
}

// Two players' commands with the same ID are judged apart.
func TestSyncCommandIDsArePerPlayer(t *testing.T) {
	f, avatar := newCommandIDFixture(t)
	f.Entity(EntityID(avatar.Entity)).Set(syncstate.Authorities{Players: players("alice", "bob")})
	f.Tick()

	f.Command("alice", castWithID(avatar, 5))
	f.Command("bob", castWithID(avatar, 5))
	f.Tick()
	require.Len(t, *f.casts, 2, "bob's 5 is not alice's repeat")
	assert.Equal(t, "alice", (*f.casts)[0].sender)
	assert.Equal(t, "bob", (*f.casts)[1].sender)

	f.Entity(EntityID(avatar.Entity)).Set(syncstate.Authorities{Players: players("bob")})
	f.Tick()
	f.rejections(t, "alice")
	f.Command("alice", castWithID(avatar, 6)) // rejected: alice lost authority
	f.Command("bob", castWithID(avatar, 6))
	f.Tick()
	assert.Equal(t, []syncstate.CommandRejected{rejected(syncstate.NoAuthority, 6)}, f.rejections(t, "alice"))
	assert.Empty(t, f.rejections(t, "bob"))
	assert.Len(t, *f.casts, 3, "alice's rejection does not reject bob's command")
}

// An ID is remembered for 30 s of the tick's server time, then judged afresh. The test world's
// step at height h runs at Unix(h).
func TestSyncCommandIDsAreForgottenAfter30Seconds(t *testing.T) {
	f, avatar := newCommandIDFixture(t)
	judgedAt := f.TickHeight()
	f.Command("alice", castWithID(avatar, 20))
	f.Command("bob", castWithID(avatar, 21))
	f.Tick()
	require.Len(t, *f.casts, 1)
	require.Len(t, f.rejections(t, "bob"), 1)

	for f.TickHeight() < judgedAt+29 {
		f.Tick()
	}
	f.Command("alice", castWithID(avatar, 20))
	f.Command("bob", castWithID(avatar, 21))
	f.Tick() // 29 s after the first copies
	assert.Len(t, *f.casts, 1, "still remembered at 29 s")
	assert.Equal(t, []syncstate.CommandRejected{rejected(syncstate.NoAuthority, 21)}, f.rejections(t, "bob"))

	f.Entity(EntityID(avatar.Entity)).Set(syncstate.Authorities{Players: players("alice", "bob")})
	f.Command("alice", castWithID(avatar, 20))
	f.Command("bob", castWithID(avatar, 21))
	f.Tick() // 30 s after: both are judged afresh, against what the tick before published
	assert.Len(t, *f.casts, 2, "alice's 20 is admitted again")
	assert.Equal(t, []syncstate.CommandRejected{rejected(syncstate.NoAuthority, 21)}, f.rejections(t, "bob"),
		"bob's 21 is judged again: the authority it now has was not yet published")

	for f.TickHeight() < judgedAt+61 {
		f.Tick()
	}
	assert.Empty(t, f.judged, "a player whose IDs are all forgotten leaves no entry")
}

// Past 256 IDs a player's oldest is forgotten first.
func TestSyncCommandIDsKeepThePlayersNewest256(t *testing.T) {
	f, avatar := newCommandIDFixture(t)
	for id := range uint64(257) {
		f.Command("alice", castWithID(avatar, id+1))
	}
	f.Tick()
	require.Len(t, *f.casts, 257)

	f.Command("alice", castWithID(avatar, 2)) // the oldest of the newest 256
	f.Tick()
	assert.Len(t, *f.casts, 257, "ID 2 is still remembered")
	f.Command("alice", castWithID(avatar, 1))
	f.Tick()
	assert.Len(t, *f.casts, 258, "ID 1 was dropped to keep 256")
}

// ID 0 means no ID: every copy is judged and may run.
func TestSyncCommandIDZeroIsNeverDeduplicated(t *testing.T) {
	f, avatar := newCommandIDFixture(t)
	f.Command("alice", castWithID(avatar, 0))
	f.Command("alice", castWithID(avatar, 0))
	f.Command("bob", castWithID(avatar, 0))
	f.Tick()
	f.Command("alice", castWithID(avatar, 0))
	f.Tick()
	assert.Len(t, *f.casts, 3)
	assert.Equal(t, []syncstate.CommandRejected{rejected(syncstate.NoAuthority, 0)}, f.rejections(t, "bob"))
}

// With two commands of one kind in flight, each rejection names the command it rejects.
func TestSyncCommandRejectionNamesItsCommandID(t *testing.T) {
	f, avatar := newCommandIDFixture(t)
	stale := avatar
	stale.Incarnation++
	f.Command("bob", castWithID(avatar, 40))
	f.Command("bob", castWithID(stale, 41))
	f.Tick()
	assert.Equal(t, []syncstate.CommandRejected{
		rejected(syncstate.NoAuthority, 40),
		rejected(syncstate.StaleTarget, 41),
	}, f.rejections(t, "bob"))
}

// The memory outlives a debug reset. The first tick after a reset still judges targets by the
// publication from before it, so a retry of an admitted command would otherwise run again.
func TestSyncCommandIDsSurviveAReset(t *testing.T) {
	f, avatar := newCommandIDFixture(t)
	f.Command("alice", castWithID(avatar, 50))
	f.Tick()
	require.Len(t, *f.casts, 1)

	f.World.reset()
	f.Command("alice", castWithID(avatar, 50))
	f.Command("alice", castWithID(avatar, 51)) // shows what the judge does with a new ID here
	f.Tick()
	require.Len(t, *f.casts, 2)
	assert.False(t, (*f.casts)[1].target.Alive(), "51 was admitted against the pre-reset publication")
	assert.Empty(t, f.rejections(t, "alice"), "50 is dropped silently, not run twice")
}

// A CommandID survives the real path without loss in both directions: a client's MessagePack
// payload decoded by the command queue, as SendCommand enqueues it, and the command_rejected
// payload the stream publishes, read by a decoder that knows no Go types. A client encoder may
// write an unsigned value in any width, or a nonnegative value as a signed int64.
func TestSyncCommandIDSurvivesTheWireEncoding(t *testing.T) {
	type form struct {
		name   string
		encode func(*msgpack.Encoder, uint64) error
		ids    []uint64
	}
	forms := []form{
		{"compact", (*msgpack.Encoder).EncodeUint, []uint64{1, 127, 128, 1 << 16, 1<<53 + 1, 1 << 63, math.MaxUint64}},
		{"uint8", func(e *msgpack.Encoder, v uint64) error { return e.EncodeUint8(uint8(v)) }, []uint64{1, 255}},
		{"uint16", func(e *msgpack.Encoder, v uint64) error { return e.EncodeUint16(uint16(v)) }, []uint64{65535}},
		{
			"uint32",
			func(e *msgpack.Encoder, v uint64) error { return e.EncodeUint32(uint32(v)) },
			[]uint64{math.MaxUint32},
		},
		{"uint64", (*msgpack.Encoder).EncodeUint64, []uint64{1, 1<<53 + 1, 1 << 63, math.MaxUint64}},
		{
			"int64",
			func(e *msgpack.Encoder, v uint64) error { return e.EncodeInt64(int64(v)) },
			[]uint64{1, 1<<53 + 1, math.MaxInt64},
		},
	}
	f, avatar := newCommandIDFixture(t)
	stale := avatar
	stale.Incarnation++ // rejected, so the ID comes back
	for _, fm := range forms {
		for _, id := range fm.ids {
			var buf bytes.Buffer
			e := msgpack.NewEncoder(&buf)
			require.NoError(t, e.EncodeMapLen(4))
			require.NoError(t, e.EncodeString("TargetSession"))
			require.NoError(t, e.EncodeString(stale.Session))
			require.NoError(t, e.EncodeString("TargetEntity"))
			require.NoError(t, e.EncodeUint(uint64(stale.Entity)))
			require.NoError(t, e.EncodeString("TargetIncarnation"))
			require.NoError(t, e.EncodeUint(stale.Incarnation))
			require.NoError(t, e.EncodeString("CommandID"))
			require.NoError(t, fm.encode(e, id))
			require.NoError(t, f.commands.Enqueue(t.Context(),
				&iscv1.Command{Name: "access_cast", Address: f.address, Payload: buf.Bytes()},
				command.PlayerSender("bob")))
			f.Tick()

			var got []map[string]any
			for _, q := range f.subs["bob"].queue {
				if evt := q.response.GetEvent(); evt.GetName() == "command_rejected" {
					var payload map[string]any
					require.NoError(t, msgpack.Unmarshal(evt.GetPayload(), &payload))
					got = append(got, payload)
				}
			}
			f.subs["bob"].queue, f.subs["bob"].queuedBytes = nil, 0
			require.Len(t, got, 1, "%s %d", fm.name, id)
			assert.Equal(t, id, got[0]["CommandID"], "%s %d: the rejection names the ID exactly", fm.name, id)
		}
	}
}
