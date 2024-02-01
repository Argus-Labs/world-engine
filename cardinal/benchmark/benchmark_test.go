// Package benchmark_test contains benchmarks that were initially used to compare the performance between different data
// recovery methods (snapshotting all redis keys vs the entity-command-buffer).
package benchmark_test

import (
	"context"
	"fmt"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"testing"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// newWorldWithRealRedis returns a *cardinal.World that is connected to a redis DB hosted at localhost:6379. The target
// database is CLEARED OF ALL DATA so that the *cardinal.World object can start from a clean slate.
func newWorldWithRealRedis(t testing.TB) *cardinal.World {
	world, err := cardinal.NewWorld()
	assert.NilError(t, err)
	return world
}

type Health struct {
	Value int
}

func (Health) Name() string {
	return "health"
}

// setupWorld Creates a new *cardinal.World and initializes the world to have numOfEntities already cardinal.Created. If
// enableHealthSystem is set, a System will be added to the world that increments every entity's "health" by 1 every
// tick.
func setupWorld(t testing.TB, numOfEntities int, enableHealthSystem bool) *cardinal.World {
	world := newWorldWithRealRedis(t)
	zerolog.SetGlobalLevel(zerolog.Disabled)
	if enableHealthSystem {
		err := cardinal.RegisterSystems(
			world,
			func(eCtx engine.Context) error {
				q := cardinal.NewSearch(eCtx, filter.Contains(Health{}))
				err := q.Each(
					func(id entity.ID) bool {
						health, err := cardinal.GetComponent[Health](eCtx, id)
						assert.NilError(t, err)
						health.Value++
						assert.NilError(t, cardinal.SetComponent[Health](eCtx, id, health))
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
	assert.NilError(t, world.LoadGameState())
	_, err := cardinal.CreateMany(cardinal.NewWorldContext(world), numOfEntities, Health{})
	assert.NilError(t, err)
	// Perform a game tick to ensure the newly cardinal.Created entities have been committed to the DB
	ctx := context.Background()
	assert.NilError(t, world.Tick(ctx))
	return world
}

func BenchmarkWorld_TickNoSystems(b *testing.B) {
	maxEntities := 10000
	enableHealthSystem := false

	for i := 1; i <= maxEntities; i *= 10 {
		world := setupWorld(b, i, enableHealthSystem)
		name := fmt.Sprintf("%d entities", i)
		b.Run(
			name, func(b *testing.B) {
				for j := 0; j < b.N; j++ {
					assert.NilError(b, world.Tick(context.Background()))
				}
			},
		)
	}
}

func BenchmarkWorld_TickWithSystem(b *testing.B) {
	maxEntities := 10000
	enableHealthSystem := true

	for i := 1; i <= maxEntities; i *= 10 {
		world := setupWorld(b, i, enableHealthSystem)
		name := fmt.Sprintf("%d entities", i)
		b.Run(
			name, func(b *testing.B) {
				for j := 0; j < b.N; j++ {
					assert.NilError(b, world.Tick(context.Background()))
				}
			},
		)
	}
}
