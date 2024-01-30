package ecs

import (
	"strconv"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

func Create(eCtx engine.Context, components ...component.Component) (entity.ID, error) {
	entities, err := CreateMany(eCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}

func CreateMany(eCtx engine.Context, num int, components ...component.Component) ([]entity.ID, error) {
	// TODO: move this up
	//if !eCtx.GetEngine().stateIsLoaded {
	//	return nil, eris.Wrap(ErrEntitiesCreatedBeforeLoadingGameState, "")
	//}
	if eCtx.IsReadOnly() {
		return nil, eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	acc := make([]component.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := eCtx.GetComponentByName(comp.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	entityIds, err := eCtx.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, err
	}
	for _, id := range entityIds {
		for _, comp := range components {
			var c component.ComponentMetadata
			c, err = eCtx.GetComponentByName(comp.Name())
			if err != nil {
				return nil, eris.Wrap(err, "must register component before creating an entity")
			}
			err = eCtx.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}
	return entityIds, nil
}

// Remove removes the given Entity from the engine.
func Remove(eCtx engine.Context, id entity.ID) error {
	return eCtx.StoreManager().RemoveEntity(id)
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T component.Component](eCtx engine.Context, id entity.ID) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	var t T
	name := t.Name()
	c, err := eCtx.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "must register component")
	}
	return eCtx.StoreManager().RemoveComponentFromEntity(c, id)
}

func AddComponentTo[T component.Component](eCtx engine.Context, id entity.ID) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	var t T
	name := t.Name()
	c, err := eCtx.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "must register component")
	}
	return eCtx.StoreManager().AddComponentToEntity(c, id)
}

// GetComponent returns component data from the entity.
func GetComponent[T component.Component](eCtx engine.Context, id entity.ID) (comp *T, err error) {
	var t T
	name := t.Name()
	c, err := eCtx.GetComponentByName(name)
	if err != nil {
		return nil, eris.Wrap(err, "must register component")
	}
	value, err := eCtx.StoreReader().GetComponentForEntity(c, id)
	if err != nil {
		return nil, err
	}
	t, ok := value.(T)
	if !ok {
		comp, ok = value.(*T)
		if !ok {
			return nil, eris.Errorf("type assertion for component failed: %v to %v", value, c)
		}
	} else {
		comp = &t
	}

	return comp, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T component.Component](eCtx engine.Context, id entity.ID, component *T) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	var t T
	name := t.Name()
	c, err := eCtx.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "get component by name failed")
	}
	err = eCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return eris.Wrap(err, "set component failed")
	}
	eCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

func UpdateComponent[T component.Component](eCtx engine.Context, id entity.ID, fn func(*T) *T) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	val, err := GetComponent[T](eCtx, id)
	if err != nil {
		return err
	}
	updatedVal := fn(val)
	return SetComponent[T](eCtx, id, updatedVal)
}
