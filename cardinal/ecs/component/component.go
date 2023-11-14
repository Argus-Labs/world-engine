package component

import (
	"errors"
	"fmt"
	"strconv"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

func Create(wCtx ecs.WorldContext, components ...metadata.Component) (entity.ID, error) {
	entities, err := CreateMany(wCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}

func CreateMany(wCtx ecs.WorldContext, num int, components ...metadata.Component) ([]entity.ID, error) {
	if wCtx.IsReadOnly() {
		return nil, ecs.ErrCannotModifyStateWithReadOnlyContext
	}
	world := wCtx.GetWorld()
	acc := make([]metadata.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := world.GetComponentByName(comp.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	entityIds, err := world.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, err
	}
	for _, id := range entityIds {
		for _, comp := range components {
			var c metadata.ComponentMetadata
			c, err = world.GetComponentByName(comp.Name())
			if err != nil {
				return nil, errors.New("must register component before creating an entity")
			}
			err = world.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}
	wCtx.GetWorld().SetEntitiesCreated(true)
	return entityIds, nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T metadata.Component](wCtx ecs.WorldContext, id entity.ID) error {
	if wCtx.IsReadOnly() {
		return ecs.ErrCannotModifyStateWithReadOnlyContext
	}
	w := wCtx.GetWorld()
	var t T
	name := t.Name()
	c, err := w.GetComponentByName(name)
	if err != nil {
		return errors.New("must register component")
	}
	return w.StoreManager().RemoveComponentFromEntity(c, id)
}

func AddComponentTo[T metadata.Component](wCtx ecs.WorldContext, id entity.ID) error {
	if wCtx.IsReadOnly() {
		return ecs.ErrCannotModifyStateWithReadOnlyContext
	}
	w := wCtx.GetWorld()
	var t T
	name := t.Name()
	c, err := w.GetComponentByName(name)
	if err != nil {
		return errors.New("must register component")
	}
	return w.StoreManager().AddComponentToEntity(c, id)
}

// GetComponent returns component data from the entity.
func GetComponent[T metadata.Component](wCtx ecs.WorldContext, id entity.ID) (comp *T, err error) {
	var t T
	name := t.Name()
	c, err := wCtx.GetWorld().GetComponentByName(name)
	if err != nil {
		return nil, errors.New("must register component")
	}
	value, err := wCtx.StoreReader().GetComponentForEntity(c, id)
	if err != nil {
		return nil, err
	}
	t, ok := value.(T)
	if !ok {
		comp, ok = value.(*T)
		if !ok {
			return nil, fmt.Errorf("type assertion for component failed: %v to %v", value, c)
		}
	} else {
		comp = &t
	}

	return comp, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T metadata.Component](wCtx ecs.WorldContext, id entity.ID, component *T) error {
	if wCtx.IsReadOnly() {
		return ecs.ErrCannotModifyStateWithReadOnlyContext
	}
	var t T
	name := t.Name()
	c, err := wCtx.GetWorld().GetComponentByName(name)
	if err != nil {
		return fmt.Errorf("%s is not registered, please register it before updating", t.Name())
	}
	err = wCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return err
	}
	wCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

func UpdateComponent[T metadata.Component](wCtx ecs.WorldContext, id entity.ID, fn func(*T) *T) error {
	if wCtx.IsReadOnly() {
		return ecs.ErrCannotModifyStateWithReadOnlyContext
	}
	val, err := GetComponent[T](wCtx, id)
	if err != nil {
		return err
	}
	updatedVal := fn(val)
	return SetComponent[T](wCtx, id, updatedVal)
}
