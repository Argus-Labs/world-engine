package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
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

// systemMetadata contains the metadata for a system.
type systemMetadata struct {
	name string // The name of the system
	fn   func() // Function that wraps a System
}

func RegisterSystem(world *World, name string, hook SystemHook, fn func()) error {
	switch hook {
	case Init, PreUpdate, Update, PostUpdate:
		assert.That(int(hook) < len(world.systems), "invalid system hook index")
		world.systems[hook] = append(world.systems[hook], systemMetadata{name: name, fn: fn})
	default:
		return eris.Errorf("invalid system hook %d", hook)
	}
	return nil
}

// SystemInfo describes a system for external introspection.
type SystemInfo struct {
	ID   int
	Name string
}

// ScheduleInfo describes the systems for one execution phase.
type ScheduleInfo struct {
	Hook    SystemHook
	Systems []SystemInfo
}
