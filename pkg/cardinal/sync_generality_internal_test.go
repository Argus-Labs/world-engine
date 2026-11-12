package cardinal

import (
	"context"
	"encoding/json"
	syncstate "github.com/argus-labs/world-engine/pkg/cardinal/sync"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"os"
	"testing"
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

// Uses no Health registration. Optionally exports actual queued frames for Unity's generated-code test.
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
	var raw [][]byte
	capture := func() syncstate.Record {
		t.Helper()
		require.Len(t, sub.queue, 1)
		raw = append(raw, append([]byte(nil), sub.queue[0].response.Event.Payload...))
		frames := syncFrames(t, sub)
		require.Len(t, frames, 1)
		require.Len(t, frames[0].Records, 1)
		return frames[0].Records[0]
	}
	readLoadout := func(r syncstate.Record) syncLoadout {
		t.Helper()
		for _, c := range r.Components {
			if c.Name == "loadout" {
				var v syncLoadout
				require.NoError(t, msgpack.Unmarshal(c.Data, &v))
				return v
			}
		}
		t.Fatal("missing loadout")
		return syncLoadout{}
	}
	e := w.Create[syncEquipment]()
	e.Set(syncLoadout{WeaponID: "wand"})
	e.Set(syncCosmetics{Tint: ^uint32(0), Opacity: 0.5})
	e.Set(syncPrivate{Secret: "server only"})
	w.Tick()
	first := capture()
	require.Len(t, first.Components, 2)
	require.Equal(t, syncLoadout{WeaponID: "wand"}, readLoadout(first))
	e.Set(syncLoadout{WeaponID: "sword", Ammo: 7, Equipped: true})
	w.Tick()
	updated := capture()
	require.Greater(t, updated.Revision, first.Revision)
	require.Equal(t, uint16(7), readLoadout(updated).Ammo)
	e.Remove[syncCosmetics]()
	w.Tick()
	absent := capture()
	require.Len(t, absent.Components, 1)
	require.Equal(t, "loadout", absent.Components[0].Name)
	require.True(t, readLoadout(absent).Equipped)
	e.Set(syncCosmetics{Tint: 0, Opacity: 1})
	w.Tick()
	restored := capture()
	require.Len(t, restored.Components, 2)
	require.Greater(t, restored.Components[0].Presence, first.Components[0].Presence)
	// Recovery: the reader unsubscribes, then subscribes again and gets a fresh baseline.
	resubscribe := func(before func()) {
		t.Helper()
		subscriptions := []*cardinalv1.EventSubscription{{Events: []string{syncstate.EventName}}}
		w.service.mu.Lock()
		removeSubscriptions(sub, subscriptions)
		w.service.mu.Unlock()
		w.Tick()
		require.Empty(t, sub.queue, "an unsubscribed reader is closed without a frame")
		before()
		w.service.mu.Lock()
		addSubscriptions(sub, subscriptions)
		w.service.mu.Unlock()
		w.Tick()
	}
	resubscribe(func() {
		e.Set(syncLoadout{WeaponID: "bow", Ammo: 9})
		e.Set(syncCosmetics{Tint: 123, Opacity: 0.25})
	})
	recovered := capture()
	require.Equal(t, syncLoadout{WeaponID: "bow", Ammo: 9}, readLoadout(recovered))
	var appearance syncCosmetics
	require.NoError(t, msgpack.Unmarshal(recovered.Components[0].Data, &appearance))
	require.Equal(t, syncCosmetics{Tint: 123, Opacity: 0.25}, appearance)
	resubscribe(func() {})
	unchanged := capture()
	require.Equal(t, recovered.Components, unchanged.Components, "recovery also restates unchanged state")
	e.Set(syncLoadout{WeaponID: "bow", Ammo: 9})
	w.Tick()
	require.Empty(t, sub.queue, "equal values do not publish")
	if output := os.Getenv("SYNC_INTEROP_OUTPUT"); output != "" {
		data, err := json.Marshal(struct {
			Contract string
			Frames   [][]byte
		}{string(contract), raw})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(output, data, 0600))
	}
}
