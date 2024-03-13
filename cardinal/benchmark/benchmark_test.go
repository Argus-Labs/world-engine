// Package benchmark_test contains benchmarks that were initially used to compare the performance between different data
// recovery methods (snapshotting all redis keys vs the entity-command-buffer).
package benchmark_test

import (
	"fmt"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/assert"
)

type Health struct {
	Value int
}

func (Health) Name() string {
	return "health"
}

// setupWorld Creates a new *cardinal.World and initializes the world to have numOfEntities already cardinal.Created. If
// enableHealthSystem is set, a System will be added to the world that increments every entity's "health" by 1 every
// tick.
func setupWorld(t testing.TB, numOfEntities int, enableHealthSystem bool) *testutils.TestFixture {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	zerolog.SetGlobalLevel(zerolog.Disabled)

	if enableHealthSystem {
		err := cardinal.RegisterSystems(
			world,
			func(wCtx engine.Context) error {
				q := cardinal.NewSearch(wCtx, filter.Contains(Health{}))
				err := q.Each(
					func(id types.EntityID) bool {
						health, err := cardinal.GetComponent[Health](wCtx, id)
						assert.NilError(t, err)
						health.Value++
						assert.NilError(t, cardinal.SetComponent[Health](wCtx, id, health))
						return true
					},
				)
				assert.NilError(t, err)
				return nil
			},
		)
		assert.NilError(t, err)
	}

	assert.NilError(t, cardinal.RegisterComponent[Health](world))

	tf.StartWorld()

	_, err := cardinal.CreateMany(cardinal.NewWorldContext(world), numOfEntities, Health{})
	assert.NilError(t, err)

	// Perform a game tick to ensure the newly created entities have been committed to the DB
	tf.DoTick()

	return tf
}

func BenchmarkWorld_TickNoSystems(b *testing.B) {
	maxEntities := 10000
	for i := 1; i <= maxEntities; i *= 10 {
		tf := setupWorld(b, i, false)
		name := fmt.Sprintf("%d entities", i)
		b.Run(name, func(b *testing.B) {
			for j := 0; j < b.N; j++ {
				tf.DoTick()
			}
		})
	}
}

func BenchmarkWorld_TickWithSystem(b *testing.B) {
	maxEntities := 10000
	for i := 1; i <= maxEntities; i *= 10 {
		tf := setupWorld(b, i, true)
		name := fmt.Sprintf("%d entities", i)
		b.Run(
			name, func(b *testing.B) {
				for j := 0; j < b.N; j++ {
					tf.DoTick()
				}
			},
		)
	}
}
