package ecs

import (
	"fmt"
	"log"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"pkg.world.dev/world-engine/cardinal/ecs/options"
	"pkg.world.dev/world-engine/cardinal/ecs/world"
	"pkg.world.dev/world-engine/cardinal/public"

	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

// NewMockWorld creates an ecs.World that uses a mock redis DB as the storage
// layer. This is only suitable for local development. If you are creating an ecs.World for
// unit tests, use NewTestWorld.
func NewMockWorld(opts ...options.Option) public.IWorld {
	// We manually set the start address to make the port deterministic
	s := miniredis.NewMiniRedis()
	err := s.StartAddr(":12345")
	if err != nil {
		panic("Unable to initialize in-memory redis")
	}
	log.Printf("Miniredis started at %s", s.Addr())

	w, err := newMockWorld(s, opts...)
	if err != nil {
		panic(fmt.Errorf("unable to initialize world: %w", err))
	}

	return w
}

// NewTestWorld creates an ecs.World suitable for running in tests. Relevant resources
// are automatically cleaned up at the completion of each test.
func NewTestWorld(t testing.TB, opts ...options.Option) public.IWorld {
	s := miniredis.RunT(t)
	w, err := newMockWorld(s, opts...)
	if err != nil {
		t.Fatalf("Unable to initialize world: %v", err)
	}
	return w
}

func newMockWorld(s *miniredis.Miniredis, opts ...options.Option) (public.IWorld, error) {
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "in-memory-world")
	worldStorage := storage.NewWorldStorage(&rs)

	return world.NewWorld(worldStorage, opts...)
}
