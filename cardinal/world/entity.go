package world

import (
	"strconv"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

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

	// Create the entities
	sm, err := wCtx.stateWriter()
	if err != nil {
		return nil, err
	}

	entityIDs, err = sm.CreateManyEntities(num, components...)
	if err != nil {
		return nil, err
	}

	// Store the components for the entities
	for _, id := range entityIDs {
		for _, comp := range components {
			if err := sm.SetComponentForEntity(id, comp); err != nil {
				return nil, err
			}
		}
	}

	return entityIDs, nil
}

// SetComponent sets component data to the entity.
func SetComponent[T types.Component](wCtx WorldContext, id types.EntityID, component *T) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	sm, err := wCtx.stateWriter()
	if err != nil {
		return err
	}

	err = sm.SetComponentForEntity(id, *component)
	if err != nil {
		return err
	}

	wCtx.Logger().Debug().
		Str("entity_id", strconv.FormatUint(uint64(id), 10)).
		Str("component_name", (*component).Name()).
		Msg("entity updated")

	return nil
}

// GetComponent returns component data from the entity.
func GetComponent[T types.Component](wCtx WorldContextReadOnly, id types.EntityID) (comp *T, err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	var t T

	// Get current component value
	compValue, err := wCtx.stateReader().GetComponentForEntity(t, id)
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

	sm, err := wCtx.stateWriter()
	if err != nil {
		return err
	}

	var t T
	if err := sm.AddComponentToEntity(t, id); err != nil {
		return err
	}

	return nil
}

// RemoveComponentFrom removes a component from an entity.
func RemoveComponentFrom[T types.Component](wCtx WorldContext, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	sm, err := wCtx.stateWriter()
	if err != nil {
		return err
	}

	var t T
	return sm.RemoveComponentFromEntity(t, id)
}

// Remove removes the given Entity from the world.
func Remove(wCtx WorldContext, id types.EntityID) (err error) {
	defer func() { panicOnFatalError(wCtx, err) }()

	sm, err := wCtx.stateWriter()
	if err != nil {
		return err
	}

	err = sm.RemoveEntity(id)
	if err != nil {
		return err
	}

	return nil
}

func EachMessage[Msg message.Message](wCtx WorldContext, fn func(Tx[Msg]) (any, error)) error {
	tick := wCtx.getTick()

	var msg Msg
	for _, txData := range tick.Txs[msg.Name()] {
		tx := Tx[Msg]{
			Hash: *txData.Tx.HashHex(),
			Msg:  txData.Msg.(Msg),
			Tx:   txData.Tx,
		}

		result, txErr := fn(tx)
		if txErr != nil {
			txErr = eris.Wrap(txErr, "")
			wCtx.Logger().Err(txErr).Msgf("tx %s from %s encountered an error with message=%+v and stack trace:\n %s",
				tx.Hash,
				tx.Tx.PersonaTag,
				tx.Msg,
				eris.ToString(txErr, true),
			)
		}

		err := tick.SetReceipts(txData.Tx.Hash, result, txErr)
		if err != nil {
			return eris.Wrap(err, "failed to set receipt")
		}
	}
	return nil
}
