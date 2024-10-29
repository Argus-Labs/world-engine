package gamestate

import (
	"fmt"
	"reflect"

	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/types"
)

type Reader interface {
	GetComponentForEntity(comp types.Component, id types.EntityID) (any, error)
	GetComponentForEntityInRawJSON(comp types.Component, id types.EntityID) (json.RawMessage, error)
	GetAllComponentsForEntityInRawJSON(id types.EntityID) (map[string]json.RawMessage, error)
	GetEntitiesForArchID(archID types.ArchetypeID) ([]types.EntityID, error)
	FindArchetypes(filter filter.ComponentFilter) ([]types.ArchetypeID, error)
	ArchetypeCount() (int, error)
}

type State struct {
	storage *redis.Storage

	ecb            EntityCommandBuffer
	finalizedState FinalizedState

	nextComponentID types.ComponentID
}

func New(storage *redis.Storage) (*State, error) {
	kv := NewRedisPrimitiveStorage(storage.Client)

	ecb, err := NewEntityCommandBuffer(kv)
	if err != nil {
		return nil, err
	}

	finalizedState, err := NewFinalizedState(kv)
	if err != nil {
		return nil, err
	}

	return &State{
		storage: storage,

		ecb:            *ecb,
		finalizedState: *finalizedState,

		nextComponentID: 1,
	}, nil
}

// RegisteredComponents returns the metadata of all registered components.
func (s *State) RegisteredComponents() []types.ComponentInfo {
	comps := make([]types.ComponentInfo, 0, len(s.ecb.compNameToComponent))
	for _, comp := range s.ecb.compNameToComponent {
		comps = append(comps, types.ComponentInfo{
			Name:   comp.Name(),
			Fields: types.GetFieldInformation(reflect.TypeOf(comp)),
		})
	}
	return comps
}

// RegisterComponent registers component with the component manager.
// There can only be one component with a given name, which is declared by the user by implementing the Name() method.
// If there is a duplicate component name, an error will be returned and the component will not be registered.
func (s *State) RegisterComponent(compMetadata types.ComponentMetadata) error {
	// Check that the component is not already registered
	// Technically, you only need to check one since we always register components together in both ECB and
	// FinalizedState, but we're being extra cautious here.
	if s.ecb.isComponentRegistered(compMetadata.Name()) {
		return eris.Errorf("message %q is already registered", compMetadata.Name())
	}
	if s.finalizedState.isComponentRegistered(compMetadata.Name()) {
		return eris.Errorf("message %q is already registered", compMetadata.Name())
	}

	// Try getting the schema from storage
	// If the error is simply the schema not existing yet in storage, we can safely proceed.
	// However, if it is a different error, we need to terminate and return the error.
	storedSchema, err := s.storage.SchemaStorage.GetSchema(compMetadata.Name())
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
		if err := s.storage.SchemaStorage.SetSchema(compMetadata.Name(), compMetadata.GetSchema()); err != nil {
			return err
		}
	}

	// Set the component ID and register the component.
	// We do this after the schema validation and storage operations to ensure that the component is only registered
	// if the schema validation and storage operations are successful.
	if err := compMetadata.SetID(s.nextComponentID); err != nil {
		return err
	}

	if err := s.ecb.registerComponent(compMetadata); err != nil {
		return err
	}
	if err := s.finalizedState.registerComponent(compMetadata); err != nil {
		return err
	}

	s.nextComponentID++

	return nil
}

// Init marks the state as ready for use. This prevents any new components from being registered.
func (s *State) Init() error {
	if err := s.ecb.init(); err != nil {
		return err
	}

	if err := s.finalizedState.init(); err != nil {
		return err
	}

	return nil
}

func (s *State) ECB() *EntityCommandBuffer {
	return &s.ecb
}

func (s *State) FinalizedState() *FinalizedState {
	return &s.finalizedState
}
