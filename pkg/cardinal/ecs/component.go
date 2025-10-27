package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// Component is the interface that all components must implement.
// Components are pure data containers that can be attached to entities.
type Component interface { //nolint:iface // We may add more methods in the future.
	// Name returns a unique string identifier for the component type.
	// This should be consistent across program executions.
	Name() string
}

// componentID is a unique identifier for a component type.
// It is used internally to track and manage component types efficiently.
type componentID = uint32

// componentManager manages component type registration and lookup.
type componentManager struct {
	nextID    componentID            // The next available component ID
	catalog   map[string]componentID // Component name -> component ID
	factories []columnFactory        // Component ID -> column factory
}

// newComponentManager creates a new component manager.
func newComponentManager() componentManager {
	return componentManager{
		nextID:    0,
		catalog:   make(map[string]componentID),
		factories: make([]columnFactory, 0),
	}
}

// register registers a new component type and returns its ID.
// If the component is already registered, no-op.
func (cm *componentManager) register(name string, factory columnFactory) (componentID, error) {
	if name == "" {
		return 0, eris.New("component name cannot be empty")
	}

	// If component already exists, no-op.
	if cid, exists := cm.catalog[name]; exists {
		return cid, nil
	}

	cm.catalog[name] = cm.nextID
	cm.factories = append(cm.factories, factory)
	cm.nextID++
	assert.That(int(cm.nextID) == len(cm.factories), "component id doesn't match number of components")

	return cm.nextID - 1, nil
}

// getID returns a component's ID given a name.
func (cm *componentManager) getID(name string) (componentID, error) {
	id, exists := cm.catalog[name]

	if !exists {
		return 0, eris.Wrapf(ErrComponentNotFound, "component %s", name)
	}

	return id, nil
}
