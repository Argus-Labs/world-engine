package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Query ergonomics: AABBOverlapResult.Entities and the requests' Ignore list.
// ---------------------------------------------------------------------------

// spawnQueryScene puts three static bodies in a row on the X axis at x = 0, 2 and 4, each
// carrying a box small enough that they stay apart and wide enough for a ray along y=0 to
// cross all three. The body at x=0 gets two slots pointing at the same shape entity, both
// inside the query volumes below: it is what proves per-shape hits collapse to one entity.
func spawnQueryScene(t *testing.T, w *cardinal.World) (*cardinal.EntityID, *cardinal.EntityID, *cardinal.EntityID) {
	t.Helper()
	near, mid, far := new(cardinal.EntityID), new(cardinal.EntityID), new(cardinal.EntityID)
	w.RegisterSystem(func(state *spawnState) {
		if state.Tick() != 0 {
			return
		}
		spawn := func(role string, x float64, shapes ...physics.ShapeRef) cardinal.EntityID {
			row := state.Spawn.Create()
			row.Set(harnessTag{Role: role})
			row.Set(physics.Transform2D{Position: physics.Vec2{X: x, Y: 0}})
			row.Set(physics.Velocity2D{})
			row.Set(newRigid(physics.BodyTypeStatic, shapes...))
			return row.ID()
		}
		// One shape entity, two slots: still one entity, two hits.
		twoSlot := boxSlot(state, 0.4, 0.4)
		*near = spawn("near", 0, twoSlot, twoSlot.At(physics.Vec2{X: 0.5, Y: 0}, 0))
		*mid = spawn("mid", 2, boxSlot(state, 0.4, 0.4))
		*far = spawn("far", 4, boxSlot(state, 0.4, 0.4))
	}, cardinal.WithHook(cardinal.Init))
	return near, mid, far
}

// TestOverlapEntitiesCollapsesPerShapeHits is the dedupe every caller was writing by hand:
// a body whose several shapes all overlap must appear once, while Hits keeps the detail.
func TestOverlapEntitiesCollapsesPerShapeHits(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	near, mid, _ := spawnQueryScene(t, w)
	initCardinalECS(w)
	tickN(t, w, 2)

	res := p.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -1, Y: -1}, Max: physics.Vec2{X: 3, Y: 1},
	})

	require.Greater(t, len(res.Hits), 2, "the two-slot body should report more hits than entities")
	require.Equal(t, []cardinal.EntityID{*near, *mid}, res.Entities(),
		"Entities collapses per-shape hits and keeps Hits order")
}

func TestOverlapEntitiesIsNilWhenNothingHit(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	spawnQueryScene(t, w)
	initCardinalECS(w)
	tickN(t, w, 2)

	res := p.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: 50, Y: 50}, Max: physics.Vec2{X: 51, Y: 51},
	})
	require.Empty(t, res.Hits)
	require.Nil(t, res.Entities())
}

func TestOverlapIgnoreSkipsListedEntities(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	near, mid, _ := spawnQueryScene(t, w)
	initCardinalECS(w)
	tickN(t, w, 2)

	req := physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -1, Y: -1}, Max: physics.Vec2{X: 3, Y: 1},
	}
	require.Equal(t, []cardinal.EntityID{*near, *mid}, p.OverlapAABB(req).Entities())

	req.Ignore = []cardinal.EntityID{*near}
	got := p.OverlapAABB(req)
	require.Equal(t, []cardinal.EntityID{*mid}, got.Entities(),
		"every slot of an ignored body is skipped, not just the first")
	for _, h := range got.Hits {
		require.NotEqual(t, *near, h.Entity)
	}
}

// TestRaycastIgnoreReportsTheNextHit is the reason Ignore lives on the request rather than
// being a filter over the result: the cast must keep going past an ignored body and return
// what is behind it. Post-filtering would have returned no hit at all.
func TestRaycastIgnoreReportsTheNextHit(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	near, mid, _ := spawnQueryScene(t, w)
	initCardinalECS(w)
	tickN(t, w, 2)

	req := physics.RaycastRequest{
		Origin: physics.Vec2{X: -3, Y: 0}, End: physics.Vec2{X: 3, Y: 0},
	}
	first := p.Raycast(req)
	require.True(t, first.Hit)
	require.Equal(t, *near, first.Entity, "the nearest body is hit first")

	req.Ignore = []cardinal.EntityID{*near}
	second := p.Raycast(req)
	require.True(t, second.Hit, "ignoring the nearest body must not lose the ray")
	require.Equal(t, *mid, second.Entity, "the ray continues to the body behind it")
	require.Greater(t, second.Fraction, first.Fraction)
}

func TestCircleSweepIgnoreReportsTheNextHit(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	near, mid, _ := spawnQueryScene(t, w)
	initCardinalECS(w)
	tickN(t, w, 2)

	req := physics.CircleSweepRequest{
		Start: physics.Vec2{X: -3, Y: 0}, End: physics.Vec2{X: 3, Y: 0}, Radius: 0.1,
	}
	first := p.CircleSweep(req)
	require.True(t, first.Hit)
	require.Equal(t, *near, first.Entity)

	req.Ignore = []cardinal.EntityID{*near}
	second := p.CircleSweep(req)
	require.True(t, second.Hit, "ignoring the nearest body must not lose the sweep")
	require.Equal(t, *mid, second.Entity)
}

// TestIgnoreAcceptsSeveralEntities guards the list form: skipping two bodies reaches the third.
func TestIgnoreAcceptsSeveralEntities(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})
	near, mid, far := spawnQueryScene(t, w)
	initCardinalECS(w)
	tickN(t, w, 2)

	res := p.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: -3, Y: 0}, End: physics.Vec2{X: 6, Y: 0},
		Ignore: []cardinal.EntityID{*near, *mid},
	})
	require.True(t, res.Hit)
	require.Equal(t, *far, res.Entity)
}
