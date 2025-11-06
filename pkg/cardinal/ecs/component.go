package ecs

import (
	"regexp"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/rotisserie/eris"
)

// Component is the interface that all components must implement.
// Components are pure data containers that can be attached to entities.
type Component interface { //nolint:iface // We may add more methods in the future.
	// Name returns a unique string identifier for the component type.
	// This should be consistent across program executions.
	//
	// Component names must follow these rules:
	//   - Start with a letter (a-z, A-Z) or underscore (_)
	//   - Contain only letters, digits (0-9), and underscores
	//   - Cannot contain hyphens (-), spaces, dots (.), or other special characters
	//
	// Valid examples: "Health", "PlayerData", "player_health", "_internal", "Component123"
	// Invalid examples: "player-data", "123Invalid", "my.component", "has space"
	//
	// These rules ensure component names work correctly in query expressions.
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

var componentNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateComponentName validates that a component name follows expr identifier rules.
//
// Per expr language specification (https://expr-lang.org/docs/language-definition#variables):
// "The variable name must start with a letter or an underscore. The variable name can contain
// letters, digits and underscores."
func validateComponentName(name string) error {
	if name == "" {
		return eris.New("component name cannot be empty")
	}

	if !componentNamePattern.MatchString(name) {
		return eris.Errorf(
			"component name '%s' is invalid: must start with a letter or underscore, "+
				"and contain only letters, digits, and underscores",
			name,
		)
	}

	return nil
}

// register registers a new component type and returns its ID.
// If the component is already registered, no-op.
func (cm *componentManager) register(name string, factory columnFactory) (componentID, error) {
	// Validate component name follows expr identifier rules
	if err := validateComponentName(name); err != nil {
		return 0, err
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
