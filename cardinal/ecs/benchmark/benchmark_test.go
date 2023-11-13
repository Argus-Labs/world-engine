// Package benchmark_test contains benchmarks that were initially used to compare the performance between different data
// recovery methods (snapshotting all redis keys vs the entity-command-buffer).
package benchmark_test

import (
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/ecb"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

// newWorldWithRealRedis returns an *ecs.World that is connected to a redis DB hosted at localhost:6379. The target
// database is CLEARED OF ALL DATA so that the *ecs.World object can start from a clean slate.
func newWorldWithRealRedis(t testing.TB) *ecs.World {
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	}, "real-world")
	assert.NilError(t, rs.Client.FlushDB(context.Background()).Err())

	sm, err := ecb.NewManager(rs.Client)
	assert.NilError(t, err)
	world, err := ecs.NewWorld(&rs, sm)

	assert.NilError(t, err)
	return world
}

type Health struct {
	Value int
}

func (Health) Name() string {
	return "health"
}

// setupWorld creates a new *ecs.World and initializes the world to have numOfEntities already created. If
// enableHealthSystem is set, a System will be added to the world that increments every entity's "health" by 1 every
// tick.
func setupWorld(t testing.TB, numOfEntities int, enableHealthSystem bool) *ecs.World {
	world := newWorldWithRealRedis(t)
	zerolog.SetGlobalLevel(zerolog.Disabled)
	if enableHealthSystem {
		world.RegisterSystem(func(wCtx ecs.WorldContext) error {
			q, err := world.NewSearch(ecs.Contains(Health{}))
			assert.NilError(t, err)
			err = q.Each(wCtx, func(id entity.ID) bool {
				health, err := component.GetComponent[Health](wCtx, id)
				assert.NilError(t, err)
				health.Value++
				assert.NilError(t, component.SetComponent[Health](wCtx, id, health))
				return true
			})
			assert.NilError(t, err)
			return nil
		})
	}

	assert.NilError(t, ecs.RegisterComponent[Health](world))
	assert.NilError(t, world.LoadGameState())
	_, err := component.CreateMany(ecs.NewWorldContext(world), numOfEntities, Health{})
	assert.NilError(t, err)
	// Perform a game tick to ensure the newly created entities have been committed to the DB
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
		b.Run(name, func(b *testing.B) {
			for j := 0; j < b.N; j++ {
				assert.NilError(b, world.Tick(context.Background()))
			}
		})
	}
}

func BenchmarkWorld_TickWithSystem(b *testing.B) {
	maxEntities := 10000
	enableHealthSystem := true

	for i := 1; i <= maxEntities; i *= 10 {
		world := setupWorld(b, i, enableHealthSystem)
		name := fmt.Sprintf("%d entities", i)
		b.Run(name, func(b *testing.B) {
			for j := 0; j < b.N; j++ {
				assert.NilError(b, world.Tick(context.Background()))
			}
		})
	}
}
