package physics2d_test

// ActiveContacts read the way a game reads it: an Exact search on the two re-exported
// components. Each entry carries both shapes' filter bits, and those persisted bits are what
// the End for a pair that vanished across a rebuild reports.

import (
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

const (
	acFloorBit uint64 = 1 << 0
	acBallBit  uint64 = 1 << 1
)

// contactRow is the plugin's singleton as a game would search for it.
type contactRow struct {
	Tag      physics.PhysicsSingletonTag
	Contacts physics.ActiveContacts
}

type acSpawnSystem struct{ run func(w *cardinal.World) }

func (s *acSpawnSystem) Run(w *cardinal.World) { s.run(w) }

type acReadSystem struct{ run func(w *cardinal.World) }

func (s *acReadSystem) Run(w *cardinal.World) { s.run(w) }

func TestActiveContacts_SingletonCarriesFilters(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var floor, ball cardinal.EntityID
	w.RegisterSystem(&acSpawnSystem{run: func(w *cardinal.World) {
		if w.TickHeight() != 0 {
			return
		}
		f := w.Create[spawnArchetype]()
		f.Set(harnessTag{Role: "floor"})
		f.Set(physics.Transform2D{})
		f.Set(physics.Velocity2D{})
		f.Set(newRigid(physics.BodyTypeStatic, physics.Box(10, 0.5).Filter(acFloorBit, ^uint64(0))))
		floor = f.ID()

		b := w.Create[spawnArchetype]()
		b.Set(harnessTag{Role: "ball"})
		b.Set(physics.Transform2D{Position: physics.Vec2{Y: 5}})
		b.Set(physics.Velocity2D{})
		b.Set(newRigid(physics.BodyTypeDynamic, physics.Circle(0.5).Filter(acBallBit, ^uint64(0))))
		ball = b.ID()
	}}, cardinal.WithHook(cardinal.Init))

	var (
		last       []physics.ContactPairEntry
		ends       []physics.ContactEndEvent
		killBall   bool
		ballKilled bool
	)
	w.RegisterSystem(&acReadSystem{run: func(w *cardinal.World) {
		last = nil
		for row := range w.Exact[contactRow]().Iter() {
			last = slices.AppendSeq(last, row.Get[physics.ActiveContacts]().Pairs.Values())
		}
		ends = slices.AppendSeq(ends, w.SystemEvents[physics.ContactEndEvent]())
		if killBall {
			killBall = false
			ballKilled = w.Entity(ball).Destroy()
		}
	}}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, 90)
	require.Len(t, last, 1, "the ball rests on the floor")
	catOf := map[cardinal.EntityID]uint64{floor: acFloorBit, ball: acBallBit}
	pair := last[0]
	require.Equal(t, min(floor, ball), pair.EntityA)
	require.Equal(t, max(floor, ball), pair.EntityB)
	require.False(t, pair.IsSensor)
	require.Equal(t, catOf[pair.EntityA], pair.FilterACategoryBits)
	require.Equal(t, catOf[pair.EntityB], pair.FilterBCategoryBits)
	require.Equal(t, ^uint64(0), pair.FilterAMaskBits)
	require.Equal(t, ^uint64(0), pair.FilterBMaskBits)

	// Destroy the ball, then drop the engine: the rebuilt world never sees the ball, so only
	// the persisted pair knows its filter. The End must still report both categories.
	ends = nil
	killBall = true
	tickN(t, w, 1)
	require.True(t, ballKilled, "ball destroyed")
	p.Reset()
	tickN(t, w, 2)
	require.Empty(t, last, "the pair is gone once the End is emitted")
	require.Len(t, ends, 1, "one End for the vanished pair")
	end := ends[0]
	require.True(t, pairHas(end.EntityA, end.EntityB, floor, ball))
	require.Equal(t, catOf[end.EntityA], end.FilterA.CategoryBits, "FilterA from the persisted pair")
	require.Equal(t, catOf[end.EntityB], end.FilterB.CategoryBits, "FilterB from the persisted pair")
}
