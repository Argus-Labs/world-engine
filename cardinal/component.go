package cardinal

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"
	"pkg.world.dev/world-engine/cardinal/types/component"
)

func RegisterComponent[T component.Component](w *World) error {
	if w.WorldState != WorldStateInit {
		return eris.New("cannot register components after loading game state")
	}
	var t T
	_, err := w.GetComponentByName(t.Name())
	if err == nil {
		return eris.Errorf("component %q is already registered", t.Name())
	}
	c, err := component.NewComponentMetadata[T]()
	if err != nil {
		return err
	}
	err = c.SetID(w.nextComponentID)
	if err != nil {
		return err
	}
	w.nextComponentID++
	w.registeredComponents = append(w.registeredComponents, c)

	storedSchema, err := w.redisStorage.GetSchema(c.Name())

	if err != nil {
		// It's fine if the schema doesn't currently exist in the db. Any other errors are a problem.
		if !eris.Is(err, redis.ErrNoSchemaFound) {
			return err
		}
	} else {
		valid, err := component.IsComponentValid(t, storedSchema)
		if err != nil {
			return err
		}
		if !valid {
			return eris.Errorf("Component: %s does not match the type stored in the db", c.Name())
		}
	}

	err = w.redisStorage.SetSchema(c.Name(), c.GetSchema())
	if err != nil {
		return err
	}
	w.nameToComponent[t.Name()] = c
	w.isComponentsRegistered = true
	return nil
}

func MustRegisterComponent[T component.Component](w *World) {
	err := RegisterComponent[T](w)
	if err != nil {
		panic(err)
	}
}

func (w *World) GetComponents() []component.ComponentMetadata {
	return w.registeredComponents
}

func (w *World) GetComponentByName(name string) (component.ComponentMetadata, error) {
	componentType, exists := w.nameToComponent[name]
	if !exists {
		return nil, eris.Wrapf(
			iterators.ErrMustRegisterComponent,
			"component %q must be registered before being used", name)
	}
	return componentType, nil
}
