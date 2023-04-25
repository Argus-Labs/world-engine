package memory

import (
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

var _ storage.ArchetypeStorage = &archetypeStorage{}

func NewArchetypeStorage() storage.ArchetypeStorage {
	return &archetypeStorage{archetypes: make(map[storage.ArchetypeIndex]*types.Archetype)}
}

type archetypeStorage struct {
	archetypes map[storage.ArchetypeIndex]*types.Archetype
}

func (a *archetypeStorage) PushArchetype(index storage.ArchetypeIndex, layout []component.IComponentType) {
	// TODO(technicallyty): marshal the layout.
	a.archetypes[index] = &types.Archetype{
		ArchetypeIndex: uint64(index),
		EntityIds:      make([]uint64, 0, 256),
		Components:     make([]*anypb.Any, 0),
	}
}

func (a *archetypeStorage) Archetype(index storage.ArchetypeIndex) *types.Archetype {
	return a.archetypes[index]
}

func (a *archetypeStorage) RemoveEntity(index storage.ArchetypeIndex, entityIndex int) entity.Entity {
	arch := a.archetypes[index]
	removed := arch.EntityIds[entityIndex]
	arch.EntityIds[entityIndex] = arch.EntityIds[len(arch.EntityIds)-1]
	arch.EntityIds = arch.EntityIds[:len(arch.EntityIds)-1]
	return entity.Entity(removed)
}

func (a *archetypeStorage) PushEntity(index storage.ArchetypeIndex, entity entity.Entity) {
	a.archetypes[index].EntityIds = append(a.archetypes[index].EntityIds, uint64(entity.ID()))
}

func (a *archetypeStorage) GetNextArchetypeIndex() (uint64, error) {
	return len(a.archetypes), nil
}
