package component

import (
	"fmt"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/types"
)

var ErrComponentNotRegistered = eris.New("component not registered")

type Manager struct {
	registeredComponents map[string]types.ComponentMetadata
	nextComponentID      types.ComponentID
	schemaStorage        SchemaStorage
}

// NewManager creates a new component manager.
func NewManager(schemaStorage SchemaStorage) *Manager {
	return &Manager{
		registeredComponents: make(map[string]types.ComponentMetadata),
		nextComponentID:      1,
		schemaStorage:        schemaStorage,
	}
}

// RegisterComponent registers component with the component manager.
// There can only be one component with a given name, which is declared by the user by implementing the Name() method.
// If there is a duplicate component name, an error will be returned and the component will not be registered.
func (m *Manager) RegisterComponent(compMetadata types.ComponentMetadata) error {
	// Check that the component is not already registered
	if err := m.isComponentNameUnique(compMetadata); err != nil {
		return err
	}

	// Try getting the schema from storage
	// If the error is simply the schema not existing yet in storage, we can safely proceed.
	// However, if it is a different error, we need to terminate and return the error.
	storedSchema, err := m.schemaStorage.GetSchema(compMetadata.Name())
	if err != nil && !eris.Is(err, redis.ErrNoSchemaFound) {
		return err
	}

	//nolint:nestif // Comments for nested if statements provided for clarity
	if storedSchema != nil {
		// If there is a schema stored in storage, check if it matches the current schema of the component.
		// If it does not match or schema validation failed, return an error.
		// If it does match, our job here is done.
		if err := compMetadata.ValidateAgainstSchema(storedSchema); err != nil {
			if eris.Is(err, types.ErrComponentSchemaMismatch) {
				return eris.Wrap(err,
					fmt.Sprintf("component %q does not match the schema stored in storage", compMetadata.Name()),
				)
			}
			return eris.Wrap(err, "error when validating component schema against stored schema in storage")
		}
	} else {
		// If there is no schema stored in storage, store the schema of the component in storage.
		if err := m.schemaStorage.SetSchema(compMetadata.Name(), compMetadata.GetSchema()); err != nil {
			return err
		}
	}

	// Set the component ID and register the component.
	// We do this after the schema validation and storage operations to ensure that the component is only registered
	// if the schema validation and storage operations are successful.
	if err := compMetadata.SetID(m.nextComponentID); err != nil {
		return err
	}
	m.registeredComponents[compMetadata.Name()] = compMetadata
	m.nextComponentID++

	return nil
}

// GetComponents returns a list of all registered components.
// Note: The order of the components in the list is not deterministic.
func (m *Manager) GetComponents() []types.ComponentMetadata {
	registeredComponents := make([]types.ComponentMetadata, 0, len(m.registeredComponents))
	for _, comp := range m.registeredComponents {
		registeredComponents = append(registeredComponents, comp)
	}
	return registeredComponents
}

// GetComponentByName returns the component metadata for the given component name.
func (m *Manager) GetComponentByName(name string) (types.ComponentMetadata, error) {
	c, ok := m.registeredComponents[name]
	if !ok {
		return nil, eris.Wrap(ErrComponentNotRegistered, fmt.Sprintf("component %q is not registered", name))
	}
	return c, nil
}

// isComponentNameUnique checks if the component name already exist in component map.
func (m *Manager) isComponentNameUnique(compMetadata types.ComponentMetadata) error {
	_, ok := m.registeredComponents[compMetadata.Name()]
	if ok {
		return eris.Errorf("message %q is already registered", compMetadata.Name())
	}
	return nil
}
