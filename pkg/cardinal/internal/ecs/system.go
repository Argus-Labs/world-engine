package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// Phase is the stage of the update cycle a system runs in.
type Phase uint8

const (
	// PreUpdate runs before the main update.
	PreUpdate Phase = 0
	// Update runs during the main update phase.
	Update Phase = 1
	// PostUpdate runs after the main update.
	PostUpdate Phase = 2
	// Init runs once during world initialization.
	Init Phase = 3
)

// systemMetadata contains the metadata for a system.
type systemMetadata struct {
	name string // The name of the system
	fn   func() // Function that wraps a System
}

func (w *World) RegisterSystem(name string, phase Phase, fn func()) error {
	switch phase {
	case Init, PreUpdate, Update, PostUpdate:
		assert.That(int(phase) < len(w.systems), "invalid system phase index")
		w.systems[phase] = append(w.systems[phase], systemMetadata{name: name, fn: fn})
	default:
		return eris.Errorf("invalid system phase %d", phase)
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
	Phase   Phase
	Systems []SystemInfo
}
