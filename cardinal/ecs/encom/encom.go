package encom

import (
	"errors"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

var (
	ErrorReadOnlyEncomStorageCannotChangeState = errors.New("read only entity/component storage cannot change state")
)

type EncomStorage struct {
	store    storage.WorldStorage
	readOnly bool
}

func NewEncomStorage(store storage.WorldStorage) *EncomStorage {
	return &EncomStorage{
		store:    store,
		readOnly: false,
	}
}

func (e *EncomStorage) AsReadOnly() *EncomStorage {
	return &EncomStorage{
		store:    e.store,
		readOnly: true,
	}
}

func (e *EncomStorage) SetComponent(cType component.IComponentType, id storage.EntityID, value any) error {
	if e.readOnly {
		return ErrorReadOnlyEncomStorageCannotChangeState
	}
	loc, err := e.store.EntityLocStore.GetLocation(id)
	if err != nil {
		return err
	}

	bz, err := cType.Marshal(value)
	if err != nil {
		return err
	}
	return e.store.CompStore.Storage(cType).SetComponent(loc.ArchID, loc.CompIndex, bz)
}

func (e *EncomStorage) GetComponent(cType component.IComponentType, id storage.EntityID) (any, error) {
	loc, err := e.store.EntityLocStore.GetLocation(id)
	if err != nil {
		return storage.BadEntity, err
	}
	bz, err := e.store.CompStore.Storage(cType).Component(loc.ArchID, loc.CompIndex)
	if err != nil {
		return nil, err
	}
	return cType.Unmarshal(bz)
}
