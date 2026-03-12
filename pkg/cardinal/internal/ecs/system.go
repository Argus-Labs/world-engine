package ecs

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// SystemHook defines when a system should be executed in the update cycle.
type SystemHook uint8

const (
	// PreUpdate runs before the main update.
	PreUpdate SystemHook = 0
	// Update runs during the main update phase.
	Update SystemHook = 1
	// PostUpdate runs after the main update.
	PostUpdate SystemHook = 2
	// Init runs once during world initialization.
	Init SystemHook = 3
)

// initSystem represents a system that should be run once during world initialization.
type initSystem struct {
	name string // The name of the system
	fn   func() // Function that wraps a System
}

func RegisterSystem[T any](world *World, options RegisterSystemOptions[T]) error {
	// TODO: maybe separate these so this error can never happen.
	// Add system event deps to component deps.
	deps := options.DepsComponent.Clone(nil)
	n := world.state.components.nextID
	assert.That(options.DepsSystemEvent.Count()+int(n) <= math.MaxUint32-1, "system dependencies exceed max limit")
	options.DepsSystemEvent.Range(func(x uint32) {
		deps.Set(n + x)
	})

	hook := options.Hook
	switch hook {
	case Init:
		world.initSystems = append(world.initSystems, initSystem{name: options.Name, fn: options.System})
	case PreUpdate, Update, PostUpdate:
		world.scheduler[hook].register(options.Name, deps, options.System)
	default:
		return eris.Errorf("invalid system hook %d", hook)
	}

	return nil
}

type RegisterSystemOptions[T any] struct {
	Name            string
	State           *T
	System          func()
	Hook            SystemHook
	DepsComponent   bitmap.Bitmap
	DepsSystemEvent bitmap.Bitmap
}
