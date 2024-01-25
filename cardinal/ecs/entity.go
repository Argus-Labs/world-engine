package ecs

import (
	"strconv"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

func Create(eCtx EngineContext, components ...component.Component) (entity.ID, error) {
	entities, err := CreateMany(eCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}

func CreateMany(eCtx EngineContext, num int, components ...component.Component) ([]entity.ID, error) {
	if eCtx.IsReadOnly() {
		return nil, eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	engine := eCtx.GetEngine()
	acc := make([]component.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := engine.GetComponentByName(comp.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	entityIds, err := engine.GameStateManager().CreateManyEntities(num, acc...)
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
			err = engine.GameStateManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}
	eCtx.GetEngine().SetEntitiesCreated(true)
	return entityIds, nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T component.Component](eCtx EngineContext, id entity.ID) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	e := eCtx.GetEngine()
	var t T
	name := t.Name()
	c, err := e.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "must register component")
	}
	return e.GameStateManager().RemoveComponentFromEntity(c, id)
}

func AddComponentTo[T component.Component](eCtx EngineContext, id entity.ID) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	e := eCtx.GetEngine()
	var t T
	name := t.Name()
	c, err := e.GetComponentByName(name)
	if err != nil {
		return eris.Wrap(err, "must register component")
	}
	return e.GameStateManager().AddComponentToEntity(c, id)
}

// GetComponent returns component data from the entity.
func GetComponent[T component.Component](eCtx EngineContext, id entity.ID) (comp *T, err error) {
	var t T
	name := t.Name()
	c, err := eCtx.GetEngine().GetComponentByName(name)
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
func SetComponent[T component.Component](eCtx EngineContext, id entity.ID, component *T) error {
	if eCtx.IsReadOnly() {
		return eris.Wrap(ErrCannotModifyStateWithReadOnlyContext, "")
	}
	var t T
	name := t.Name()
	c, err := eCtx.GetEngine().GetComponentByName(name)
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

func UpdateComponent[T component.Component](eCtx EngineContext, id entity.ID, fn func(*T) *T) error {
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
