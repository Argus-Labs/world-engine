package encom

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type EncomStorage struct {
	store storage.WorldStorage
}

func NewEncomStorage(store storage.WorldStorage) *EncomStorage {
	return &EncomStorage{
		store: store,
	}
}

func (e *EncomStorage) SetComponent(cType component.IComponentType, id storage.EntityID, value any) error {
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
