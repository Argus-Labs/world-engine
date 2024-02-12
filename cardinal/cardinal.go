package cardinal

import (
	"errors"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/system"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
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

func RegisterSystems(w *World, sys ...system.System) error {
	return w.systemManager.RegisterSystems(sys...)
}

func RegisterInitSystems(w *World, sys ...system.System) error {
	return w.systemManager.RegisterInitSystems(sys...)
}

func RegisterComponent[T types.Component](w *World) error {
	if w.WorldState != WorldStateInit {
		return eris.New("cannot register components after loading game state")
	}
	var t T
	_, err := w.GetComponentByName(t.Name())
	if err == nil {
		return eris.Errorf("component %q is already registered", t.Name())
	}
	c, err := NewComponentMetadata[T]()
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
		valid, err := types.IsComponentValid(t, storedSchema)
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

func MustRegisterComponent[T types.Component](w *World) {
	err := RegisterComponent[T](w)
	if err != nil {
		panic(err)
	}
}

// RegisterMessages adds the given messages to the game world. HTTP endpoints to queue up/execute these
// messages will automatically be created when StartGame is called. This Register method must only be called once.
func RegisterMessages(w *World, msgs ...types.Message) error {
	if w.WorldState != WorldStateInit {
		return eris.Errorf(
			"engine state is %s, expected %s to register messages",
			w.WorldState,
			WorldStateInit,
		)
	}
	return w.msgManager.RegisterMessages(msgs...)
}

func RegisterQuery[Request any, Reply any](
	world *World,
	name string,
	handler func(wCtx engine.Context, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) error {
	if world.WorldState != WorldStateInit {
		panic("cannot register queries after loading game state")
	}

	if _, ok := world.nameToQuery[name]; ok {
		return eris.Errorf("query with name %s is already registered", name)
	}

	q, err := NewQueryType[Request, Reply](name, handler, opts...)
	if err != nil {
		return err
	}

	world.registeredQueries = append(world.registeredQueries, q)
	world.nameToQuery[q.Name()] = q

	return nil
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(wCtx engine.Context, components ...types.Component) (types.EntityID, error) {
	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return 0, ErrEntityMutationOnReadOnly
	}

	id, err := CreateMany(wCtx, 1, components...)
	if err != nil {
		return 0, logAndPanic(wCtx, err)
	}

	return id[0], nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(wCtx engine.Context, num int, components ...types.Component) ([]types.EntityID, error) {
	if wCtx.IsWorldReady() {
		return nil, eris.Wrap(ErrEntitiesCreatedBeforeStartGame, "")
	}

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return nil, ErrEntityMutationOnReadOnly
	}

	// Get all component metadata for the given components
	acc := make([]types.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := wCtx.GetComponentByName(comp.Name())
		if err != nil {
			return nil, logAndPanic(wCtx, ErrComponentNotRegistered)
		}
		acc = append(acc, c)
	}

	// Create the entities
	entityIds, err := wCtx.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, logAndPanic(wCtx, err)
	}

	// Set the components for the entities
	for _, id := range entityIds {
		for _, comp := range components {
			var c types.ComponentMetadata
			c, err = wCtx.GetComponentByName(comp.Name())
			if err != nil {
				return nil, logAndPanic(wCtx, ErrComponentNotRegistered)
			}

			err = wCtx.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, logAndPanic(wCtx, err)
			}
		}
	}

	return entityIds, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T types.Component](wCtx engine.Context, id types.EntityID, component *T) error {
	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return logAndPanic(wCtx, err)
	}

	// Set the component
	err = wCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) || eris.Is(err, ErrComponentNotOnEntity) {
			return err
		}
		return logAndPanic(wCtx, err)
	}

	// Log
	wCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", c.Name()).
		Int("component_id", int(c.ID())).
		Msg("entity updated")

	return nil
}

// GetComponent returns component data from the entity.
func GetComponent[T types.Component](wCtx engine.Context, id types.EntityID) (comp *T, err error) {
	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return nil, logAndPanic(wCtx, err)
	}

	// Get current component value
	compValue, err := wCtx.StoreReader().GetComponentForEntity(c, id)
	if err != nil {
		if wCtx.IsReadOnly() {
			return nil, err
		}
		if eris.Is(err, ErrEntityDoesNotExist) || eris.Is(err, ErrComponentNotOnEntity) || eris.Is(err,
			ErrComponentNotRegistered) {
			return nil, err
		}
		return nil, logAndPanic(wCtx, err)
	}

	// Type assert the component value to the component type
	t, ok := compValue.(T)
	if !ok {
		comp, ok = compValue.(*T)
		if !ok {
			return nil, logAndPanic(wCtx, eris.Errorf("type assertion for component failed: %v to %v", compValue, c))
		}
	} else {
		comp = &t
	}

	return comp, nil
}

func UpdateComponent[T types.Component](wCtx engine.Context, id types.EntityID, fn func(*T) *T) error {
	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get current component value
	val, err := GetComponent[T](wCtx, id)
	if err != nil {
		return err // We don't need to panic here because GetComponent will handle the panic if needed for us.
	}

	// Get the new component value
	updatedVal := fn(val)

	// Set the new component value
	err = SetComponent[T](wCtx, id, updatedVal)
	if err != nil {
		return err // We don't need to panic here because SetComponent will handle the panic if needed for us.
	}

	return nil
}

func AddComponentTo[T types.Component](wCtx engine.Context, id types.EntityID) error {
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return logAndPanic(wCtx, err)
	}

	// Add the component to entity
	err = wCtx.StoreManager().AddComponentToEntity(c, id)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) || eris.Is(err, ErrComponentAlreadyOnEntity) {
			return err
		}
		return logAndPanic(wCtx, err)
	}

	return nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T types.Component](wCtx engine.Context, id types.EntityID) error {
	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return logAndPanic(wCtx, err)
	}

	// Remove the component from entity
	err = wCtx.StoreManager().RemoveComponentFromEntity(c, id)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) ||
			eris.Is(err, ErrComponentNotOnEntity) ||
			eris.Is(err, ErrEntityMustHaveAtLeastOneComponent) {
			return err
		}
		return logAndPanic(wCtx, err)
	}

	return nil
}

// Remove removes the given Entity from the engine.
func Remove(wCtx engine.Context, id types.EntityID) error {
	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	err := wCtx.StoreManager().RemoveEntity(id)
	if err != nil {
		if eris.Is(err, ErrEntityDoesNotExist) {
			return err
		}
		return logAndPanic(wCtx, err)
	}

	return nil
}
