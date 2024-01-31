package cardinal

import (
	"errors"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"strconv"
)

var (
	ErrEntityMutationOnReadOnly          = errors.New("cannot modify state with read only context")
	ErrEntityDoesNotExist                = iterators.ErrEntityDoesNotExist
	ErrEntityMustHaveAtLeastOneComponent = iterators.ErrEntityMustHaveAtLeastOneComponent
	ErrComponentNotOnEntity              = iterators.ErrComponentNotOnEntity
	ErrComponentAlreadyOnEntity          = iterators.ErrComponentAlreadyOnEntity
	ErrComponentNotRegistered            = iterators.ErrMustRegisterComponent
)

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(eCtx engine.Context, components ...component.Component) (EntityID, error) {
	// Error if the context is read only
	if eCtx.IsReadOnly() {
		return 0, ErrEntityMutationOnReadOnly
	}

	id, err := CreateMany(eCtx, 1, components...)
	if err != nil {
		return 0, logAndPanic(eCtx, err)
	}

	return id[0], nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(eCtx engine.Context, num int, components ...component.Component) ([]EntityID, error) {
	// TODO: uncomment this. use engine state instead.
	// if !eCtx.GetEngine().stateIsLoaded {
	// 		return nil, eris.Wrap(ErrEntitiesCreatedBeforeLoadingGameState, "")
	// }

	// Error if the context is read only
	if eCtx.IsReadOnly() {
		return nil, ErrEntityMutationOnReadOnly
	}

	// Get all component metadata for the given components
	acc := make([]component.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := eCtx.GetComponentByName(comp.Name())
		if err != nil {
			return nil, logAndPanic(eCtx, ErrComponentNotRegistered)
		}
		acc = append(acc, c)
	}

	// Create the entities
	entityIds, err := eCtx.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, logAndPanic(eCtx, err)
	}

	// Set the components for the entities
	for _, id := range entityIds {
		for _, comp := range components {
			var c component.ComponentMetadata
			c, err = eCtx.GetComponentByName(comp.Name())
			if err != nil {
				return nil, logAndPanic(eCtx, ErrComponentNotRegistered)
			}

			err = eCtx.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, logAndPanic(eCtx, err)
			}
		}
	}

	return entityIds, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T component.Component](eCtx engine.Context, id entity.ID, component *T) error {
	// Error if the context is read only
	if eCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := eCtx.GetComponentByName(t.Name())
	if err != nil {
		return logAndPanic(eCtx, err)
	}

	// Set the component
	err = eCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) || eris.Is(err, ErrComponentNotOnEntity) {
			return err
		}
		return logAndPanic(eCtx, err)
	}

	// Log
	eCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")

	return nil
}

// GetComponent returns component data from the entity.
func GetComponent[T component.Component](eCtx engine.Context, id entity.ID) (comp *T, err error) {
	// Get the component metadata
	var t T
	c, err := eCtx.GetComponentByName(t.Name())
	if err != nil {
		return nil, logAndPanic(eCtx, err)
	}

	// Get current component value
	compValue, err := eCtx.StoreReader().GetComponentForEntity(c, id)
	if err != nil {
		if eCtx.IsReadOnly() {
			return nil, err
		}
		if eris.Is(err, ErrEntityDoesNotExist) || eris.Is(err, ErrComponentNotOnEntity) || eris.Is(err,
			ErrComponentNotRegistered) {
			return nil, err
		}
		return nil, logAndPanic(eCtx, err)
	}

	// Type assert the component value to the component type
	t, ok := compValue.(T)
	if !ok {
		comp, ok = compValue.(*T)
		if !ok {
			return nil, logAndPanic(eCtx, eris.Errorf("type assertion for component failed: %v to %v", compValue, c))
		}
	} else {
		comp = &t
	}

	return comp, nil
}

func UpdateComponent[T component.Component](eCtx engine.Context, id entity.ID, fn func(*T) *T) error {
	// Error if the context is read only
	if eCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get current component value
	val, err := GetComponent[T](eCtx, id)
	if err != nil {
		return err // We don't need to panic here because GetComponent will handle the panic if needed for us.
	}

	// Get the new component value
	updatedVal := fn(val)

	// Set the new component value
	err = SetComponent[T](eCtx, id, updatedVal)
	if err != nil {
		return err // We don't need to panic here because SetComponent will handle the panic if needed for us.
	}

	return nil
}

func AddComponentTo[T component.Component](eCtx engine.Context, id entity.ID) error {
	if eCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := eCtx.GetComponentByName(t.Name())
	if err != nil {

		return logAndPanic(eCtx, err)
	}

	// Add the component to entity
	err = eCtx.StoreManager().AddComponentToEntity(c, id)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) || eris.Is(err, ErrComponentAlreadyOnEntity) {
			return err
		}
		return logAndPanic(eCtx, err)
	}

	return nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T component.Component](eCtx engine.Context, id entity.ID) error {
	// Error if the context is read only
	if eCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := eCtx.GetComponentByName(t.Name())
	if err != nil {
		return logAndPanic(eCtx, err)
	}

	// Remove the component from entity
	err = eCtx.StoreManager().RemoveComponentFromEntity(c, id)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) ||
			eris.Is(err, ErrComponentNotOnEntity) ||
			eris.Is(err, ErrEntityMustHaveAtLeastOneComponent) {
			return err
		}
		return logAndPanic(eCtx, err)
	}

	return nil
}

// Remove removes the given Entity from the engine.
func Remove(eCtx engine.Context, id entity.ID) error {
	// Error if the context is read only
	if eCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	err := eCtx.StoreManager().RemoveEntity(id)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) {
			return err
		}
		return logAndPanic(eCtx, err)
	}

	return nil
}
