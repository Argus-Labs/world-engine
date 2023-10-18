package cardinal

import (
	"errors"
	"fmt"
	"strconv"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

func CreateMany(world *ecs.World, num int, components ...component_metadata.Component) ([]entity.ID, error) {
	acc := make([]component_metadata.IComponentMetaData, 0, len(components))
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
			c, err := world.GetComponentByName(comp.Name())
			if err != nil {
				return nil, errors.New("Must register component before creating an entity")
			}
			err = world.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}
	return entityIds, nil
}

func Create(world *ecs.World, components ...component_metadata.Component) (entity.ID, error) {
	entities, err := CreateMany(world, 1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}

func RemoveComponentFrom[T component_metadata.Component](w *ecs.World, id entity.ID) error {
	var t T
	name := t.Name()
	c, err := w.GetComponentByName(name)
	if err != nil {
		return errors.New("Must register component")
	}
	return w.StoreManager().RemoveComponentFromEntity(c, id)
}

func AddComponentTo[T component_metadata.Component](w *ecs.World, id entity.ID) error {
	var t T
	name := t.Name()
	c, err := w.GetComponentByName(name)
	if err != nil {
		return errors.New("Must register component")
	}
	return w.StoreManager().AddComponentToEntity(c, id)
}

// Get returns component data from the entity.
func GetComponent[T component_metadata.Component](w *ecs.World, id entity.ID) (comp *T, err error) {
	var t T
	name := t.Name()
	c, err := w.GetComponentByName(name)
	if err != nil {
		return nil, errors.New("Must register component")
	}
	value, err := w.StoreManager().GetComponentForEntity(c, id)
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

// Set sets component data to the entity.
func SetComponent[T component_metadata.Component](w *ecs.World, id entity.ID, component *T) error {
	var t T
	name := t.Name()
	c, err := w.GetComponentByName(name)
	if err != nil {
		return fmt.Errorf("%s is not registered, please register it before updating", t.Name())
	}
	err = w.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return err
	}
	w.Logger.Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")
	return nil
}

func UpdateComponent[T component_metadata.Component](w *ecs.World, id entity.ID, fn func(*T) *T) error {
	val, err := GetComponent[T](w, id)
	if err != nil {
		return err
	}
	updatedVal := fn(val)
	return SetComponent[T](w, id, updatedVal)
}
