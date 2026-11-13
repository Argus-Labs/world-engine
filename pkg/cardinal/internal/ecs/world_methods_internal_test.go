package ecs

import (
	"fmt"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
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

// TestWorldMethods_IterEntitiesWhileMutating verifies that a search yields each entity that matched
// when it began exactly once, and only while it still matches, even when the callback moves,
// destroys, or creates entities.
func TestWorldMethods_IterEntitiesWhileMutating(t *testing.T) {
	t.Parallel()

	type fixture struct {
		*World

		onlyA, both bitmap.Bitmap
	}
	createN := func(w fixture, components bitmap.Bitmap, n int) {
		for range n {
			w.CreateWithArchetype(components)
		}
	}

	cases := []struct {
		name    string
		matches []SearchMatch
		setup   func(w fixture)
		mutate  func(t *testing.T, w fixture, eid EntityID, first bool)
		want    []EntityID
	}{
		{
			// The {A, B} archetype exists before the search and is scanned after {A}.
			name:    "add component moves entity to a later matching archetype",
			matches: []SearchMatch{MatchContains, MatchAll},
			setup: func(w fixture) {
				createN(w, w.onlyA, 4)
				createN(w, w.both, 1)
			},
			mutate: func(t *testing.T, w fixture, eid EntityID, _ bool) {
				require.NoError(t, w.Set(eid, testutils.ComponentB{}))
			},
			want: []EntityID{0, 1, 2, 3, 4},
		},
		{
			// The {A} archetype exists before the search and is scanned after {A, B}.
			name:    "remove component moves entity to a later matching archetype",
			matches: []SearchMatch{MatchContains, MatchAll},
			setup: func(w fixture) {
				createN(w, w.both, 4)
				createN(w, w.onlyA, 1)
			},
			mutate: func(t *testing.T, w fixture, eid EntityID, _ bool) {
				if w.Has[testutils.ComponentB](eid) {
					require.NoError(t, w.Remove[testutils.ComponentB](eid))
				}
			},
			want: []EntityID{0, 1, 2, 3, 4},
		},
		{
			// The respawn reuses the destroyed ID and fills the archetype slot the search reads next.
			name:    "destroy and respawn skips no entity",
			matches: []SearchMatch{MatchExact, MatchContains, MatchAll},
			setup:   func(w fixture) { createN(w, w.onlyA, 4) },
			mutate: func(t *testing.T, w fixture, eid EntityID, first bool) {
				if first {
					require.True(t, w.Destroy(eid))
					require.Equal(t, eid, w.CreateWithArchetype(w.onlyA))
				}
			},
			want: []EntityID{0, 1, 2, 3},
		},
		{
			name:    "destroyed entities are not yielded",
			matches: []SearchMatch{MatchExact, MatchContains, MatchAll},
			setup:   func(w fixture) { createN(w, w.onlyA, 4) },
			mutate: func(t *testing.T, w fixture, eid EntityID, first bool) {
				if !first {
					return
				}
				for other := range EntityID(4) {
					if other != eid {
						require.True(t, w.Destroy(other))
					}
				}
			},
			want: nil, // Filled in below with the first entity the search yielded.
		},
	}

	for _, tc := range cases {
		for _, match := range tc.matches {
			t.Run(fmt.Sprintf("%s/%s", tc.name, match), func(t *testing.T) {
				t.Parallel()
				w := fixture{World: NewWorld()}
				a, err := w.RegisterComponent[testutils.ComponentA]()
				require.NoError(t, err)
				b, err := w.RegisterComponent[testutils.ComponentB]()
				require.NoError(t, err)
				w.onlyA.Set(a)
				w.both.Set(a)
				w.both.Set(b)
				tc.setup(w)

				query := w.onlyA
				if match == MatchAll {
					query = bitmap.Bitmap{}
				}
				var visited []EntityID
				require.NoError(t, w.IterEntities(query, match, func(eid EntityID) bool {
					require.NoError(t, w.MatchArchetype(eid, query, match), "yielded entity %d no longer matches", eid)
					visited = append(visited, eid)
					tc.mutate(t, w, eid, len(visited) == 1)
					return true
				}))

				want := tc.want
				if want == nil {
					want = visited[:1]
				}
				assert.ElementsMatch(t, want, visited, "each matching entity must be visited exactly once")
				w.CheckWorld(t)
			})
		}
	}
}

// TestWorldMethods_IterEntitiesNested verifies that a search started inside another one, or after
// one stopped early, doesn't disturb the entities either search visits.
func TestWorldMethods_IterEntitiesNested(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	a, err := w.RegisterComponent[testutils.ComponentA]()
	require.NoError(t, err)
	b, err := w.RegisterComponent[testutils.ComponentB]()
	require.NoError(t, err)
	var onlyA, onlyB bitmap.Bitmap
	onlyA.Set(a)
	onlyB.Set(b)
	for range 3 {
		w.CreateWithArchetype(onlyA) // 0, 1, 2
	}
	for range 2 {
		w.CreateWithArchetype(onlyB) // 3, 4
	}

	// Stop a search over every entity early, so a reused buffer is large enough for both searches.
	require.NoError(t, w.IterEntities(bitmap.Bitmap{}, MatchAll, func(EntityID) bool { return false }))
	var outer, inner []EntityID
	require.NoError(t, w.IterEntities(onlyA, MatchContains, func(eid EntityID) bool {
		outer = append(outer, eid)
		require.NoError(t, w.IterEntities(onlyB, MatchContains, func(eid EntityID) bool {
			inner = append(inner, eid)
			return true
		}))
		return true
	}))

	assert.ElementsMatch(t, []EntityID{0, 1, 2}, outer)
	assert.ElementsMatch(t, []EntityID{3, 4, 3, 4, 3, 4}, inner)
}
