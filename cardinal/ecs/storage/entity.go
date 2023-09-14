package storage

import (
	"math"
)

var _ EntityManager = &entityMgrImpl{}

func NewEntityManager() EntityManager {
	return &entityMgrImpl{destroyed: make([]EntityID, 0, 256), nextID: 0}
}

type entityMgrImpl struct {
	destroyed []EntityID
	nextID    EntityID
}

func (e *entityMgrImpl) GetNextEntityID() EntityID {
	e.nextID++
	return e.nextID
}

func (e *entityMgrImpl) shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

func (e *entityMgrImpl) NewEntity() (EntityID, error) {
	if len(e.destroyed) == 0 {
		id := e.GetNextEntityID()
		return id, nil
	}
	newEntity := e.destroyed[(len(e.destroyed) - 1)]
	e.shrink()
	return newEntity, nil
}

func (e *entityMgrImpl) Destroy(id EntityID) {
	e.destroyed = append(e.destroyed, id)
}

type EntityID uint64

// Entity is a struct that contains an EntityID and a location in an archetype.
type Entity struct {
	ID  EntityID
	Loc Location
}

func NewEntity(id EntityID, loc Location) Entity {
	return Entity{
		ID:  id,
		Loc: loc,
	}
}

var (
	BadID     EntityID = math.MaxUint64
	BadEntity Entity   = Entity{BadID, Location{}}
)

// EntityID returns the Entity.
func (e Entity) EntityID() EntityID {
	return e.ID
}

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
