package ecs

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/climate"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
)

const (
	climateKeyTxQueue  = "tx_queue_climate_key"
	climateKeyWorld    = "world_climate_key"
	climateKeyReadOnly = "read_only_climate_key"
	climateKeyLogger   = "logger_climate_key"
)

func climateGetAs[T any](clim climate.Climate, key string) (value T, err error) {
	iface := clim.Get(key)
	if iface == nil {
		return value, fmt.Errorf("cannot find value for %q", key)
	}
	value, ok := iface.(T)
	if !ok {
		return value, fmt.Errorf("cannot value at %q to type %T", key, value)
	}
	return value, nil
}

func climateGetTxQueue(clim climate.Climate) (*transaction.TxQueue, error) {
	return climateGetAs[*transaction.TxQueue](clim, climateKeyTxQueue)
}

func climateGetWorld(clim climate.Climate) (*World, error) {
	return climateGetAs[*World](clim, climateKeyWorld)
}

func climateIsReadOnly(clim climate.Climate) bool {
	ro, err := climateGetAs[bool](clim, climateKeyReadOnly)
	// There was an error, but just to be safe let's disallow state changes
	if err != nil {
		return true
	}
	return ro
}

func NewClimate(world *World, tq *transaction.TxQueue) climate.Climate {
	clim := climate.NewClimate()
	clim.Set(climateKeyWorld, world)
	clim.Set(climateKeyTxQueue, tq)
	clim.Set(climateKeyLogger, world.Logger)
	clim.Set(climateKeyReadOnly, false)
	return clim
}

func NewReadOnlyClimate(world *World) climate.Climate {
	clim := climate.NewClimate()
	clim.Set(climateKeyWorld, world)
	clim.Set(climateKeyTxQueue, nil)
	clim.Set(climateKeyLogger, world.Logger)
	clim.Set(climateKeyReadOnly, true)
	return clim
}

func climateGetStoreReader(clim climate.Climate) (store.Reader, error) {
	world, err := climateGetWorld(clim)
	if err != nil {
		return nil, err
	}
	readOnly := climateIsReadOnly(clim)
	if readOnly {
		return world.StoreManager().ToReadOnly(), nil
	}
	return world.StoreManager(), nil
}
