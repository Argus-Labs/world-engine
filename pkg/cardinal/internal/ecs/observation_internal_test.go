package ecs

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserve_TellsObserversInRegistrationOrder(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	var told []string
	w.Observe(func(m Mutation) { told = append(told, "first") })
	w.Observe(func(m Mutation) { told = append(told, "second") })

	w.Create()

	assert.Equal(t, []string{"first", "second"}, told)
}

// An observer added to a world that already holds entities learns of each one. Their lifetimes
// were started by the first observer, and a later one does not renumber them.
func TestObserve_LaterObserverSeesLiveEntitiesAsCreated(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	var first []Mutation
	w.Observe(func(m Mutation) { first = append(first, m) })
	a, b := w.Create(), w.Create()
	session, incarnation := w.Identity(b)
	first = nil

	var second []Mutation
	w.Observe(func(m Mutation) { second = append(second, m) })

	assert.Equal(t, []Mutation{{Entity: a, Kind: Created}, {Entity: b, Kind: Created}}, second)
	assert.Empty(t, first, "an earlier observer is not told again")
	gotSession, gotIncarnation := w.Identity(b)
	assert.Equal(t, session, gotSession)
	assert.Equal(t, incarnation, gotIncarnation)
}

func TestReadOnly_RefusesEveryWrite(t *testing.T) {
	t.Parallel()
	writes := map[string]func(w *World, eid EntityID){
		"Create":  func(w *World, _ EntityID) { w.Create() },
		"Destroy": func(w *World, eid EntityID) { w.Destroy(eid) },
		"Set":     func(w *World, eid EntityID) { _ = w.Set(eid, testutils.ComponentA{X: 1}) },
		"Remove":  func(w *World, eid EntityID) { _ = w.Remove[testutils.ComponentA](eid) },
		"Reset":   func(w *World, _ EntityID) { w.Reset() },
		"FromProto": func(w *World, _ EntityID) {
			state, err := NewWorld().ToProto()
			if err != nil {
				panic(err)
			}
			_ = w.FromProto(state)
		},
		"EmitSystemEvent": func(w *World, _ EntityID) { _ = w.EmitSystemEvent(testutils.SimpleSystemEvent{Value: 1}) },
	}
	for op, write := range writes {
		t.Run(op, func(t *testing.T) {
			t.Parallel()
			w := NewWorld()
			_, err := w.RegisterComponent[testutils.ComponentA]()
			require.NoError(t, err)
			_, err = w.RegisterSystemEvent[testutils.SimpleSystemEvent]()
			require.NoError(t, err)
			eid := w.Create()
			require.NoError(t, w.Set(eid, testutils.ComponentA{X: 7}))

			previous := w.ReadOnly("the test's reader")
			assert.PanicsWithValue(t,
				"cardinal: the test's reader must not change the world, and it called "+op,
				func() { write(w, eid) })
			value, err := w.Get[testutils.ComponentA](eid)
			require.NoError(t, err, "a refused write changes nothing")
			assert.Equal(t, testutils.ComponentA{X: 7}, value)

			w.ReadOnly(previous)
			assert.NotPanics(t, func() { write(w, eid) }, "the world takes writes again")
		})
	}
}

// A change is committed under the world's lock, so an observer that wrote back would deadlock.
// It panics instead, and the world takes writes again once the panic has unwound.
func TestObserve_ObserverCannotWriteToTheWorld(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	writeBack := true
	w.Observe(func(m Mutation) {
		if writeBack && m.Kind == Created {
			w.Create()
		}
	})

	assert.PanicsWithValue(t,
		"cardinal: a mutation observer must not change the world, and it called Create",
		func() { w.Create() })

	writeBack = false
	assert.NotPanics(t, func() { w.Create() })
}
