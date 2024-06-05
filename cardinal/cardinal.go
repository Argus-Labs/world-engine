package cardinal

import (
	"errors"
	"reflect"
	"strconv"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/component"
	"pkg.world.dev/world-engine/cardinal/iterators"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/worldstage"
)

var (
	ErrEntityMutationOnReadOnly          = errors.New("cannot modify state with read only context")
	ErrEntitiesCreatedBeforeReady        = errors.New("entities should not be created before world is ready")
	ErrEntityDoesNotExist                = iterators.ErrEntityDoesNotExist
	ErrEntityMustHaveAtLeastOneComponent = iterators.ErrEntityMustHaveAtLeastOneComponent
	ErrComponentNotOnEntity              = iterators.ErrComponentNotOnEntity
	ErrComponentAlreadyOnEntity          = iterators.ErrComponentAlreadyOnEntity
)

// FilterFunction wrap your component filter function of func(comp T) bool inside FilterFunction to use
// in search.
//
// Usage:
//
// cardinal.NewSearch().Entity(filter.Not(filter.
// Contains(filter.Component[AlphaTest]()))).Where(cardinal.FilterFunction[GammaTest](func(_ GammaTest) bool {
//  	return true
// }))

func FilterFunction[T types.Component](f func(comp T) bool) func(ctx WorldContext, id types.EntityID) (bool, error) {
	return ComponentFilter[T](f)
}

func RegisterSystems(w *World, sys ...System) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"world state is %s, expected %s to register systems",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}
	return w.SystemManager.registerSystems(false, sys...)
}

func RegisterInitSystems(w *World, sys ...System) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"world state is %s, expected %s to register init systems",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}
	return w.SystemManager.registerSystems(true, sys...)
}

func RegisterComponent[T types.Component](w *World) error {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"world state is %s, expected %s to register component",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}

	compMetadata, err := component.NewComponentMetadata[T]()
	if err != nil {
		return err
	}

	err = w.RegisterComponent(compMetadata)
	if err != nil {
		return err
	}

	return nil
}

func MustRegisterComponent[T types.Component](w *World) {
	err := RegisterComponent[T](w)
	if err != nil {
		panic(err)
	}
}

func EachMessage[In any, Out any](wCtx WorldContext, fn func(TxData[In]) (Out, error)) error {
	var msg MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := wCtx.getMessageByType(msgType)
	if !ok {
		return eris.Errorf("Could not find %s, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*MessageType[In, Out])
	if !ok {
		return eris.New("wrong type")
	}
	res.Each(wCtx, fn)
	return nil
}

// RegisterMessage registers a message to the world. Cardinal will automatically set up HTTP routes that map to each
// registered message. Message URLs are take the form of "group.name". A default group, "game", is used
// unless the WithCustomMessageGroup option is used. Example: game.throw-rock
func RegisterMessage[In any, Out any](world *World, name string, opts ...MessageOption[In, Out]) error {
	if world.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"world state is %s, expected %s to register messages",
			world.worldStage.Current(),
			worldstage.Init,
		)
	}

	// Create the message type
	msgType := NewMessageType[In, Out](name, opts...)

	// Register the message with the manager
	err := world.RegisterMessage(msgType, reflect.TypeOf(*msgType))
	if err != nil {
		return err
	}

	return nil
}

func RegisterQuery[Request any, Reply any](
	w *World,
	name string,
	handler func(wCtx WorldContext, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) (err error) {
	if w.worldStage.Current() != worldstage.Init {
		return eris.Errorf(
			"world state is %s, expected %s to register query",
			w.worldStage.Current(),
			worldstage.Init,
		)
	}

	q, err := newQueryType[Request, Reply](name, handler, opts...)
	if err != nil {
		return err
	}

	res := w.RegisterQuery(q)
	return res
}

// Create creates a single entity in the world, and returns the id of the newly created entity.
// At least 1 component must be provided.
func Create(wCtx WorldContext, components ...types.Component) (_ types.EntityID, err error) {
	// We don't handle panics here because we let CreateMany handle it for us
	entityIDs, err := CreateMany(wCtx, 1, components...)
	if err != nil {
		return 0, err
	}
	return entityIDs[0], nil
}

// CreateMany creates multiple entities in the world, and returns the slice of ids for the newly created
// entities. At least 1 component must be provided.
func CreateMany(wCtx WorldContext, num int, components ...types.Component) (entityIDs []types.EntityID, err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.isReadOnly() {
		return nil, ErrEntityMutationOnReadOnly
	}

	if !wCtx.isWorldReady() {
		return nil, ErrEntitiesCreatedBeforeReady
	}

	// Get all component metadata for the given components
	acc := make([]types.ComponentMetadata, 0, len(components))
	for _, comp := range components {
		c, err := wCtx.getComponentByName(comp.Name())
		if err != nil {
			return nil, eris.Wrap(err, "failed to create entity because component is not registered")
		}
		acc = append(acc, c)
	}

	// Create the entities
	entityIDs, err = wCtx.storeManager().CreateManyEntities(num, acc...)
	if err != nil {
		return nil, err
	}

	// Store the components for the entities
	for _, id := range entityIDs {
		for _, comp := range components {
			var c types.ComponentMetadata
			c, err = wCtx.getComponentByName(comp.Name())
			if err != nil {
				return nil, eris.Wrap(err, "failed to create entity because component is not registered")
			}

			err = wCtx.storeManager().SetComponentForEntity(c, id, comp)
			if err != nil {
				return nil, err
			}
		}
	}

	return entityIDs, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T types.Component](wCtx WorldContext, id types.EntityID, component *T) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.isReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.getComponentByName(t.Name())
	if err != nil {
		return err
	}

	// Store the component
	err = wCtx.storeManager().SetComponentForEntity(c, id, component)
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
func GetComponent[T types.Component](wCtx WorldContext, id types.EntityID) (comp *T, err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Get the component metadata
	var t T
	c, err := wCtx.getComponentByName(t.Name())
	if err != nil {
		return nil, err
	}

	// Get current component value
	compValue, err := wCtx.storeReader().GetComponentForEntity(c, id)
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

func UpdateComponent[T types.Component](wCtx WorldContext, id types.EntityID, fn func(*T) *T) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.isReadOnly() {
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

func AddComponentTo[T types.Component](wCtx WorldContext, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.isReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.getComponentByName(t.Name())
	if err != nil {
		return err
	}

	// Add the component to entity
	err = wCtx.storeManager().AddComponentToEntity(c, id)
	if err != nil {
		return err
	}

	return nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T types.Component](wCtx WorldContext, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.isReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	// Get the component metadata
	var t T
	c, err := wCtx.getComponentByName(t.Name())
	if err != nil {
		return err
	}

	// Remove the component from entity
	err = wCtx.storeManager().RemoveComponentFromEntity(c, id)
	if err != nil {
		return err
	}

	return nil
}

// Remove removes the given Entity from the world.
func Remove(wCtx WorldContext, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	// Error if the context is read only
	if wCtx.isReadOnly() {
		return ErrEntityMutationOnReadOnly
	}

	err = wCtx.storeManager().RemoveEntity(id)
	if err != nil {
		return err
	}

	return nil
}
