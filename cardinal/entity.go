package cardinal

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(eCtx engine.Context, num int, components ...component.Component) ([]EntityID, error) {
	ids, err := ecs.CreateMany(eCtx, num, components...)
	if eCtx.IsReadOnly() || err == nil {
		return ids, err
	}
	return nil, logAndPanic(eCtx, err)
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(eCtx engine.Context, components ...component.Component) (EntityID, error) {
	id, err := ecs.Create(eCtx, components...)
	if eCtx.IsReadOnly() || err == nil {
		return id, err
	}
	return 0, logAndPanic(eCtx, err)
}

// SetComponent Set sets component data to the entity.
func SetComponent[T component.Component](eCtx engine.Context, id entity.ID, comp *T) error {
	err := ecs.SetComponent[T](eCtx, id, comp)
	if eCtx.IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) {
		return err
	}
	return logAndPanic(eCtx, err)
}

// GetComponent Get returns component data from the entity.
func GetComponent[T component.Component](eCtx engine.Context, id entity.ID) (*T, error) {
	result, err := ecs.GetComponent[T](eCtx, id)
	_ = result
	if eCtx.IsReadOnly() || err == nil {
		return result, err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) {
		return nil, err
	}

	return nil, logAndPanic(eCtx, err)
}

// UpdateComponent Updates a component on an entity.
func UpdateComponent[T component.Component](eCtx engine.Context, id entity.ID, fn func(*T) *T) error {
	err := ecs.UpdateComponent[T](eCtx, id, fn)
	if eCtx.IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) {
		return err
	}

	return logAndPanic(eCtx, err)
}

// AddComponentTo Adds a component on an entity.
func AddComponentTo[T component.Component](eCtx engine.Context, id entity.ID) error {
	err := ecs.AddComponentTo[T](eCtx, id)
	if eCtx.IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentAlreadyOnEntity) {
		return err
	}

	return logAndPanic(eCtx, err)
}

// RemoveComponentFrom Removes a component from an entity.
func RemoveComponentFrom[T component.Component](eCtx engine.Context, id entity.ID) error {
	err := ecs.RemoveComponentFrom[T](eCtx, id)
	if eCtx.IsReadOnly() || err == nil {
		return err
	}
	if eris.Is(err, ErrEntityDoesNotExist) ||
		eris.Is(err, ErrComponentNotOnEntity) ||
		eris.Is(err, ErrEntityMustHaveAtLeastOneComponent) {
		return err
	}
	return logAndPanic(eCtx, err)
}

// Remove removes the given entity id from the world.
func Remove(eCtx engine.Context, id EntityID) error {
	return ecs.Remove(eCtx, id)
}
