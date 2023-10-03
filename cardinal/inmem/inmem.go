// Package inmem is a helper package that allows for the creation of an *ecs.World object
// that uses an in-memory redis DB as the storage layer. This is useful for local development
// or for tests. Data will not be persisted between runs, so this is not suitable for any
// kind of prodcution or staging environemnts.
package inmem

import (
	"fmt"
	"log"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/public"
	"pkg.world.dev/world-engine/cardinal/storage"
)

// NewECSWorld creates an ecs.World that uses an in-memory redis DB as the storage
// layer. This is only suitable for local development. If you are creating an ecs.World for
// unit tests, use NewECSWorldForTest.
func NewECSWorld(opts ...ecs.Option) public.IWorld {
	// We manually set the start address to make the port deterministic
	s := miniredis.NewMiniRedis()
	err := s.StartAddr(":12345")
	if err != nil {
		panic("Unable to initialize in-memory redis")
	}
	log.Printf("Miniredis started at %s", s.Addr())

	w, err := newInMemoryWorld(s, opts...)
	if err != nil {
		panic(fmt.Sprintf("Unable to initialize world: %v", err))
	}

	return w
}

// NewECSWorldForTest creates an ecs.World suitable for running in tests. Relevant resources
// are automatically cleaned up at the completion of each test.
func NewECSWorldForTest(t testing.TB, opts ...ecs.Option) public.IWorld {
	s := miniredis.RunT(t)
	w, err := newInMemoryWorld(s, opts...)
	if err != nil {
		t.Fatalf("Unable to initialize world: %v", err)
	}
	return w
}

func newInMemoryWorld(s *miniredis.Miniredis, opts ...ecs.Option) (public.IWorld, error) {
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "in-memory-world")
	worldStorage := storage.NewWorldStorage(&rs)

	return ecs.NewWorld(worldStorage, opts...)
}
