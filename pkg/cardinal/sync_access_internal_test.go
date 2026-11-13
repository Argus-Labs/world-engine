package cardinal

import (
	"bytes"
	"context"
	"testing"
	"time"

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
		w.RegisterComponent[syncLoadout]()
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

// quiet asserts that each player is told nothing: it receives only the heartbeat, which every
// TestWorld step is due.
func (f *accessFixture) quiet(t *testing.T, players ...string) {
	t.Helper()
	for _, p := range players {
		requireHeartbeat(t, syncFrames(t, f.subs[p]), "%s should be told nothing", p)
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

// value is a component as a frame carries it.
func (f *accessFixture) value(t *testing.T, name string, presence uint64, component any) syncstate.Value {
	t.Helper()
	data, err := schema.Serialize(component.(schema.Serializable))
	require.NoError(t, err)
	return syncstate.Value{Component: syncIndex(t, f.World, name), Presence: presence, Data: data}
}

// Each row of the fold table in the design: what a reader held, what the change gives, and
// what the reader is told.
//
//nolint:maintidx // One scenario walks the table row by row.
func TestSyncFoldTable(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob", "carol")
	e := f.Create[syncAvatar]()
	e.Set(syncHealth{HP: 10})
	e.Set(syncLoadout{WeaponID: "wand"})
	e.Set(syncstate.Viewers{Players: players("bob")})
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	ref := refOf(e)
	loadout := syncIndex(t, f.World, "loadout")

	// Spawn: nothing held, a row given: record and access.
	alice, bob := f.frame(t, "alice"), f.frame(t, "bob")
	assert.Equal(t, []syncstate.Record{{Ref: ref, Revision: 1, Presence: 1, Components: []syncstate.Value{
		f.value(t, "health", 1, syncHealth{HP: 10}), f.value(t, "loadout", 1, syncLoadout{WeaponID: "wand"}),
	}}}, alice.Records)
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.Authority}}, alice.Access)
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.View}}, bob.Access)
	assert.Equal(t, alice.Records, bob.Records, "records are shared state; access travels apart")
	f.quiet(t, "carol")

	// Kept view, revision rose, entity presence unchanged: a patch holding only what changed.
	e.Set(syncHealth{HP: 9})
	f.Tick()
	patch := syncstate.Patch{
		Ref: ref, Revision: 2, Changed: []syncstate.Value{f.value(t, "health", 1, syncHealth{HP: 9})},
	}
	for _, p := range []string{"alice", "bob"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.Patch{patch}, fr.Patches, p)
		assert.Empty(t, fr.Records, p)
		assert.Empty(t, fr.Access, p)
	}
	f.quiet(t, "carol")

	// Gained view in the same commit that changed the entity: the whole record and a grant, while
	// the readers that kept view get the patch.
	e.Set(syncHealth{HP: 8})
	e.Set(syncstate.Viewers{Players: players("bob", "carol")})
	f.Tick()
	carol := f.frame(t, "carol")
	assert.Empty(t, carol.Patches)
	assert.Equal(t, []syncstate.Record{{Ref: ref, Revision: 3, Presence: 1, Components: []syncstate.Value{
		f.value(t, "health", 1, syncHealth{HP: 8}), f.value(t, "loadout", 1, syncLoadout{WeaponID: "wand"}),
	}}}, carol.Records)
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.View}}, carol.Access)
	for _, p := range []string{"alice", "bob"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.Patch{{Ref: ref, Revision: 3, Changed: []syncstate.Value{
			f.value(t, "health", 1, syncHealth{HP: 8}),
		}}}, fr.Patches, p)
		assert.Empty(t, fr.Records, p)
		assert.Empty(t, fr.Access, p)
	}

	// A removed component arrives in Dropped.
	e.Remove[syncLoadout]()
	f.Tick()
	for _, p := range []string{"alice", "bob", "carol"} {
		dropped := []syncstate.Patch{{Ref: ref, Revision: 4, Dropped: []uint16{loadout}}}
		assert.Equal(t, dropped, f.frame(t, p).Patches, p)
	}

	// A re-added component arrives in Changed with its new presence.
	e.Set(syncLoadout{WeaponID: "bow"})
	f.Tick()
	readded := f.frame(t, "bob").Patches
	require.Len(t, readded, 1)
	require.Len(t, readded[0].Changed, 1)
	added := readded[0].Changed[0]
	assert.Greater(t, added.Presence, uint64(1), "a new presence")
	assert.Equal(t, []syncstate.Patch{{Ref: ref, Revision: 5, Changed: []syncstate.Value{
		f.value(t, "loadout", added.Presence, syncLoadout{WeaponID: "bow"}),
	}}}, readded)
	for _, p := range []string{"alice", "carol"} {
		assert.Equal(t, readded, f.frame(t, p).Patches, p)
	}

	// Removed and re-added between two publications with equal data: only its presence changed.
	e.Remove[syncLoadout]()
	e.Set(syncLoadout{WeaponID: "bow"})
	f.Tick()
	again := f.frame(t, "bob").Patches
	require.Len(t, again, 1)
	require.Len(t, again[0].Changed, 1)
	assert.Greater(t, again[0].Changed[0].Presence, added.Presence)
	assert.Equal(t, []syncstate.Patch{{Ref: ref, Revision: 6, Changed: []syncstate.Value{
		f.value(t, "loadout", again[0].Changed[0].Presence, syncLoadout{WeaponID: "bow"}),
	}}}, again)
	f.frame(t, "alice")
	f.frame(t, "carol")

	// Kept view, revision rose and promoted: a patch and a grant.
	e.Set(syncHealth{HP: 7})
	e.Set(syncstate.Authorities{Players: players("alice", "bob")})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Len(t, bob.Patches, 1)
	assert.Empty(t, bob.Records)
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.Authority}}, bob.Access)
	assert.Empty(t, f.frame(t, "alice").Access)
	f.frame(t, "carol")
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	f.frame(t, "bob")
	f.quiet(t, "alice", "carol")

	// Entity presence changed (sync removed and re-added): every holder gets the whole record,
	// and no grant, since access did not change.
	e.Remove[syncstate.Sync]()
	e.Set(syncstate.Sync{})
	f.Tick()
	for _, p := range []string{"alice", "bob", "carol"} {
		fr := f.frame(t, p)
		assert.Empty(t, fr.Patches, p)
		assert.Empty(t, fr.Access, p)
		require.Len(t, fr.Records, 1, p)
		assert.Equal(t, uint64(8), fr.Records[0].Revision, p)
		assert.Greater(t, fr.Records[0].Presence, uint64(1), "%s: a new entity presence", p)
		assert.Equal(t, ref, fr.Records[0].Ref, p)
	}

	// Promoted: view to authority gives access only, and does not bump the revision.
	e.Set(syncstate.Authorities{Players: players("alice", "bob")})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Empty(t, bob.Records)
	assert.Empty(t, bob.Patches)
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.Authority}}, bob.Access)
	f.quiet(t, "alice", "carol")

	// Demoted: access only again.
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Empty(t, bob.Patches)
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.View}}, bob.Access)
	f.quiet(t, "alice", "carol")

	// Lost view: an empty Viewers leaves the entity to its authorities; bob is told it is gone.
	e.Set(syncstate.Viewers{})
	f.Tick()
	for _, p := range []string{"bob", "carol"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.Ref{ref}, fr.Removed, p)
		assert.Empty(t, fr.Records, p)
		assert.Empty(t, fr.Patches, p)
	}
	f.quiet(t, "alice")

	// Gained view of an entity it saw before: the full current record and access.
	e.Set(syncstate.Viewers{Players: players("bob")})
	f.Tick()
	bob = f.frame(t, "bob")
	assert.Equal(t, uint64(8), revisionOf(t, bob), "access changes never bump the revision")
	assert.Equal(t, []syncstate.Grant{{Ref: ref, Access: syncstate.View}}, bob.Access)
	f.quiet(t, "alice", "carol")

	// A list rewrite that changes nobody's access sends nothing.
	e.Set(syncstate.Viewers{Players: players("bob"), Groups: players("nobody")})
	f.Tick()
	f.quiet(t, "alice", "bob", "carol")

	// Numeric ID reused within a tick: removed, then record and access, in one frame.
	e.Destroy()
	next := f.Create[syncAvatar]()
	next.Set(syncHealth{HP: 1})
	f.Tick()
	require.Equal(t, e.ID(), next.ID())
	for _, p := range []string{"alice", "bob"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.Ref{ref}, fr.Removed, p)
		assert.Equal(t, []syncstate.Grant{{Ref: refOf(next), Access: syncstate.View}}, fr.Access, p)
		assert.Equal(t, uint64(1), revisionOf(t, fr), p)
		assert.Empty(t, fr.Patches, p)
	}
	assert.Len(t, f.frame(t, "carol").Records, 1)

	// Readmission: an empty ledger takes every current entry, as a baseline on a new stream.
	f.admitted["bob"] = false
	f.Tick()
	revoked := f.frame(t, "bob")
	assert.NotNil(t, revoked.Open)
	assert.Empty(t, revoked.Records)
	f.quiet(t, "alice", "carol")
	f.admitted["bob"] = true
	f.Tick()
	readmitted := f.frame(t, "bob")
	require.NotNil(t, readmitted.Open)
	assert.Equal(t, next.SyncID().Session, readmitted.Open.Session)
	assert.Greater(t, readmitted.Stream, revoked.Stream)
	assert.Equal(t, uint64(1), readmitted.Sequence)
	assert.Len(t, readmitted.Records, 1)
	assert.Empty(t, readmitted.Patches, "a baseline carries only records and grants")
	assert.Empty(t, readmitted.Removed, "a baseline carries only records and grants")
	assert.Equal(t, []syncstate.Grant{{Ref: refOf(next), Access: syncstate.View}}, readmitted.Access)
	f.quiet(t, "alice", "carol")
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
	assert.Equal(t, []syncstate.Ref{refOf(e)}, f.frame(t, "bob").Removed)
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
		{Ref: refOf(e), Access: syncstate.View}, {Ref: refOf(other), Access: syncstate.Authority},
	}, alice.Access)
	f.quiet(t, "bob")

	// Renaming the group empties the old name.
	team.Set(syncstate.Group{GroupName: "renamed", Players: players("alice", "bob")})
	f.Tick()
	for _, p := range []string{"alice", "bob"} {
		fr := f.frame(t, p)
		assert.Equal(t, []syncstate.Ref{refOf(e)}, fr.Removed, p)
		assert.Equal(t, []syncstate.Grant{{Ref: refOf(other), Access: syncstate.View}}, fr.Access, p)
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
	assert.NotNil(t, cleared.Open)
	assert.Empty(t, cleared.Records, "alice never sees bob's entity")
	joined := f.frame(t, "bob")
	assert.NotNil(t, joined.Open)
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

// The frame layout, byte for byte, against the compact wire spec: positional arrays, the smallest
// integers, the session and heartbeat once per stream, the tick's time, component indexes, and the
// fingerprint only on the baseline. Records and patches are encoded once and carried by every
// reader's frame. A heartbeat is an empty delta.
func TestSyncFrameLayoutGolden(t *testing.T) {
	f := newAccessFixture(t, "alice", "bob")
	e := f.Create[syncAvatar]()
	e.Set(syncHealth{HP: 7, MaxHP: 9})
	e.Set(syncLoadout{WeaponID: "wand"})
	e.Set(syncstate.Authorities{Players: players("alice")})
	f.Tick()
	syncFrames(t, f.subs["alice"])
	syncFrames(t, f.subs["bob"])
	f.connect(t, "dave")
	f.admitted["dave"] = true
	f.Tick()
	require.Equal(t, time.Unix(2, 0), f.Timestamp(), "dave's baseline is at 2,000 ms")
	require.Less(t, uint32(e.ID()), uint32(128))
	require.Less(t, e.SyncID().Incarnation, uint64(128))
	ref := []byte{0x92, byte(e.ID()), byte(e.SyncID().Incarnation)}
	bin := func(t *testing.T, component any) []byte {
		t.Helper()
		data, err := schema.Serialize(component.(schema.Serializable))
		require.NoError(t, err)
		require.Less(t, len(data), 256)
		return append([]byte{0xc4, byte(len(data))}, data...)
	}
	str := func(s string) []byte { return append([]byte{0xd9, byte(len(s))}, s...) }
	cat := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	fingerprint, err := f.SyncContract().Fingerprint()
	require.NoError(t, err)
	health, loadout := syncIndex(t, f.World, "health"), syncIndex(t, f.World, "loadout")
	require.Equal(t, []uint16{0, 1}, []uint16{health, loadout}, "contract order: health, loadout")

	second := []byte{0xcd, 0x03, 0xe8} // 1,000 ms: the heartbeat, and the time between TestWorld steps

	// alice and bob heard nothing for a heartbeat: an empty delta, one second on.
	for player, stream := range map[string]byte{"alice": 1, "bob": 2} {
		heartbeat := cat([]byte{0x98, 0xc0, stream, 0x03}, second, []byte{0xc0, 0xc0, 0xc0, 0xc0})
		require.Len(t, f.subs[player].queue, 1, player)
		assert.Equal(t, heartbeat, f.subs[player].queue[0].response.GetEvent().GetPayload(), player)
		syncFrames(t, f.subs[player])
	}

	// dave's baseline: [Open, Stream, Sequence, Time, Records, Patches, Removed, Access]
	want := cat(
		[]byte{0x98},
		[]byte{0x94, 0x02}, str(fingerprint), str(e.SyncID().Session), second, // Open, with the heartbeat
		[]byte{0x03, 0x01},                                // stream 3 (alice 1, bob 2), sequence 1
		[]byte{0xcd, 0x07, 0xd0},                          // time: 2,000 ms since the Unix epoch
		[]byte{0x91, 0x94}, ref, []byte{0x01, 0x01, 0x92}, // one record: ref, revision 1, presence 1, two values
		[]byte{0x93, 0x00, 0x01}, bin(t, syncHealth{HP: 7, MaxHP: 9}), // health: index 0, presence 1
		[]byte{0x93, 0x01, 0x01}, bin(t, syncLoadout{WeaponID: "wand"}), // loadout: index 1, presence 1
		[]byte{0xc0, 0xc0},                    // no patches, no removals
		[]byte{0x91, 0x92}, ref, []byte{0x01}, // one grant: ref, view
	)
	baseline := f.subs["dave"].queue
	require.Len(t, baseline, 1)
	assert.Equal(t, want, baseline[0].response.GetEvent().GetPayload())
	syncFrames(t, f.subs["dave"])

	// One entity with two components moves one: a delta holds a patch with that one component.
	e.Set(syncLoadout{WeaponID: "bow", Ammo: 3})
	f.Tick()
	moved := bin(t, syncLoadout{WeaponID: "bow", Ammo: 3})
	// a patch: ref, revision 2, one changed value (loadout, presence 1), nothing dropped
	patch := cat([]byte{0x94}, ref, []byte{0x02, 0x91, 0x93, 0x01, 0x01}, moved, []byte{0xc0})
	for player, stream := range map[string]byte{"alice": 1, "bob": 2, "dave": 3} {
		sequence := byte(4)
		if player == "dave" {
			sequence = 2
		}
		delta := cat([]byte{0x98, 0xc0, stream, sequence}, second, []byte{0xc0, 0x91}, patch, []byte{0xc0, 0xc0})
		require.Len(t, f.subs[player].queue, 1, player)
		wire := f.subs[player].queue[0].response.GetEvent().GetPayload()
		assert.Equal(t, delta, wire, player)
		t.Logf("%s's delta: %d bytes, of which %d are the changed component's data", player, len(wire), len(moved)-2)
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
	assert.Equal(t, []syncstate.Grant{{Ref: refOf(blue), Access: syncstate.View}}, fr.Access)
}
