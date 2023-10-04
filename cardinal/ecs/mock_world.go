package ecs

import (
	"fmt"
	"log"
	"pkg.world.dev/world-engine/cardinal/engine"
	storage2 "pkg.world.dev/world-engine/cardinal/engine/storage"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

// NewMockWorld creates an ecs.World that uses a mock redis DB as the storage
// layer. This is only suitable for local development. If you are creating an ecs.World for
// unit tests, use NewTestWorld.
func NewMockWorld(opts ...engine.Option) *World {
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
func NewTestWorld(t testing.TB, opts ...engine.Option) *World {
	s := miniredis.RunT(t)
	w, err := newMockWorld(s, opts...)
	if err != nil {
		t.Fatalf("Unable to initialize world: %v", err)
	}
	return w
}

func newMockWorld(s *miniredis.Miniredis, opts ...engine.Option) (*World, error) {
	rs := storage2.NewRedisStorage(storage2.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "in-memory-world")
	worldStorage := storage2.NewWorldStorage(&rs)

	return NewWorld(worldStorage, opts...)
}
