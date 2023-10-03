package storage

import (
	"errors"
	"math"

	"pkg.world.dev/world-engine/cardinal/entity"
	"pkg.world.dev/world-engine/cardinal/public"
)

var _ public.EntityManager = &entityMgrImpl{}

func NewEntityManager() public.EntityManager {
	return &entityMgrImpl{destroyed: make([]public.EntityID, 0, 256), nextID: 0}
}

type entityMgrImpl struct {
	destroyed []public.EntityID
	nextID    public.EntityID
}

func (e *entityMgrImpl) GetNextEntityID() public.EntityID {
	e.nextID++
	return e.nextID
}

func (e *entityMgrImpl) shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

func (e *entityMgrImpl) NewEntity() (public.EntityID, error) {
	if len(e.destroyed) == 0 {
		id := e.GetNextEntityID()
		return id, nil
	}
	newEntity := e.destroyed[(len(e.destroyed) - 1)]
	e.shrink()
	return newEntity, nil
}

func (e *entityMgrImpl) Destroy(id public.EntityID) {
	e.destroyed = append(e.destroyed, id)
}

func NewEntity(id public.EntityID, loc public.ILocation) public.IEntity {
	res := entity.Entity{
		ID:  id,
		Loc: loc,
	}
	return &res
}

var (
	BadID     public.EntityID = math.MaxUint64
	BadEntity entity.Entity   = entity.Entity{BadID, &entity.Location{}}
)

var (
	ErrorComponentAlreadyOnEntity = errors.New("component already on entity")
	ErrorComponentNotOnEntity     = errors.New("component not on entity")
)

var _ public.StateStorage = &stateStorageImpl{}

func NewStateStorage() public.StateStorage {
	return &stateStorageImpl{
		data: map[string][]byte{},
	}
}

type stateStorageImpl struct {
	data map[string][]byte
}

func (s stateStorageImpl) Save(key string, data []byte) error {
	s.data[key] = data
	return nil
}

func (s stateStorageImpl) Load(key string) (data []byte, ok bool, err error) {
	buf, ok := s.data[key]
	return buf, ok, nil
}
