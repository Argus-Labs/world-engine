package cardinal

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type syncLoadout struct {
	syncstate.State
	WeaponID string
	Ammo     uint16
	Equipped bool
}

func (syncLoadout) Name() string { return "loadout" }

type syncCosmetics struct {
	syncstate.State
	Tint    uint32
	Opacity float32
}

func (syncCosmetics) Name() string { return "cosmetics" }

type syncEquipment struct {
	syncstate.Sync
	Loadout   syncLoadout
	Cosmetics syncCosmetics
	Private   syncPrivate
}

// Uses no Health registration. Its first eight frames cover every part of the compact layout,
// in order, the last an empty delta; SYNC_INTEROP_OUTPUT writes them, with the contract, for
// Unity's interop test. It ticks at 20 Hz from a real Unix time, so each Time has the width a
// server sends: the baseline's 8 bytes, a delta's 50 ms in one, the empty delta's 1,000 ms in two.
//
//nolint:maintidx // One scenario, frame by frame, as the fixture lists them.
func TestSyncMultipleUnrelatedComponents(t *testing.T) {
	register := func(w *World) {
		w.RegisterComponent[syncLoadout]()
		w.RegisterComponent[syncCosmetics]()
		w.RegisterComponent[syncPrivate]()
	}
	contract, err := ExportSyncContract(register)
	require.NoError(t, err)
	require.JSONEq(t, `{"version":2,"components":[{"name":"cosmetics","fields":[{"name":"Opacity","type":"float32"},{"name":"Tint","type":"uint32"}]},{"name":"loadout","fields":[{"name":"Ammo","type":"uint16"},{"name":"Equipped","type":"bool"},{"name":"WeaponID","type":"string"}]}]}`, string(contract))
	w := NewTestWorld(t, func(w *World) { register(w); w.EnableSync(func(string) bool { return true }) })
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	sub := &streamSubscriber{ctx: ctx, cancel: cancel, events: map[string]struct{}{syncstate.EventName: {}}, queueReady: make(chan struct{}, 1)}
	w.service.addSubscriber("observer", sub)
	client := newSyncClient(w.SyncContract())
	cosmetics, loadout := syncIndex(t, w.World, "cosmetics"), syncIndex(t, w.World, "loadout")
	require.Equal(t, []uint16{0, 1}, []uint16{cosmetics, loadout})
	var raw [][]byte
	// next takes the one frame the tick sent, applies it by the client rules, and keeps its bytes.
	next := func() syncstate.Frame {
		t.Helper()
		require.Len(t, sub.queue, 1)
		raw = append(raw, append([]byte(nil), sub.queue[0].response.Event.Payload...))
		frames := syncFrames(t, sub)
		applied, err := client.apply(frames[0])
		require.NoError(t, err)
		require.True(t, applied)
		return frames[0]
	}
	data := func(component any) []byte {
		t.Helper()
		b, err := schema.Serialize(component.(schema.Serializable))
		require.NoError(t, err)
		return b
	}
	clock := time.UnixMilli(1_762_950_000_000)
	tick := func(after time.Duration) {
		clock = clock.Add(after)
		w.World.Tick(clock)
	}
	const step = 50 * time.Millisecond
	held := func(e Entity) syncstate.Record {
		t.Helper()
		h, ok := client.held[uint32(e.ID())]
		require.True(t, ok)
		return h
	}

	a := w.Create[syncEquipment]()
	a.Set(syncLoadout{WeaponID: "wand"})
	a.Set(syncCosmetics{Tint: ^uint32(0), Opacity: 0.5})
	a.Set(syncPrivate{Secret: "server only"})
	b := w.Create[syncEquipment]()
	b.Remove[syncCosmetics]()
	b.Set(syncLoadout{WeaponID: "axe", Ammo: 2})
	b.Set(syncstate.Authorities{Players: players("observer")})
	tick(step)

	// 1. A baseline with two entities and their grants; the private component is omitted.
	base := next()
	require.NotNil(t, base.Open)
	require.Equal(t, a.SyncID().Session, base.Open.Session)
	require.Equal(t, []uint64{1, 1}, []uint64{base.Stream, base.Sequence})
	require.Equal(t, uint64(1000), base.Open.Heartbeat)
	require.Equal(t, uint64(1_762_950_000_050), base.Time, "a baseline's Time is its tick's Unix ms")
	require.Equal(t, []syncstate.Record{
		{Ref: refOf(a), Revision: 1, Presence: 1, Components: []syncstate.Value{
			{Component: cosmetics, Presence: 1, Data: data(syncCosmetics{Tint: ^uint32(0), Opacity: 0.5})},
			{Component: loadout, Presence: 1, Data: data(syncLoadout{WeaponID: "wand"})},
		}},
		{Ref: refOf(b), Revision: 1, Presence: 1, Components: []syncstate.Value{
			{Component: loadout, Presence: 1, Data: data(syncLoadout{WeaponID: "axe", Ammo: 2})},
		}},
	}, base.Records)
	require.Equal(t, []syncstate.Grant{
		{Ref: refOf(a), Access: syncstate.View}, {Ref: refOf(b), Access: syncstate.Authority},
	}, base.Access)

	// 2. A patch that changes one component.
	a.Set(syncLoadout{WeaponID: "sword", Ammo: 7, Equipped: true})
	tick(step)
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 2, Time: 50, Patches: []syncstate.Patch{{
		Ref: refOf(a), Revision: 2, Changed: []syncstate.Value{
			{Component: loadout, Presence: 1, Data: data(syncLoadout{WeaponID: "sword", Ammo: 7, Equipped: true})},
		},
	}}}, next())
	require.Len(t, held(a).Components, 2, "the patch kept cosmetics")

	// 3. A patch that drops a component.
	a.Remove[syncCosmetics]()
	tick(step)
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 3, Time: 50, Patches: []syncstate.Patch{{
		Ref: refOf(a), Revision: 3, Dropped: []uint16{cosmetics},
	}}}, next())
	require.Equal(t, []uint16{loadout}, componentsOf(held(a).Components))

	// 4. A patch that adds a component, with a presence the client has not seen.
	a.Set(syncCosmetics{Tint: 0, Opacity: 1})
	tick(step)
	added := next()
	require.Len(t, added.Patches, 1)
	require.Len(t, added.Patches[0].Changed, 1)
	presence := added.Patches[0].Changed[0].Presence
	require.Greater(t, presence, uint64(1))
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 4, Time: 50, Patches: []syncstate.Patch{{
		Ref: refOf(a), Revision: 4, Changed: []syncstate.Value{
			{Component: cosmetics, Presence: presence, Data: data(syncCosmetics{Tint: 0, Opacity: 1})},
		},
	}}}, added)

	// 5. Only a grant: a is promoted, and no revision moves.
	a.Set(syncstate.Authorities{Players: players("observer")})
	tick(step)
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 5, Time: 50, Access: []syncstate.Grant{
		{Ref: refOf(a), Access: syncstate.Authority},
	}}, next())

	// 6. A removal: b leaves the observer's view.
	b.Remove[syncstate.Authorities]()
	b.Set(syncstate.Viewers{})
	tick(step)
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 6, Time: 50, Removed: []syncstate.Ref{refOf(b)}}, next())

	// 7. The same lifetime re-enters view: its whole record, at the revision it never left, and a grant.
	b.Set(syncstate.Viewers{Players: players("observer")})
	tick(step)
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 7, Time: 50,
		Records: []syncstate.Record{{Ref: refOf(b), Revision: 1, Presence: 1, Components: []syncstate.Value{
			{Component: loadout, Presence: 1, Data: data(syncLoadout{WeaponID: "axe", Ammo: 2})},
		}}},
		Access: []syncstate.Grant{{Ref: refOf(b), Access: syncstate.View}},
	}, next())

	// 8. Nothing changes for a heartbeat: an empty delta, which still takes a sequence number.
	tick(time.Second)
	require.Equal(t, syncstate.Frame{Stream: 1, Sequence: 8, Time: 1000}, next())
	fixture := raw

	// Recovery: the reader unsubscribes, then subscribes again and gets a fresh baseline.
	resubscribe := func(before func()) syncstate.Frame {
		t.Helper()
		subscriptions := []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}}
		w.service.mu.Lock()
		removeSubscriptions(sub, subscriptions)
		w.service.mu.Unlock()
		tick(step)
		require.Empty(t, sub.queue, "an unsubscribed reader is closed without a frame")
		before()
		w.service.mu.Lock()
		addSubscriptions(sub, subscriptions)
		w.service.mu.Unlock()
		tick(step)
		return next()
	}
	recovered := resubscribe(func() {
		a.Set(syncLoadout{WeaponID: "bow", Ammo: 9})
		a.Set(syncCosmetics{Tint: 123, Opacity: 0.25})
	})
	require.NotNil(t, recovered.Open)
	require.Greater(t, recovered.Stream, uint64(1))
	var appearance syncCosmetics
	require.NoError(t, msgpack.Unmarshal(held(a).Components[0].Data, &appearance))
	require.Equal(t, syncCosmetics{Tint: 123, Opacity: 0.25}, appearance)
	before := held(a)
	unchanged := resubscribe(func() {})
	require.Equal(t, recovered.Records, unchanged.Records, "recovery also restates unchanged state")
	require.Equal(t, before, held(a))
	a.Set(syncLoadout{WeaponID: "bow", Ammo: 9})
	tick(step)
	require.Empty(t, sub.queue, "equal values do not publish")
	if output := os.Getenv("SYNC_INTEROP_OUTPUT"); output != "" {
		data, err := json.Marshal(struct {
			Contract string
			Frames   [][]byte
		}{string(contract), fixture})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(output, data, 0600))
	}
}
