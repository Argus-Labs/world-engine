package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	filterEditOldLayer uint64 = 1 << 0
	filterEditNewLayer uint64 = 1 << 1
)

type filterEditSpawnSystem struct{ run func(w *cardinal.World) }

func (s *filterEditSpawnSystem) Run(w *cardinal.World) { s.run(w) }

type filterEditSystem struct{ run func(w *cardinal.World) }

func (s *filterEditSystem) Run(w *cardinal.World) { s.run(w) }

// TestShapeFilterEditReachesQueries moves one collider slot from an old layer to a new one and
// checks that queries see the move on the next tick, without a rebuild.
//
// Chain slots have no per-slot shape id (the chain owns its segment shapes), so the in-place
// edit path once skipped them and still advanced its shadow: ECS reported the new layer while
// queries and contacts kept the old one until something forced a rebuild.
func TestShapeFilterEditReachesQueries(t *testing.T) {
	t.Parallel()

	edge := physics.ColliderShape{
		ShapeType:    physics.ShapeTypeEdge,
		EdgeVertices: [2]physics.Vec2{{X: -2}, {X: 2}},
	}
	chain := physics.ColliderShape{
		ShapeType: physics.ShapeTypeStaticChain,
		ChainPoints: immutable.SliceOf(
			physics.Vec2{X: -3}, physics.Vec2{X: -2}, physics.Vec2{X: 2}, physics.Vec2{X: 3},
		),
	}
	loop := physics.ColliderShape{
		ShapeType: physics.ShapeTypeStaticChainLoop,
		ChainPoints: immutable.SliceOf(
			physics.Vec2{X: -2, Y: -1}, physics.Vec2{X: 2, Y: -1}, physics.Vec2{X: 2, Y: 1}, physics.Vec2{X: -2, Y: 1},
		),
	}
	box := physics.ColliderShape{ShapeType: physics.ShapeTypeBox, HalfExtents: physics.Vec2{X: 0.5, Y: 0.5}}
	at := func(s physics.ColliderShape, y float64) physics.ColliderShape {
		s.LocalOffset = physics.Vec2{Y: y}
		return s
	}

	cases := []struct {
		name   string
		shapes []physics.ColliderShape
		edit   int  // slot moved to the new layer
		reset  bool // rebuild the world after the edit instead of reconciling it in place
	}{
		{name: "edge", shapes: []physics.ColliderShape{edge}},
		{name: "chain", shapes: []physics.ColliderShape{chain}},
		{name: "chain loop", shapes: []physics.ColliderShape{loop}},
		{
			// Only the second chain moves: an edit must land on its own slot's chain, not the first.
			name:   "second chain of a compound",
			shapes: []physics.ColliderShape{at(box, -6), at(chain, -3), at(edge, 0), at(chain, 3)},
			edit:   3,
		},
		{name: "chain across reset", shapes: []physics.ColliderShape{chain}, reset: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w, p := makeWorld(t, physics.Vec2{})

			shapes := make([]physics.ColliderShape, len(tc.shapes))
			for i, s := range tc.shapes {
				s.CategoryBits = filterEditOldLayer
				s.MaskBits = ^uint64(0)
				shapes[i] = s
			}

			var (
				id      cardinal.EntityID
				pending bool
			)

			w.RegisterSystem(&filterEditSpawnSystem{run: func(w *cardinal.World) {
				row := w.Create[spawnArchetype]()
				row.Set(harnessTag{Role: "filtered"})
				row.Set(physics.Transform2D{})
				row.Set(physics.Velocity2D{})
				row.Set(newRigid(physics.BodyTypeStatic, shapes...))
				id = row.ID()
			}}, cardinal.WithPhase(cardinal.Init))
			w.RegisterSystem(&filterEditSystem{run: func(w *cardinal.World) {
				if !pending {
					return
				}

				pending = false

				for row := range w.Exact[spawnArchetype]().Iter() {
					if row.ID() != id {
						continue
					}

					pb := row.Get[physics.PhysicsBody2D]()
					s := pb.Shapes.At(tc.edit)
					s.CategoryBits = filterEditNewLayer
					pb.Shapes.Set(tc.edit, s)
					row.Set(pb)
				}
			}}, cardinal.WithPhase(cardinal.Update))

			initCardinalECS(w)
			tickN(t, w, 2)

			all := make([]int, len(shapes))
			for i := range all {
				all[i] = i
			}

			require.ElementsMatch(t, all, filterEditHits(p, filterEditOldLayer),
				"setup: every slot starts on the old layer")
			require.Empty(t, filterEditHits(p, filterEditNewLayer), "setup: no slot starts on the new layer")

			// The edit lands in Update; the pipeline reconciles it in the next tick's PreUpdate.
			pending = true

			tickN(t, w, 1)

			if tc.reset {
				p.Reset()
			}

			tickN(t, w, 2)

			var unchanged []int

			for _, i := range all {
				if i != tc.edit {
					unchanged = append(unchanged, i)
				}
			}

			assert.ElementsMatch(t, unchanged, filterEditHits(p, filterEditOldLayer),
				"only the slots left alone stay on the old layer")
			assert.ElementsMatch(t, []int{tc.edit}, filterEditHits(p, filterEditNewLayer),
				"the edited slot answers on the new layer")
		})
	}
}

// filterEditHits returns the collider slots that an overlap query masked to layer reports
// inside the area every test body occupies.
func filterEditHits(p *physics.Plugin, layer uint64) []int {
	res := p.OverlapAABB(physics.AABBOverlapRequest{
		Min:    physics.Vec2{X: -8, Y: -8},
		Max:    physics.Vec2{X: 8, Y: 8},
		Filter: &physics.Filter{CategoryBits: ^uint64(0), MaskBits: layer},
	})

	slots := make([]int, 0, len(res.Hits))
	for _, h := range res.Hits {
		slots = append(slots, h.ShapeIndex)
	}

	return slots
}
