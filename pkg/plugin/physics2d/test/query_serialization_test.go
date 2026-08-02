package physics2d_test

import (
	"testing"

	json "github.com/goccy/go-json"
	"github.com/stretchr/testify/require"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// TestQuery_EmptyOverlapMarshalsAsEmptyArray pins the JSON shape of a miss.
//
// A query result crosses process boundaries (persisted, or returned to a
// client), so whether an empty hit list marshals as [] or null is part of the
// public contract, not an implementation detail. The CGO backend always
// produced a non-nil slice; a nil slice here would silently change the wire
// form to "hits":null. Equality assertions on the decoded value do not catch
// this, which is why it is asserted on the encoded bytes.
func TestQuery_EmptyOverlapMarshalsAsEmptyArray(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// One body, parked far away from the region queried below.
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "far"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 500, Y: 500}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 1, Y: 1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 2)

	// Query a region containing nothing, against a live world.
	res := p.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -1, Y: -1},
		Max: physics.Vec2{X: 1, Y: 1},
	})
	require.Empty(t, res.Hits, "region is empty, so there should be no hits")
	require.NotNil(t, res.Hits, "an empty hit list must stay non-nil so it marshals as []")

	encoded, err := json.Marshal(res)
	require.NoError(t, err)
	require.JSONEq(t, `{"hits":[]}`, string(encoded))
}
