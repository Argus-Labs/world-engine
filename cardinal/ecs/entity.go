package ecs

import (
	"pkg.world.dev/world-engine/cardinal"
	"strconv"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

func Create(wCtx cardinal.WorldContext, components ...component.Component) (entity.ID, error) {
	entities, err := CreateMany(wCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}

func CreateMany(wCtx cardinal.WorldContext, num int, components ...component.Component) ([]entity.ID, error) {
	if wCtx.IsReadOnly() {
		return nil, eris.Wrap(cardinal.ErrCannotModifyStateWithReadOnlyContext, "")
	}
	engine := wCtx.GetEngine()
	acc := make([]component.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := engine.GetComponentByName(comp.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	entityIds, err := engine.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, err
	}
	for _, id := range entityIds {
		for _, comp := range components {
			var c component.ComponentMetadata
			c, err = engine.GetComponentByName(comp.Name())
			if err != nil {
				return nil, eris.Wrap(err, "must register component before creating an entity")
			}
			err = engine.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}
	wCtx.GetEngine().SetEntitiesCreated(true)
	return entityIds, nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T component.Component](wCtx cardinal.WorldContext, id entity.ID) error {
	if wCtx.IsReadOnly() {
		return eris.Wrap(cardinal.ErrCannotModifyStateWithReadOnlyContext, "")
	}
	e := wCtx.GetEngine()
	var t T
	name := t.Name()
	c, err := e.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "must register component")
	}
	return e.StoreManager().RemoveComponentFromEntity(c, id)
}

func AddComponentTo[T component.Component](wCtx cardinal.WorldContext, id entity.ID) error {
	if wCtx.IsReadOnly() {
		return eris.Wrap(cardinal.ErrCannotModifyStateWithReadOnlyContext, "")
	}
	e := wCtx.GetEngine()
	var t T
	name := t.Name()
	c, err := e.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "must register component")
	}
	return e.StoreManager().AddComponentToEntity(c, id)
}

// GetComponent returns component data from the entity.
func GetComponent[T component.Component](wCtx cardinal.WorldContext, id entity.ID) (comp *T, err error) {
	var t T
	name := t.Name()
	c, err := wCtx.GetEngine().GetComponentByName(name)
	if err != nil {
		return nil, eris.Wrap(err, "must register component")
	}
	value, err := wCtx.StoreReader().GetComponentForEntity(c, id)
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
func SetComponent[T component.Component](wCtx cardinal.WorldContext, id entity.ID, component *T) error {
	if wCtx.IsReadOnly() {
		return eris.Wrap(cardinal.ErrCannotModifyStateWithReadOnlyContext, "")
	}
	var t T
	name := t.Name()
	c, err := wCtx.GetEngine().GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "get component by name failed")
	}
	err = wCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return eris.Wrap(err, "set component failed")
	}
	wCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

func UpdateComponent[T component.Component](wCtx cardinal.WorldContext, id entity.ID, fn func(*T) *T) error {
	if wCtx.IsReadOnly() {
		return eris.Wrap(cardinal.ErrCannotModifyStateWithReadOnlyContext, "")
	}
	val, err := GetComponent[T](wCtx, id)
	if err != nil {
		return err
	}
	updatedVal := fn(val)
	return SetComponent[T](wCtx, id, updatedVal)
}
