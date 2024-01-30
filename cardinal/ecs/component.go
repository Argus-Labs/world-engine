package ecs

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/iterators"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"
	"pkg.world.dev/world-engine/cardinal/types/component"
)

func RegisterComponent[T component.Component](engine *Engine) error {
	if engine.EngineState != EngineStateInit {
		return eris.New("cannot register components after loading game state")
	}
	var t T
	_, err := engine.GetComponentByName(t.Name())
	if err == nil {
		return eris.Errorf("component %q is already registered", t.Name())
	}
	c, err := component.NewComponentMetadata[T]()
	if err != nil {
		return err
	}
	err = c.SetID(engine.nextComponentID)
	if err != nil {
		return err
	}
	engine.nextComponentID++
	engine.registeredComponents = append(engine.registeredComponents, c)

	storedSchema, err := engine.redisStorage.GetSchema(c.Name())

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

	err = engine.redisStorage.SetSchema(c.Name(), c.GetSchema())
	if err != nil {
		return err
	}
	engine.nameToComponent[t.Name()] = c
	engine.isComponentsRegistered = true
	return nil
}

func MustRegisterComponent[T component.Component](engine *Engine) {
	err := RegisterComponent[T](engine)
	if err != nil {
		panic(err)
	}
}

func (e *Engine) GetComponents() []component.ComponentMetadata {
	return e.registeredComponents
}

func (e *Engine) GetComponentByName(name string) (component.ComponentMetadata, error) {
	componentType, exists := e.nameToComponent[name]
	if !exists {
		return nil, eris.Wrapf(
			iterators.ErrMustRegisterComponent,
			"component %q must be registered before being used", name)
	}
	return componentType, nil
}
