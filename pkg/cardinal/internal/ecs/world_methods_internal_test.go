package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/require"
)

func TestWorldMethods_EntityLifecycle(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	other := NewWorld()
	cid, err := w.RegisterComponent[testutils.ComponentA]()
	require.NoError(t, err)
	var components bitmap.Bitmap
	components.Set(cid)
	eid := w.CreateWithArchetype(components)
	require.True(t, w.Alive(eid))
	require.False(t, other.Alive(eid))
	require.True(t, w.Has[testutils.ComponentA](eid))
	require.NoError(t, w.Set(eid, testutils.ComponentA{X: 3, Y: 4, Z: 5}))
	value, err := w.Get[testutils.ComponentA](eid)
	require.NoError(t, err)
	require.Equal(t, testutils.ComponentA{X: 3, Y: 4, Z: 5}, value)
	require.NoError(t, w.MatchArchetype(eid, components, MatchExact))
	var values []testutils.ComponentA
	require.NoError(t, w.IterEntities(components, MatchContains, func(id EntityID) bool {
		item, getErr := w.Get[testutils.ComponentA](id)
		require.NoError(t, getErr)
		values = append(values, item)
		return true
	}))
	require.Equal(t, []testutils.ComponentA{{X: 3, Y: 4, Z: 5}}, values)
	require.NoError(t, w.Remove[testutils.ComponentA](eid))
	_, err = w.Get[testutils.ComponentA](eid)
	require.ErrorContains(t, err, "doesn't contain component component_a")
	require.True(t, w.Destroy(eid))
	require.False(t, w.Alive(eid))
	require.False(t, w.Destroy(eid))
	empty := w.Create()
	require.True(t, w.Alive(empty))
	w.CheckWorld(t)
}

func TestWorldMethods_SystemEvents(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	_, err := w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.NoError(t, w.RegisterSystem("emit", Update, func() {
		require.NoError(t, w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 42}))
	}))
	var values []testutils.SimpleSystemEvent
	require.NoError(t, w.RegisterSystem("receive", PostUpdate, func() {
		events, err := w.GetSystemEvents[testutils.SimpleSystemEvent]()
		require.NoError(t, err)
		values = append(values, events...)
	}))
	w.Init()
	w.Tick()
	require.Equal(t, []testutils.SimpleSystemEvent{{Value: 42}}, values)
	events, err := w.GetSystemEvents[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Empty(t, events)
}

// TestWorldMethods_SystemEvents_ResetClearsInitBuffer verifies that Reset drains the
// system-event buffer. Init emits into the buffer outside Tick's deferred clear, so
// Reset must take ownership of draining init-emitted events to keep re-Init self-contained.
func TestWorldMethods_SystemEvents_ResetClearsInitBuffer(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	_, err := w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.NoError(t, w.RegisterSystem("emit-on-init", Init, func() {
		require.NoError(t, w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 1}))
	}))

	w.Init() // emits one event into the buffer
	events, err := w.GetSystemEvents[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Len(t, events, 1)

	w.Reset() // must drain the buffer alongside world state
	events, err = w.GetSystemEvents[testutils.SimpleSystemEvent]()
	require.NoError(t, err)
	require.Empty(t, events, "Reset must clear buffered system events")
}
