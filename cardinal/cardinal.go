package cardinal

import (
	"errors"
	"strconv"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/system"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/worldstage"
)

var (
	ErrEntityMutationOnReadOnly          = errors.New("cannot modify state with read only context")
	ErrEntitiesCreatedBeforeReady        = errors.New("entities should not be created before world is ready")
	ErrEntityDoesNotExist                = iterators.ErrEntityDoesNotExist
	ErrEntityMustHaveAtLeastOneComponent = iterators.ErrEntityMustHaveAtLeastOneComponent
	ErrComponentNotOnEntity              = iterators.ErrComponentNotOnEntity
	ErrComponentAlreadyOnEntity          = iterators.ErrComponentAlreadyOnEntity
	ErrComponentNotRegistered            = iterators.ErrMustRegisterComponent
)

func RegisterSystems(w *World, sys ...system.System) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"engine state is %s, expected %s to register systems",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}
	return w.systemManager.RegisterSystems(sys...)
}

func RegisterInitSystems(w *World, sys ...system.System) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"engine state is %s, expected %s to register init systems",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}
	return w.systemManager.RegisterInitSystems(sys...)
}

func RegisterComponent[T types.Component](w *World) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"engine state is %s, expected %s to register component",
			w.worldStage.Current(),
			worldstage.Init,
		)
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

// RegisterMessagesByName adds the given messages to the game world. HTTP endpoints to queue up/execute these
// messages will automatically be created when StartGame is called. This Register method must only be called once.
func RegisterMessagesByName(w *World, msgs ...types.Message) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"engine state is %s, expected %s to register messages",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}
	return w.msgManager.RegisterMessagesByName(msgs...)
}

func RegisterQuery[Request any, Reply any](
	w *World,
	name string,
	handler func(wCtx engine.Context, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) (err error) {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"engine state is %s, expected %s to register query",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}

	if _, ok := w.nameToQuery[name]; ok {
		return eris.Errorf("query with name %s is already registered", name)
	}

	q, err := NewQueryType[Request, Reply](name, handler, opts...)
	if err != nil {
		return err
	}

	w.registeredQueries = append(w.registeredQueries, q)
	w.nameToQuery[q.Name()] = q

	return nil
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(wCtx engine.Context, components ...types.Component) (_ types.EntityID, err error) {
	// We don't handle panics here because we let CreateMany handle it for us
	entityIds, err := CreateMany(wCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entityIds[0], nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(wCtx engine.Context, num int, components ...types.Component) (entityIds []types.EntityID, err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return nil, ErrEntityMutationOnReadOnly
	}

	if !wCtx.IsWorldReady() {
		return nil, ErrEntitiesCreatedBeforeReady
	}

	// Get all component metadata for the given components
	acc := make([]types.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := wCtx.GetComponentByName(comp.Name())
		if err != nil {
			return nil, ErrComponentNotRegistered
		}
		acc = append(acc, c)
	}

	// Create the entities
	entityIds, err = wCtx.StoreManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, err
	}

	// Store the components for the entities
	for _, id := range entityIds {
		for _, comp := range components {
			var c types.ComponentMetadata
			c, err = wCtx.GetComponentByName(comp.Name())
			if err != nil {
				return nil, ErrComponentNotRegistered
			}

			err = wCtx.StoreManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}

	return entityIds, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T types.Component](wCtx engine.Context, id types.EntityID, component *T) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return err
	}

	// Store the component
	err = wCtx.StoreManager().SetComponentForEntity(c, id, component)
	if err != nil {
		return err
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
	defer func() { panicOnFatalError(wCtx, err) }()

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return nil, err
	}

	// Get current component value
	compValue, err := wCtx.StoreReader().GetComponentForEntity(c, id)
	if err != nil {
		return nil, err
	}

	// Type assert the component value to the component type
	t, ok := compValue.(T)
	if !ok {
		comp, ok = compValue.(*T)
		if !ok {
			return nil, err
		}
	} else {
		comp = &t
	}

	return comp, nil
}

func UpdateComponent[T types.Component](wCtx engine.Context, id types.EntityID, fn func(*T) *T) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return err
	}

	// Get current component value
	val, err := GetComponent[T](wCtx, id)
	if err != nil {
		return err
	}

	// Get the new component value
	updatedVal := fn(val)

	// Store the new component value
	err = SetComponent[T](wCtx, id, updatedVal)
	if err != nil {
		return err
	}

	return nil
}

func AddComponentTo[T types.Component](wCtx engine.Context, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return err
	}

	// Add the component to entity
	err = wCtx.StoreManager().AddComponentToEntity(c, id)
	if err != nil {
		return err
	}

	return nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T types.Component](wCtx engine.Context, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.GetComponentByName(t.Name())
	if err != nil {
		return err
	}

	// Remove the component from entity
	err = wCtx.StoreManager().RemoveComponentFromEntity(c, id)
	if err != nil {
		return err
	}

	return nil
}

// Remove removes the given Entity from the engine.
func Remove(wCtx engine.Context, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.IsReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	err = wCtx.StoreManager().RemoveEntity(id)
	if err != nil {
		return err
	}

	return nil
}
