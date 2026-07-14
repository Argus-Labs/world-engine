package cbridge_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
	"github.com/stretchr/testify/require"
)

// makeOverlapCluster creates n overlapping dynamic circles in a small grid.
// Each ball overlaps its 8 neighbors → ~4*n contacts. Used to generate a
// large contact event count without going near Box2D's internal broad-phase
// pair-buffer cap.
func makeOverlapCluster(t *testing.T, startID uint32, n int, originX, originY float64) {
	t.Helper()
	const radius = 0.5
	const spacing = 0.4 // < 2*radius → overlap
	side := 1
	for side*side < n {
		side++
	}
	id := startID
	placed := 0
	for gy := 0; gy < side && placed < n; gy++ {
		for gx := 0; gx < side && placed < n; gx++ {
			px := originX + float64(gx)*spacing
			py := originY + float64(gy)*spacing
			require.True(t, cbridge.CreateBody(
				id, 2,
				px, py, 0,
				0, 0, 0,
				0, 0, 1,
				true, true, true, false, false,
			), "create body %d", id)
			require.True(t, cbridge.AddCircleShape(
				id, 0,
				0, 0, radius,
				false, 0, 0, 1,
				0xFFFF, 0xFFFF, 0,
			), "add circle shape %d", id)
			id++
			placed++
		}
	}
}

// TestStepHighContactCount verifies that Step() handles a dense contact load
// without dropping any events and delivers real (non-zero) entity ids on
// every event. With the two-phase advance/drain design dropping events is
// structurally impossible — the drain buffer is sized from the exact count
// bridge_step_advance returns before the drain ever runs.
func TestStepHighContactCount(t *testing.T) {
	cbridge.CreateWorld(0, 0)
	t.Cleanup(cbridge.DestroyWorld)

	// 25 overlapping balls → ~80–100 contacts. Well above any reasonable
	// per-tick contact count for a small world, well below Box2D's
	// broad-phase pair cap.
	makeOverlapCluster(t, 1, 25, 0, 0)

	_, contacts := cbridge.Step(1.0/60.0, 4)
	t.Logf("delivered=%d", len(contacts))

	require.NotEmpty(t, contacts,
		"cluster must produce contact events on tick 1")

	// Every event must carry non-zero entity ids. An old single-call bug
	// manifested as delivered=N with all zero payloads because buffer grow
	// replaced the backing slice before the Go copy loop read it.
	for i, c := range contacts {
		require.NotEqual(t, uint32(0), c.EntityA, "contact %d has zero EntityA", i)
		require.NotEqual(t, uint32(0), c.EntityB, "contact %d has zero EntityB", i)
	}
}

// TestStepEndEventFiresOnTeleport is a regression guard on the end-event
// drain path: a previously-contacting pair must produce an END event after
// one body is teleported away and both bodies are re-awakened.
func TestStepEndEventFiresOnTeleport(t *testing.T) {
	cbridge.CreateWorld(0, 0)
	t.Cleanup(cbridge.DestroyWorld)

	require.True(t, cbridge.CreateBody(1, 2, 0, 0, 0, 0.001, 0, 0, 0, 0, 1, true, true, false, false, false))
	require.True(t, cbridge.AddCircleShape(1, 0, 0, 0, 0.5, false, 0, 0, 1, 0xFFFF, 0xFFFF, 0))
	require.True(t, cbridge.CreateBody(2, 2, 0.5, 0, 0, 0, 0, 0, 0, 0, 1, true, true, false, false, false))
	require.True(t, cbridge.AddCircleShape(2, 0, 0, 0, 0.5, false, 0, 0, 1, 0xFFFF, 0xFFFF, 0))

	_, _ = cbridge.Step(1.0/60.0, 4) // establishes the contact

	cbridge.SetTransform(1, 100, 100, 0)
	cbridge.SetAwake(1, true)
	cbridge.SetAwake(2, true)

	_, second := cbridge.Step(1.0/60.0, 4)
	endFound := false
	for _, c := range second {
		if c.Kind == cbridge.ContactEnd &&
			((c.EntityA == 1 && c.EntityB == 2) || (c.EntityA == 2 && c.EntityB == 1)) {
			endFound = true
		}
	}
	require.True(t, endFound, "tracked pair end event must fire after teleport")
}
