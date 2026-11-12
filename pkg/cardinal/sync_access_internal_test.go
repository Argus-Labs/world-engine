package cardinal

import (
	"bytes"
	"context"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/micro"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type accessCast struct {
	syncstate.Target

	Spell string
}

func (accessCast) Name() string { return "access_cast" }

type accessRelay struct{ Note string }

func (accessRelay) Name() string { return "access_relay" }

type accessSpawn struct {
	syncstate.Event

	Entity uint32
}

func (accessSpawn) Name() string         { return "access_spawn" }
func (e accessSpawn) SyncEntity() uint32 { return e.Entity }

type accessHit struct {
	syncstate.Event
	syncstate.Occurrence

	Entity uint32
}

func (accessHit) Name() string         { return "access_hit" }
func (e accessHit) SyncEntity() uint32 { return e.Entity }

type accessTeam struct{ Group syncstate.Group }

type accessPlain struct{ Health syncHealth } // an entity that is never synced

type accessCastSeen struct {
	sender string
	target Entity
}

type accessCastHandler struct{ seen *[]accessCastSeen }

func (h accessCastHandler) Run(w *World) {
	for cmd := range w.Commands[accessCast]() {
		*h.seen = append(*h.seen, accessCastSeen{cmd.Sender(), cmd.Target()})
	}
}

// accessDuring runs a test's function inside the tick, after the commands are drained and
// before the tick's final capture.
type accessDuring struct{ fn *func(*World) }

func (d accessDuring) Run(w *World) {
	if fn := *d.fn; fn != nil {
		*d.fn = nil
		fn(w)
	}
}

type accessFixture struct {
	*TestWorld

	admitted map[string]bool
	subs     map[string]*streamSubscriber
	casts    *[]accessCastSeen
	hook     *func(*World)
}

// during runs fn inside the next tick, then finishes the tick.
func (f *accessFixture) during(fn func(w *World)) {
	*f.hook = fn
	f.Tick()
}

// newAccessFixture opens a sync stream for each player. Each is admitted, then the first tick
// gives each an empty baseline, which the fixture discards.
func newAccessFixture(t *testing.T, players ...string) *accessFixture {
	t.Helper()
	f := &accessFixture{
		admitted: map[string]bool{}, subs: map[string]*streamSubscriber{}, casts: &[]accessCastSeen{},
		hook: new(func(*World)),
	}
	f.TestWorld = NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncHealth]()
		w.RegisterComponent[syncPrivate]()
		w.EnableSync(func(player string) bool { return f.admitted[player] })
		w.RegisterCommand[accessCast](syncstate.FromPlayers)
		w.RegisterCommand[accessRelay](syncstate.FromShards)
		w.RegisterEvent[accessSpawn]()
		w.RegisterEvent[accessHit]()
		w.RegisterSystem(accessCastHandler{f.casts})
		w.RegisterSystem(accessDuring{f.hook})
	})
	f.World.events.RegisterHandler(event.KindDefault, f.service.publishDefaultEvent)
	for _, p := range players {
		f.admitted[p] = true
		f.connect(t, p)
	}
	f.Tick()
	for _, p := range players {
		require.Len(t, syncFrames(t, f.subs[p]), 1)
	}
	return f
}

func (f *accessFixture) connect(t *testing.T, player string) {
	t.Helper()
	ctx, cancel := context.WithCancelCause(t.Context())
	sub := &streamSubscriber{
		ctx:        ctx,
		cancel:     cancel,
		events:     map[string]struct{}{},
		queueReady: make(chan struct{}, 1),
	}
	addSubscriptions(sub, []*cardinalv1.EventSubscription{{Events: []string{
		syncstate.EventName, "access_spawn", "access_hit", "command_rejected",
	}}})
	f.service.addSubscriber(player, sub)
	f.subs[player] = sub
}

// frame returns the one sync frame player received, or fails.
func (f *accessFixture) frame(t *testing.T, player string) syncstate.Frame {
	t.Helper()
	frames := syncFrames(t, f.subs[player])
	require.Len(t, frames, 1, "%s's sync frames", player)
	return frames[0]
}

func (f *accessFixture) quiet(t *testing.T, players ...string) {
	t.Helper()
	for _, p := range players {
		require.Empty(t, syncFrames(t, f.subs[p]), "%s should receive no sync frame", p)
	}
}

// received returns the names of the non-sync events player received, and clears its queue.
func (f *accessFixture) received(player string) []string {
	var names []string
	for _, q := range f.subs[player].queue {
		if name := q.response.GetEvent().GetName(); name != syncstate.EventName {
			names = append(names, name)
		}
	}
	f.subs[player].queue, f.subs[player].queuedBytes = nil, 0
	return names
}

func players(names ...string) immutable.Slice[string] { return immutable.SliceOf(names...) }

func revisionOf(t *testing.T, f syncstate.Frame) uint64 {
	t.Helper()
	require.Len(t, f.Records, 1)
	return f.Records[0].Revision
}

// Each row of the fold table in the design: what a reader held, what the change gives, and
// what the reader is told.
func TestSyncFoldTable(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")
	e := f.Create[syncAvatar]()
	e.Set(syncHealth{HP: 10})
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	id := e.SyncID()

	// Spawn: nothing held, a row given: record and access.
	alice, bob := f.frame(t, "alice"), f.frame(t, "bob")
	assert.Len(t, alice.Records, 1)
	assert.Equal(t, []syncstate.Grant{{ID: id, Access: syncstate.Authority}}, alice.Access)
	assert.Equal(t, []syncstate.Grant{{ID: id, Access: syncstate.View}}, bob.Access)
	assert.Equal(t, alice.Records, bob.Records, "records are shared state; access travels apart")

	// Update: a higher revision gives the record only.
	e.Set(syncHealth{HP: 9})
	f.Tick()
	alice = f.frame(t, "alice")
	assert.Equal(t, uint64(2), revisionOf(t, alice))
	assert.Empty(t, alice.Access)
	assert.Empty(t, f.frame(t, "bob").Access)

	// Promoted: view to authority gives access only, and does not bump the revision.
	e.Set(syncstate.Authorities{Players: players("alice", "bob")})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Empty(t, bob.Records)
	assert.Equal(t, []syncstate.Grant{{ID: id, Access: syncstate.Authority}}, bob.Access)
	f.quiet(t, "alice")

	// Demoted: access only again.
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	assert.Equal(t, []syncstate.Grant{{ID: id, Access: syncstate.View}}, f.frame(t, "bob").Access)
	f.quiet(t, "alice")

	// Lost view: an empty Viewers leaves the entity to its authorities; bob is told it is gone.
	e.Set(syncstate.Viewers{})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Equal(t, []syncstate.ID{id}, bob.Removed)
	assert.Empty(t, bob.Records)
	f.quiet(t, "alice")

	// Gained view of an entity it saw before: the full current record and access.
	e.Set(syncstate.Viewers{Players: players("bob")})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Equal(t, uint64(2), revisionOf(t, bob), "access changes never bump the revision")
	assert.Equal(t, []syncstate.Grant{{ID: id, Access: syncstate.View}}, bob.Access)
	f.quiet(t, "alice")

	// A list rewrite that changes nobody's access sends nothing.
	e.Set(syncstate.Viewers{Players: players("bob"), Groups: players("nobody")})
	f.Tick()
	f.quiet(t, "alice", "bob")

	// Numeric ID reused within a tick: removed, then record and access, in one frame.
	e.Destroy()
	next := f.Create[syncAvatar]()
	next.Set(syncHealth{HP: 1})
	f.Tick()
	require.Equal(t, e.ID(), next.ID())
	for _, p := range []string{"alice", "bob"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.ID{id}, fr.Removed, p)
		assert.Equal(t, []syncstate.Grant{{ID: next.SyncID(), Access: syncstate.View}}, fr.Access, p)
		assert.Equal(t, uint64(1), revisionOf(t, fr), p)
	}

	// Readmission: an empty ledger takes every current entry, flagged as a baseline.
	f.admitted["bob"] = false
	f.Tick()
	revoked := f.frame(t, "bob")
	assert.True(t, revoked.Baseline)
	assert.Empty(t, revoked.Records)
	f.admitted["bob"] = true
	f.Tick()
	readmitted := f.frame(t, "bob")
	assert.True(t, readmitted.Baseline)
	assert.NotEqual(t, revoked.Stream, readmitted.Stream)
	assert.Equal(t, uint64(1), readmitted.Sequence)
	assert.Equal(t, []syncstate.Grant{{ID: next.SyncID(), Access: syncstate.View}}, readmitted.Access)
	f.quiet(t, "alice")
}

func TestSyncCommandVerdicts(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob", "dave")
	avatar := f.Create[syncAvatar]()
	avatar.Set(syncstate.Authorities{Players: players("alice")})
	stash := f.Create[syncAvatar]()
	stash.Set(syncstate.Viewers{})
	stash.Set(syncstate.Authorities{Players: players("carol")})
	gone := f.Create[syncAvatar]()
	daves := f.Create[syncAvatar]()
	daves.Set(syncstate.Authorities{Players: players("dave")})
	f.Tick()
	goneID := gone.SyncID()
	gone.Destroy()
	f.admitted["dave"] = false
	f.Tick()
	syncFrames(t, f.subs["alice"])
	syncFrames(t, f.subs["bob"])
	syncFrames(t, f.subs["dave"])

	stale := avatar.SyncID()
	stale.Incarnation++
	f.Command("alice", accessCast{Target: syncstate.TargetOf(avatar.SyncID())})
	f.Command("bob", accessCast{Target: syncstate.TargetOf(avatar.SyncID())})
	f.Command("bob", accessCast{Target: syncstate.TargetOf(stash.SyncID())})
	f.Command("alice", accessCast{Target: syncstate.TargetOf(stale)})
	f.Command("alice", accessCast{Target: syncstate.TargetOf(goneID)})
	f.Command("dave", accessCast{Target: syncstate.TargetOf(daves.SyncID())})
	f.Command("bob", accessRelay{})
	payload, err := schema.Serialize(accessCast{Target: syncstate.TargetOf(avatar.SyncID())})
	require.NoError(t, err)
	require.NoError(t, f.commands.Enqueue(t.Context(),
		&iscv1.Command{Name: "access_cast", Address: f.address, Payload: payload},
		command.ShardSender(micro.GetAddress("other", "shard"))))
	f.Tick()

	require.Len(t, *f.casts, 1, "only the command with authority reaches the handler")
	assert.Equal(t, "alice", (*f.casts)[0].sender)
	assert.Equal(t, avatar.ID(), (*f.casts)[0].target.ID())
	assert.True(t, (*f.casts)[0].target.Alive())

	got := map[string][]syncstate.Reason{}
	for _, p := range []string{"alice", "bob", "dave"} {
		for _, q := range f.subs[p].queue {
			if q.response.GetEvent().GetName() != "command_rejected" {
				continue
			}
			var r syncstate.CommandRejected
			require.NoError(t, msgpack.Unmarshal(q.response.GetEvent().GetPayload(), &r))
			got[p] = append(got[p], r.Reason)
		}
	}
	assert.Equal(
		t,
		[]syncstate.Reason{syncstate.StaleTarget, syncstate.StaleTarget},
		got["alice"],
		"another lifetime; gone",
	)
	assert.Equal(t, []syncstate.Reason{syncstate.NoAuthority, syncstate.StaleTarget, syncstate.ShardOnly}, got["bob"],
		"views but no authority; cannot view, so existence is not revealed; shard-only command")
	assert.Equal(t, []syncstate.Reason{syncstate.StaleTarget}, got["dave"], "authority never bypasses admission")
}

// A command is judged by what the server last published, not by the live world.
func TestSyncCommandsAreJudgedByPublishedEntries(t *testing.T) {
	f := newAccessFixture(t, "bob")
	avatar := f.Create[syncAvatar]()
	f.Tick()
	syncFrames(t, f.subs["bob"])
	avatar.Set(syncstate.Authorities{Players: players("bob")}) // live, not yet published
	f.Command("bob", accessCast{Target: syncstate.TargetOf(avatar.SyncID())})
	f.Tick()
	assert.Empty(t, *f.casts)
	assert.Equal(t, []string{"command_rejected"}, f.received("bob"))
	f.Command("bob", accessCast{Target: syncstate.TargetOf(avatar.SyncID())})
	f.Tick()
	assert.Len(t, *f.casts, 1)
}

func authorityIDs(w *World, player string) []EntityID {
	var out []EntityID
	for e := range w.AuthorityOf(player) {
		out = append(out, e.ID())
	}
	return out
}

func TestSyncAuthorityOf(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")
	team := f.Create[accessTeam]()
	team.Set(syncstate.Group{GroupName: "red", Players: players("alice", "bob")})
	mine := f.Create[syncAvatar]()
	mine.Set(syncstate.Authorities{Players: players("alice")})
	turret := f.Create[syncAvatar]()
	turret.Set(syncstate.Authorities{Groups: players("red")})
	f.Create[syncAvatar]() // nobody can target it
	plain := f.Create[accessPlain]()
	plain.Set(syncstate.Authorities{Players: players("alice")}) // not synced: never yielded
	f.Tick()
	assert.Equal(t, []EntityID{mine.ID(), turret.ID()}, authorityIDs(f.World, "alice"))
	assert.Equal(t, []EntityID{turret.ID()}, authorityIDs(f.World, "bob"))
	assert.Empty(t, authorityIDs(f.World, "carol"))
	f.admitted["alice"] = false
	assert.Empty(t, authorityIDs(f.World, "alice"), "authority never bypasses admission")
	f.admitted["alice"] = true

	// An entity destroyed while the caller consumes the sequence is skipped.
	var seen []EntityID
	for e := range f.AuthorityOf("alice") {
		seen = append(seen, e.ID())
		turret.Destroy()
	}
	assert.Equal(t, []EntityID{mine.ID()}, seen)

	// Ascending entity IDs, whichever archetype holds each entity: the lower ID lands in an
	// archetype created later.
	low, high := f.Create[syncAvatar](), f.Create[syncAvatar]()
	require.Less(t, low.ID(), high.ID())
	high.Set(syncstate.Authorities{Players: players("bob")})
	low.Set(syncstate.Viewers{Players: players("bob")})
	low.Set(syncstate.Authorities{Players: players("bob")})
	assert.Equal(t, []EntityID{low.ID(), high.ID()}, authorityIDs(f.World, "bob"))
}

// AuthorityOf answers from the world as written, inside the tick that wrote it, before any
// capture publishes the change.
func TestSyncAuthorityOfReadsTheLiveWorld(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")
	published := func(w *World, e Entity) bool {
		entry := w.sync.entry(e.ID())
		return entry != nil && entry.id == e.SyncID()
	}

	var avatar, team, dup, turret Entity
	f.during(func(w *World) { // spawn
		avatar = w.Create[syncAvatar]()
		avatar.Set(syncstate.Authorities{Players: players("alice")})
		assert.False(t, published(w, avatar))
		assert.Equal(t, []EntityID{avatar.ID()}, authorityIDs(w, "alice"), "same-tick spawn")
		assert.Empty(t, authorityIDs(w, "bob"))
	})
	f.during(func(w *World) { // authority removed, then a new holder destroyed
		avatar.Remove[syncstate.Authorities]()
		assert.True(t, published(w, avatar))
		assert.Empty(t, authorityIDs(w, "alice"), "same-tick authority removal")
		avatar.Set(syncstate.Authorities{Players: players("bob")})
		assert.Equal(t, []EntityID{avatar.ID()}, authorityIDs(w, "bob"))
		avatar.Destroy()
		assert.Empty(t, authorityIDs(w, "bob"), "same-tick destroy")
	})
	f.during(func(w *World) { // a group created and changed
		turret = w.Create[syncAvatar]()
		turret.Set(syncstate.Authorities{Groups: players("red", "blue")}) // blue never exists
		assert.Empty(t, authorityIDs(w, "alice"), "unknown group names have no members")
		team = w.Create[accessTeam]()
		team.Set(syncstate.Group{GroupName: "red", Players: players("alice")})
		assert.Equal(t, []EntityID{turret.ID()}, authorityIDs(w, "alice"), "same-tick group creation")
		assert.Empty(t, authorityIDs(w, "bob"))
	})
	f.during(func(w *World) {
		team.Set(syncstate.Group{GroupName: "red", Players: players("bob")})
		assert.Empty(t, authorityIDs(w, "alice"), "same-tick membership change")
		assert.Equal(t, []EntityID{turret.ID()}, authorityIDs(w, "bob"))

		dup = w.Create[accessTeam]()
		dup.Set(syncstate.Group{GroupName: "red", Players: players("bob")})
		assert.Empty(t, authorityIDs(w, "bob"), "a duplicate name has no members")
		dup.Destroy()
		assert.Equal(t, []EntityID{turret.ID()}, authorityIDs(w, "bob"))

		f.admitted["bob"] = false
		assert.Empty(t, authorityIDs(w, "bob"), "authority never bypasses admission")
		f.admitted["bob"] = true

		team.Destroy()
		assert.Empty(t, authorityIDs(w, "bob"), "same-tick group deletion")
	})
}

func TestSyncAuthorityOfNeedsSync(t *testing.T) {
	w := NewTestWorld(t, func(w *World) {
		w.RegisterComponent[syncHealth]()
	})
	assert.Empty(t, authorityIDs(w.World, "alice"))
}

func TestSyncGroupsFailClosed(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")
	var logs bytes.Buffer
	f.tel.Logger = zerolog.New(&logs)
	e := f.Create[syncAvatar]()
	e.Set(syncstate.Viewers{Groups: players("team")})
	f.Tick()
	f.quiet(t, "alice", "bob") // an unknown name has no members

	team := f.Create[accessTeam]()
	team.Set(syncstate.Group{GroupName: "team", Players: players("bob")})
	f.Tick()
	assert.Len(t, f.frame(t, "bob").Records, 1)
	f.quiet(t, "alice")

	// Two groups with one name: the name has no members, and the engine logs an error.
	dup := f.Create[accessTeam]()
	dup.Set(syncstate.Group{GroupName: "team", Players: players("alice")})
	f.Tick()
	assert.Equal(t, []syncstate.ID{e.SyncID()}, f.frame(t, "bob").Removed)
	f.quiet(t, "alice")
	assert.Contains(t, logs.String(), "not unique")

	dup.Destroy()
	f.Tick()
	assert.Len(t, f.frame(t, "bob").Records, 1)

	// One membership change applies to every entity that names the group.
	other := f.Create[syncAvatar]()
	other.Set(syncstate.Authorities{Groups: players("team")})
	f.Tick()
	syncFrames(t, f.subs["alice"])
	syncFrames(t, f.subs["bob"])
	team.Set(syncstate.Group{GroupName: "team", Players: players("alice", "bob")})
	f.Tick()
	alice := f.frame(t, "alice")
	assert.Len(t, alice.Records, 1, "alice gains view of the entity that names the group")
	assert.ElementsMatch(t, []syncstate.Grant{
		{ID: e.SyncID(), Access: syncstate.View}, {ID: other.SyncID(), Access: syncstate.Authority},
	}, alice.Access)
	f.quiet(t, "bob")

	// Renaming the group empties the old name.
	team.Set(syncstate.Group{GroupName: "renamed", Players: players("alice", "bob")})
	f.Tick()
	for _, p := range []string{"alice", "bob"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.ID{e.SyncID()}, fr.Removed, p)
		assert.Equal(t, []syncstate.Grant{{ID: other.SyncID(), Access: syncstate.View}}, fr.Access, p)
	}

	// Group members never reach the wire, even on a synced entity.
	team.Set(syncstate.Sync{})
	f.Tick()
	record := f.frame(t, "bob").Records
	require.Len(t, record, 1)
	assert.Empty(t, record[0].Components)
	contract, err := f.SyncContract().CanonicalJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(contract), "sync_group")
}

// A slot freed during a tick is not reused until that tick's log is dispatched: the reader who
// joins in the same tick must not inherit the leaving reader's slot while transitions for the
// leaving reader's stream are still queued.
func TestSyncSlotCoolsUntilDispatch(t *testing.T) {
	f := newAccessFixture(t, "alice")
	secret := f.Create[syncAvatar]()
	secret.Set(syncstate.Viewers{Players: players("bob")})
	f.Tick()
	f.quiet(t, "alice")

	secret.Set(syncHealth{HP: 1}) // a transition queued in the tick alice leaves and bob joins
	f.admitted["alice"] = false
	f.admitted["bob"] = true
	f.connect(t, "bob")
	f.Tick()
	cleared := f.frame(t, "alice")
	assert.True(t, cleared.Baseline)
	assert.Empty(t, cleared.Records, "alice never sees bob's entity")
	joined := f.frame(t, "bob")
	assert.True(t, joined.Baseline)
	assert.Len(t, joined.Records, 1)
	assert.Equal(t, 1, f.sync.roster.slotOf["bob"], "bob took a new slot while alice's cooled")
	f.Tick()
	f.connect(t, "carol")
	f.admitted["carol"] = true
	f.Tick()
	assert.Equal(t, 0, f.sync.roster.slotOf["carol"], "the cooled slot is reused after dispatch")
}

func TestSyncEventsReachOnlyViewers(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")

	// Spawn code that broadcasts before it narrows Viewers is judged at the end of the tick.
	e := f.Create[syncAvatar]()
	f.Broadcast(accessSpawn{Entity: uint32(e.ID())})
	e.Set(syncstate.Viewers{Players: players("alice")})
	f.Tick()
	assert.Equal(t, []string{"access_spawn"}, f.received("alice"))
	assert.Empty(t, f.received("bob"))

	// An occurrence reaches only open readers who can view its entity.
	f.Broadcast(accessHit{Entity: uint32(e.ID())})
	f.Tick()
	assert.Equal(t, []string{"access_hit"}, f.received("alice"))
	assert.Empty(t, f.received("bob"))

	// An event sent after Destroy is judged by the lifetime's last published entry, even after
	// another event has published the removal.
	id := e.SyncID()
	e.Destroy()
	f.Broadcast(accessSpawn{Event: syncstate.For(id), Entity: id.Entity})
	f.Broadcast(accessHit{Event: syncstate.For(id), Entity: id.Entity})
	f.Tick()
	assert.Equal(t, []string{"access_spawn", "access_hit"}, f.received("alice"))
	assert.Empty(t, f.received("bob"))

	// An event about an entity that is not synced is not gated by access.
	plain := f.Create[accessPlain]()
	f.Broadcast(accessSpawn{Entity: uint32(plain.ID())})
	f.Broadcast(accessHit{Entity: uint32(plain.ID())})
	f.Tick()
	assert.Equal(t, []string{"access_spawn", "access_hit"}, f.received("alice"))
	assert.Equal(t, []string{"access_spawn", "access_hit"}, f.received("bob"))

	// An occurrence needs an open sync stream, even about a public entity.
	public := f.Create[syncAvatar]()
	f.Tick()
	f.service.mu.Lock()
	removeSubscriptions(f.subs["bob"], []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}})
	f.service.mu.Unlock()
	f.Tick() // bob's sync stream closes
	f.received("alice")
	f.received("bob")
	f.Broadcast(accessHit{Entity: uint32(public.ID())})
	f.Broadcast(accessSpawn{Entity: uint32(public.ID())})
	f.Tick()
	assert.Equal(t, []string{"access_spawn"}, f.received("bob"))
}

// Each entry's record is encoded once, and every reader's frame carries those bytes: the frame
// on the wire equals the struct encoding of the same frame built from the world.
func TestSyncFramesAreByteIdenticalToStructEncoding(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")
	e := f.Create[syncAvatar]()
	e.Set(syncHealth{HP: 7, MaxHP: 9})
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	health, err := schema.Serialize(syncHealth{HP: 7, MaxHP: 9})
	require.NoError(t, err)
	record := syncstate.Record{ID: e.SyncID(), Revision: 1, Presence: 1, Components: []syncstate.Value{
		{Name: "health", Presence: 1, Data: health},
	}}
	fingerprint, err := f.SyncContract().Fingerprint()
	require.NoError(t, err)
	for player, access := range map[string]syncstate.Access{"alice": syncstate.Authority, "bob": syncstate.View} {
		require.Len(t, f.subs[player].queue, 1)
		wire := f.subs[player].queue[0].response.GetEvent().GetPayload()
		var decoded syncstate.Frame
		require.NoError(t, msgpack.Unmarshal(wire, &decoded))
		want, err := schema.Serialize(syncstate.Frame{
			Version: syncstate.ProtocolVersion, Fingerprint: fingerprint, Stream: decoded.Stream, Sequence: 2,
			Records: []syncstate.Record{record}, Access: []syncstate.Grant{{ID: e.SyncID(), Access: access}},
		})
		require.NoError(t, err)
		assert.Equal(t, want, wire, player)
		assert.True(
			t,
			bytes.Contains(wire, f.sync.entry(e.ID()).record),
			"%s's frame carries the record encoded once",
			player,
		)
	}
}

// Entities whose lists name different groups that do not exist yet share no policy: creating one
// of the groups changes access only on the entity that names it.
func TestSyncPolicyTellsUnknownGroupsApart(t *testing.T) {
	f := newAccessFixture(t, "alice")
	red := f.Create[syncAvatar]()
	red.Set(syncstate.Viewers{Groups: players("red")})
	blue := f.Create[syncAvatar]()
	blue.Set(syncstate.Viewers{Groups: players("blue")})
	f.Tick()
	f.quiet(t, "alice")
	team := f.Create[accessTeam]()
	team.Set(syncstate.Group{GroupName: "blue", Players: players("alice")})
	f.Tick()
	fr := f.frame(t, "alice")
	assert.Equal(t, []syncstate.Grant{{ID: blue.SyncID(), Access: syncstate.View}}, fr.Access)
}
