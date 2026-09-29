package physics2d_test

import (
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

type feSpawnSystem struct{ run func(w *cardinal.World) }

func (s *feSpawnSystem) Run(w *cardinal.World) { s.run(w) }

type feEditSystem struct{ run func(w *cardinal.World) }

func (s *feEditSystem) Run(w *cardinal.World) { s.run(w) }

// A ball rests on a floor; the floor's mask gains a bit that still allows the ball. The ball
// never leaves the floor, so no End should fire and the pair should stay in ActiveContacts.
func TestFilterEditKeepsRestingContact(t *testing.T) {
	runFilterEdit(t, func(mask uint64) uint64 {
		return mask | lootBit
	}, func(t *testing.T, ballY float64, ends int, listed bool) {
		require.InDelta(t, 0.5, ballY, 0.01, "the ball is still resting on the floor")
		require.Zero(t, ends, "the ball never left the floor, so no End should fire")
		require.True(t, listed, "the pair should still be in ActiveContacts")
	})
}

// The floor's mask drops the ball's bit. The contact must End once and the ball falls through.
func TestFilterEditEndsExcludedContact(t *testing.T) {
	runFilterEdit(t, func(mask uint64) uint64 {
		return mask &^ ballBit
	}, func(t *testing.T, ballY float64, ends int, listed bool) {
		require.Less(t, ballY, 0.0, "the ball fell through the floor")
		require.Equal(t, 1, ends, "exactly one End")
		require.False(t, listed, "the pair left ActiveContacts")
	})
}

const floorBit, ballBit, lootBit uint64 = 1 << 0, 1 << 1, 1 << 2

func runFilterEdit(t *testing.T,
	edit func(mask uint64) uint64, check func(t *testing.T, ballY float64, ends int, listed bool)) {
	floors := map[string]physics.Shape{
		"chain": physics.Chain(physics.Vec2{X: 10}, physics.Vec2{X: 5}, physics.Vec2{X: -5}, physics.Vec2{X: -10}),
		"box":   physics.Box(10, 0.5).At(physics.Vec2{Y: -0.5}, 0),
	}
	for name, floorShape := range floors {
		t.Run(name, func(t *testing.T) {
			w, _ := makeWorld(t, physics.Vec2{Y: -10})

			var floor, ball cardinal.EntityID
			w.RegisterSystem(&feSpawnSystem{run: func(w *cardinal.World) {
				if w.TickHeight() != 0 {
					return
				}
				f := w.Create[spawnArchetype]()
				f.Set(harnessTag{Role: "floor"})
				f.Set(physics.Transform2D{})
				f.Set(physics.Velocity2D{})
				f.Set(newRigid(physics.BodyTypeStatic, floorShape.Filter(floorBit, floorBit|ballBit)))
				floor = f.ID()

				b := w.Create[spawnArchetype]()
				b.Set(harnessTag{Role: "ball"})
				b.Set(physics.Transform2D{Position: physics.Vec2{Y: 2}})
				b.Set(physics.Velocity2D{})
				b.Set(newRigid(physics.BodyTypeDynamic, physics.Circle(0.5).Filter(ballBit, ^uint64(0))))
				ball = b.ID()
			}}, cardinal.WithHook(cardinal.Init))

			var (
				editMask bool
				ends     int
				listed   bool
				ballY    float64
			)
			w.RegisterSystem(&feEditSystem{run: func(w *cardinal.World) {
				for ev := range w.SystemEvents[physics.ContactEndEvent]() {
					if pairHas(ev.EntityA, ev.EntityB, floor, ball) {
						ends++
					}
				}
				listed = false
				for row := range w.Exact[contactRow]().Iter() {
					listed = slices.ContainsFunc(slices.Collect(row.Get[physics.ActiveContacts]().Pairs.Values()),
						func(p physics.ContactPairEntry) bool { return pairHas(p.EntityA, p.EntityB, floor, ball) })
				}
				for row := range w.Exact[spawnArchetype]().Iter() {
					switch {
					case row.ID() == ball:
						ballY = row.Get[physics.Transform2D]().Position.Y
					case row.ID() == floor && editMask:
						editMask = false
						pb := row.Get[physics.PhysicsBody2D]()
						s := pb.Shapes.At(0)
						s.MaskBits = edit(s.MaskBits)
						pb.Shapes = pb.Shapes.With(0, s)
						row.Set(pb)
					}
				}
			}}, cardinal.WithHook(cardinal.Update))

			initCardinalECS(w)
			tickN(t, w, 120)
			require.True(t, listed, "setup: the ball rests on the floor")
			require.Zero(t, ends, "setup: no End while settling")

			editMask = true
			tickN(t, w, 60)
			check(t, ballY, ends, listed)
		})
	}
}
