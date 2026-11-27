package ecs

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests check that the world lets go of component data it no longer holds. Each payload is
// tracked with a weak pointer, so after a full collection it is reachable only if something still
// references it. Each test also checks that payloads the world does hold stay reachable, so a test
// can't pass because the owner itself was collected.

// retainedPayload is a component that references a heap allocation.
type retainedPayload struct{ Payload string }

func (retainedPayload) Name() string { return "retained_payload" }

const retentionRows = 32

// newPayload returns a payload in its own allocation and a weak pointer to it.
func newPayload(i int) (string, weak.Pointer[byte]) {
	payload := strings.Repeat(string(rune('a'+i%26)), 4<<10)
	return payload, weak.Make(unsafe.StringData(payload))
}

// reachable collects garbage and counts the payloads that are still alive.
//
//go:noinline
func reachable(refs []weak.Pointer[byte]) int {
	runtime.GC()
	runtime.GC()

	n := 0

	for _, ref := range refs {
		if ref.Value() != nil {
			n++
		}
	}

	return n
}

// newPayloadColumn returns a column of retentionRows payloads and their weak pointers. It doesn't
// inline, so no payload lingers in the caller's frame.
//
//go:noinline
func newPayloadColumn() (*column[retainedPayload], []weak.Pointer[byte]) {
	col := newColumn[retainedPayload]()
	refs := make([]weak.Pointer[byte], retentionRows)

	for row := range retentionRows {
		payload, ref := newPayload(row)
		refs[row] = ref

		col.extend()
		col.set(row, retainedPayload{Payload: payload})
	}

	return &col, refs
}

//go:noinline
func setNewPayload(col *column[retainedPayload], row, i int) weak.Pointer[byte] {
	payload, ref := newPayload(i)
	col.set(row, retainedPayload{Payload: payload})

	return ref
}

func TestColumn_RemoveReleasesComponent(t *testing.T) {
	t.Parallel()

	t.Run("last row", func(t *testing.T) {
		t.Parallel()

		col, refs := newPayloadColumn()

		for row := retentionRows - 1; row >= retentionRows/2; row-- {
			col.remove(row)
		}

		assert.Zero(t, reachable(refs[retentionRows/2:]), "removed components stay reachable")
		assert.Equal(t, retentionRows/2, reachable(refs[:retentionRows/2]), "kept components are lost")
		runtime.KeepAlive(col)
	})

	t.Run("swap remove", func(t *testing.T) {
		t.Parallel()

		col, refs := newPayloadColumn()

		// Each remove moves the last component into the removed row.
		for row := range retentionRows / 2 {
			col.remove(row)
		}

		require.Zero(t, reachable(refs[:retentionRows/2]), "removed components stay reachable")
		require.Equal(t, retentionRows/2, reachable(refs[retentionRows/2:]), "moved components are lost")

		// Once the moved components are overwritten, only their old slots past the end could hold them.
		replaced := make([]weak.Pointer[byte], 0, col.len())
		for row := range col.len() {
			replaced = append(replaced, setNewPayload(col, row, row))
		}

		assert.Zero(t, reachable(refs), "overwritten components stay reachable")
		assert.Equal(t, len(replaced), reachable(replaced), "replacement components are lost")
		runtime.KeepAlive(col)
	})
}

// newPayloadWorld returns a world whose entities each hold a payload, half of them also
// ComponentA, so the payloads span two archetypes.
//
//go:noinline
func newPayloadWorld(t *testing.T) (*World, []EntityID, []weak.Pointer[byte]) {
	t.Helper()

	w := NewWorld()
	_, err := w.RegisterComponent[retainedPayload]()
	require.NoError(t, err)
	_, err = w.RegisterComponent[testutils.ComponentA]()
	require.NoError(t, err)

	ids := make([]EntityID, retentionRows)
	refs := make([]weak.Pointer[byte], retentionRows)

	for i := range retentionRows {
		payload, ref := newPayload(i)
		refs[i] = ref
		ids[i] = w.Create()
		require.NoError(t, w.Set(ids[i], retainedPayload{Payload: payload}))

		if i%2 == 0 {
			require.NoError(t, w.Set(ids[i], testutils.ComponentA{}))
		}
	}

	return w, ids, refs
}

//go:noinline
func createWithNewPayload(t *testing.T, w *World) weak.Pointer[byte] {
	t.Helper()

	payload, ref := newPayload(0)
	require.NoError(t, w.Set(w.Create(), retainedPayload{Payload: payload}))

	return ref
}

func TestWorld_ReleasesRetiredComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		retire func(t *testing.T, w *World, ids []EntityID)
	}{
		{
			name: "destroy",
			retire: func(t *testing.T, w *World, ids []EntityID) {
				t.Helper()

				for _, id := range ids {
					require.True(t, w.Destroy(id))
				}
			},
		},
		{
			name: "remove component",
			retire: func(t *testing.T, w *World, ids []EntityID) {
				t.Helper()

				for _, id := range ids {
					require.NoError(t, w.Remove[retainedPayload](id))
				}
			},
		},
		{
			name: "reset",
			retire: func(t *testing.T, w *World, _ []EntityID) {
				t.Helper()

				w.Reset()
				w.Reset() // An idle reset must not bring the old archetypes back either.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w, ids, refs := newPayloadWorld(t)
			tt.retire(t, w, ids)
			assert.Zero(t, reachable(refs), "retired components stay reachable from the world")

			// Checked after, since a new archetype can take a retired one's slot and hide it.
			live := createWithNewPayload(t, w)
			assert.Equal(t, 1, reachable([]weak.Pointer[byte]{live}), "a live component is lost")
			runtime.KeepAlive(w)
		})
	}
}
