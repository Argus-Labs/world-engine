package storage

import (
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

// ComponentIndex represents the Index of component in an archetype.
type ComponentIndex int

// Components is a structure that facilitates the storage and retrieval of component data.
// TODO: this kinda sucks.. could prob refactor this and make it prettier for devs.
type Components struct {
	Store            ComponentStorageManager
	ComponentIndices ComponentIndexStorage
}

// NewComponents creates a new empty structure that stores data of components.
func NewComponents(store ComponentStorageManager, idxStore ComponentIndexStorage) Components {
	return Components{
		Store:            store,
		ComponentIndices: idxStore,
	}
}

func (cs *Components) PushRawComponents(comps []*anypb.Any, archetypeIndex ArchetypeIndex) (ComponentIndex, error) {
	for _, c := range comps {
		v := cs.Store.GetComponentStorage(component.ID(c))
		err := v.PushRawComponent(c, archetypeIndex)
		if err != nil {
			return 0, err
		}
	}
	idx, ok, err := cs.ComponentIndices.ComponentIndex(archetypeIndex)
	if err != nil && err != redis.Nil {
		return 0, err
	}
	if !ok {
		err := cs.ComponentIndices.SetIndex(archetypeIndex, 0)
		if err != nil {
			return 0, err
		}
		idx = 0
	} else {
		err := cs.ComponentIndices.IncrementIndex(archetypeIndex)
		if err != nil {
			return 0, err
		}
		idx += 1
	}
	return idx, err
}

// PushComponents stores the new data of the component in the archetype.
func (cs *Components) PushComponents(components []component.IComponentType, archetypeIndex ArchetypeIndex) (ComponentIndex, error) {
	for _, componentType := range components {
		v := cs.Store.GetComponentStorage(component.ID(componentType))
		_, err := v.PushComponent(componentType, archetypeIndex)
		if err != nil {
			return 0, err
		}
	}
	idx, ok, err := cs.ComponentIndices.ComponentIndex(archetypeIndex)
	if err != nil && err != redis.Nil {
		return 0, err
	}
	if !ok {
		err := cs.ComponentIndices.SetIndex(archetypeIndex, 0)
		if err != nil {
			return 0, err
		}
		idx = 0
	} else {
		err := cs.ComponentIndices.IncrementIndex(archetypeIndex)
		if err != nil {
			return 0, err
		}
		idx += 1
	}
	return idx, err
}

// Move moves the bytes of data of the component in the archetype.
func (cs *Components) Move(src ArchetypeIndex, dst ArchetypeIndex) error {
	err := cs.ComponentIndices.DecrementIndex(src)
	if err != nil {
		return err
	}
	err = cs.ComponentIndices.IncrementIndex(dst)
	if err != nil {
		return err
	}
	return nil
}

// Storage returns the component data storage accessor.
func (cs *Components) Storage(c component.IComponentType) ComponentStorage {
	return cs.Store.GetComponentStorage(component.ID(c))
}

func (cs *Components) StorageFromAny(anyComp *anypb.Any) ComponentStorage {
	return cs.Store.GetComponentStorage(component.ID(anyComp))
}

func (cs *Components) StorageFromID(id string) ComponentStorage {
	return cs.Store.GetComponentStorage(id)
}

func (cs *Components) GetComponentIndexStorage(c component.IComponentType) ComponentIndexStorage {
	return cs.Store.GetComponentIndexStorage(c)
}

// Remove removes the component from the storage.
func (cs *Components) Remove(ai ArchetypeIndex, comps []*anypb.Any, ci ComponentIndex) error {
	for _, ct := range comps {
		err := cs.remove(ct, ai, ci)
		if err != nil {
			return err
		}
	}
	return cs.ComponentIndices.DecrementIndex(ai)
}

func (cs *Components) remove(ct *anypb.Any, ai ArchetypeIndex, ci ComponentIndex) error {
	storage := cs.StorageFromAny(ct)
	err := storage.RemoveComponent(ai, ci)
	return err
}
