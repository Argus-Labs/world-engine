package storage

import (
	"errors"
	"math"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/entityid"
)

var _ EntityManager = &entityMgrImpl{}

func NewEntityManager() EntityManager {
	return &entityMgrImpl{destroyed: make([]entityid.ID, 0, 256), nextID: 0}
}

type entityMgrImpl struct {
	destroyed []entityid.ID
	nextID    entityid.ID
}

func (e *entityMgrImpl) GetNextEntityID() entityid.ID {
	e.nextID++
	return e.nextID
}

func (e *entityMgrImpl) shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

func (e *entityMgrImpl) NewEntity() (entityid.ID, error) {
	if len(e.destroyed) == 0 {
		id := e.GetNextEntityID()
		return id, nil
	}
	newEntity := e.destroyed[(len(e.destroyed) - 1)]
	e.shrink()
	return newEntity, nil
}

func (e *entityMgrImpl) Destroy(id entityid.ID) {
	e.destroyed = append(e.destroyed, id)
}

func NewEntity(id entityid.ID, loc entity.Location) entity.Entity {
	return entity.Entity{
		ID:  id,
		Loc: loc,
	}
}

var (
	BadID     entityid.ID   = math.MaxUint64
	BadEntity entity.Entity = entity.Entity{BadID, entity.Location{}}
)

var (
	ErrorComponentAlreadyOnEntity = errors.New("component already on entity")
	ErrorComponentNotOnEntity     = errors.New("component not on entity")
)

var _ StateStorage = &stateStorageImpl{}

func NewStateStorage() StateStorage {
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
