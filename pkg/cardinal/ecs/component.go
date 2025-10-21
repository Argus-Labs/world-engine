package ecs

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// componentID is a unique identifier for a component type.
// It is used internally to track and manage component types efficiently.
type componentID = uint32

// maxComponentID is the maximum number of components types that can be registered.
const maxComponentID = math.MaxUint32 - 1

// Component is the interface that all components must implement.
// Components are pure data containers that can be attached to entities.
type Component interface { //nolint:iface // We may add more methods in the future.
	// Name returns a unique string identifier for the component type.
	// This should be consistent across program executions.
	Name() string
}

// componentManager manages component type registration and lookup.
type componentManager struct {
	nextID        componentID            // The next available component ID
	registry      map[string]componentID // Component name -> Component ID
	columnFactory []func() any           // Component ID -> Column constructor
}

// newComponentManager creates a new component manager.
func newComponentManager() componentManager {
	return componentManager{
		nextID:        0,
		registry:      make(map[string]componentID),
		columnFactory: make([]func() any, 0),
	}
}

// register registers a new component type. if the component is already registered, no-op.
func (c *componentManager) register(name string, factory func() any) error {
	if name == "" {
		return eris.New("component name cannot be empty")
	}

	if _, exists := c.registry[name]; exists {
		return nil
	}

	if c.nextID > maxComponentID {
		return eris.New("max number of components exceeded")
	}

	// Register new component type.
	c.registry[name] = c.nextID
	c.columnFactory = append(c.columnFactory, factory)
	c.nextID++
	assert.That(int(c.nextID) == len(c.columnFactory), "component id doesn't match number of components")

	return nil
}

// get returns the ID for a given component name.
func (c *componentManager) get(name string) (componentID, error) {
	id, exists := c.registry[name]
	if !exists {
		return 0, eris.Errorf("component %s not registered", name)
	}
	return id, nil
}

// getComponentID returns the ID for a given component.
func (c *componentManager) getComponentID(component Component) (componentID, error) {
	return c.get(component.Name())
}

// toComponentBitmap returns a bitmap of component IDs for the given components.
func (c *componentManager) toComponentBitmap(components []Component) (bitmap.Bitmap, error) {
	var comps bitmap.Bitmap
	for _, component := range components {
		id, err := c.getComponentID(component)
		if err != nil {
			return comps, eris.Wrap(err, "failed to get component ID")
		}
		comps.Set(id)
	}
	return comps, nil
}

// createArchetype creates a new archetype for the given component type IDs. Callers are expected
// to check that all components are registered.
func (c *componentManager) createArchetype(id archetypeID, components bitmap.Bitmap) archetype {
	maxID, ok := components.Max()
	assert.That(ok && maxID < c.nextID, "component not registered") // This should never happen

	// Create the archetype columns.
	i := 0
	columns := make([]any, components.Count())
	components.Range(func(compID uint32) {
		columns[i] = c.columnFactory[compID]()
		i++
	})

	return newArchetype(id, components, columns)
}

// getColumnFactory returns the column factory function for the given component name.
func (c *componentManager) getColumnFactory(name string) (func() any, error) {
	id, exists := c.registry[name]
	if !exists {
		return nil, eris.Errorf("component %s not registered", name)
	}
	return c.columnFactory[id], nil
}
