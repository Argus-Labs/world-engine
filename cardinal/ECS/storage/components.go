package storage

import (
	"github.com/argus-labs/cardinal/ECS/component"
)

// ComponentIndex represents the Index of component in an archetype.
type ComponentIndex int

// Components is a structure that facilitates the storage and retrieval of component data.
// TODO: this kinda sucks.. could prob refactor this and make it prettier for devs.
type Components struct {
	store            ComponentStorageManager
	componentIndices ComponentIndexStorage
}

// NewComponents creates a new empty structure that stores data of components.
func NewComponents(store ComponentStorageManager, idxStore ComponentIndexStorage) Components {
	return Components{
		store:            store,
		componentIndices: idxStore,
	}
}

// PushComponents stores the new data of the component in the archetype.
func (cs *Components) PushComponents(components []component.IComponentType, archetypeIndex ArchetypeIndex) (ComponentIndex, error) {
	for _, componentType := range components {
		v := cs.store.GetComponentStorage(componentType.ID())
		err := v.PushComponent(componentType, archetypeIndex)
		if err != nil {
			return 0, err
		}
	}
	if _, ok, _ := cs.componentIndices.ComponentIndex(archetypeIndex); !ok {
		if err := cs.componentIndices.SetIndex(archetypeIndex, 0); err != nil {
			return 0, err
		}
	} else {
		if err := cs.componentIndices.IncrementIndex(archetypeIndex); err != nil {
			return 0, err
		}
	}
	idx, _, _ := cs.componentIndices.ComponentIndex(archetypeIndex)
	return idx, nil
}

// Move moves the bytes of data of the component in the archetype.
func (cs *Components) Move(src ArchetypeIndex, dst ArchetypeIndex) error {
	if err := cs.componentIndices.DecrementIndex(src); err != nil {
		return err
	}
	if err := cs.componentIndices.IncrementIndex(dst); err != nil {
		return err
	}
	return nil
}

// Storage returns the component data storage accessor.
func (cs *Components) Storage(c component.IComponentType) ComponentStorage {
	return cs.store.GetComponentStorage(c.ID())
}

func (cs *Components) GetComponentIndexStorage(c component.IComponentType) ComponentIndexStorage {
	return cs.store.GetComponentIndexStorage(c.ID())
}

// Remove removes the component from the storage.
func (cs *Components) Remove(ai ArchetypeIndex, comps []component.IComponentType, ci ComponentIndex) error {
	for _, ct := range comps {
		if err := cs.remove(ct, ai, ci); err != nil {
			return err
		}
	}
	return cs.componentIndices.DecrementIndex(ai)
}

func (cs *Components) remove(ct component.IComponentType, ai ArchetypeIndex, ci ComponentIndex) error {
	storage := cs.Storage(ct)
	_, err := storage.SwapRemove(ai, ci)
	return err
}
