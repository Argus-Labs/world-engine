package ecs

import (
	"math"
	"reflect"
	"regexp"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/rotisserie/eris"
)

// Component is the interface that all components must implement.
// Components are pure data containers that can be attached to entities.
type Component interface { //nolint:iface // the wire contract lives on schema.Serializable; this names the ECS role
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
	schema.Serializable
}

// ComponentID is a unique identifier for a component type.
// It is used internally to track and manage component types efficiently.
type ComponentID = uint32

// maxComponentID is the maximum number of component types that can be registered.
const maxComponentID = math.MaxUint32 - 1

// componentManager manages component type registration and lookup.
type componentManager struct {
	nextID    ComponentID            // The next available component ID
	catalog   map[string]ComponentID // Component name -> component ID
	factories []columnFactory        // Component ID -> column factory
	names     []string               // Component ID -> name
	types     []reflect.Type         // Component ID -> registered Go type
}

// newComponentManager creates a new component manager.
func newComponentManager() componentManager {
	return componentManager{
		nextID:    0,
		catalog:   make(map[string]ComponentID),
		factories: make([]columnFactory, 0),
		names:     make([]string, 0),
		types:     make([]reflect.Type, 0),
	}
}

var componentNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateComponentName validates that a component name follows expr identifier rules.
// See: https://expr-lang.org/docs/language-definition#variables
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
// Registering the same type again is a no-op. Reusing a name for another type returns an error.
func (cm *componentManager) register[T Component](name string) (ComponentID, error) {
	// Validate component name follows expr identifier rules
	if err := validateComponentName(name); err != nil {
		return 0, err
	}

	if cid, exists := cm.catalog[name]; exists {
		if cm.types[cid] != reflect.TypeFor[T]() {
			return 0, eris.Errorf("component %s already registered with a different type", name)
		}
		return cid, nil
	}

	if cm.nextID > maxComponentID {
		return 0, eris.New("max number of components exceeded")
	}

	cm.catalog[name] = cm.nextID
	cm.factories = append(cm.factories, newColumnFactory[T]())
	cm.names = append(cm.names, name)
	cm.types = append(cm.types, reflect.TypeFor[T]())
	cm.nextID++
	assert.That(int(cm.nextID) == len(cm.factories), "component id doesn't match number of components")

	return cm.nextID - 1, nil
}

// getID returns a component's ID given a name.
func (cm *componentManager) getID(name string) (ComponentID, error) {
	id, exists := cm.catalog[name]

	if !exists {
		return 0, eris.Wrapf(ErrComponentNotFound, "component %s", name)
	}

	return id, nil
}

// lookup returns the ID of the registered component with the given name and Go type, or an
// error if it was never registered or its name is registered with a different type.
func (cm *componentManager) lookup(name string, typ reflect.Type) (ComponentID, error) {
	cid, err := cm.getID(name)
	if err != nil {
		return 0, err
	}
	if cm.types[cid] != typ {
		return 0, eris.Wrapf(ErrComponentNotFound, "component %s is registered with a different type", name)
	}
	return cid, nil
}

// RegisterComponent registers a component type with the world.
func (w *World) RegisterComponent[T Component]() (ComponentID, error) {
	var zero T
	if w.onComponentRegister != nil {
		if err := w.onComponentRegister(zero); err != nil {
			return 0, eris.Wrap(err, "component registered callback failed")
		}
	}
	return w.state.components.register[T](zero.Name())
}

// ComponentID returns the ID of a component type registered with RegisterComponent.
func (w *World) ComponentID[T Component]() (ComponentID, error) {
	var zero T
	return w.state.components.lookup(zero.Name(), reflect.TypeFor[T]())
}

// ComponentIDOf returns the ID of the registered component with c's name and dynamic type.
// It serves callers that hold a component value rather than a type parameter.
func (w *World) ComponentIDOf(c Component) (ComponentID, error) {
	return w.state.components.lookup(c.Name(), reflect.TypeOf(c))
}
