package storage

import (
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

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

// PushComponents stores the new data of the component in the archetype.
func (cs *Components) PushComponents(components []component.IComponentMetaData, archetypeID archetype.ID) (component.Index, error) {
	for _, componentType := range components {
		v := cs.Store.GetComponentStorage(componentType.ID())
		err := v.PushComponent(componentType, archetypeID)
		if err != nil {
			return 0, err
		}
	}
	idx, err := cs.ComponentIndices.IncrementIndex(archetypeID)
	if err != nil {
		return 0, err
	}
	return idx, err
}

// Move moves the bytes of data of the component in the archetype.
func (cs *Components) Move(src archetype.ID, dst archetype.ID) error {
	err := cs.ComponentIndices.DecrementIndex(src)
	if err != nil {
		return err
	}
	_, err = cs.ComponentIndices.IncrementIndex(dst)
	if err != nil {
		return err
	}
	return nil
}

// Storage returns the component data storage accessor.
func (cs *Components) Storage(c component.IComponentMetaData) ComponentStorage {
	return cs.Store.GetComponentStorage(c.ID())
}

func (cs *Components) GetComponentIndexStorage(c component.IComponentMetaData) ComponentIndexStorage {
	return cs.Store.GetComponentIndexStorage(c.ID())
}

// Remove removes the component from the storage.
func (cs *Components) Remove(ai archetype.ID, comps []component.IComponentMetaData, ci component.Index) error {
	for _, ct := range comps {
		err := cs.remove(ct, ai, ci)
		if err != nil {
			return err
		}
	}
	return cs.ComponentIndices.DecrementIndex(ai)
}

func (cs *Components) remove(ct component.IComponentMetaData, ai archetype.ID, ci component.Index) error {
	storage := cs.Storage(ct)
	_, err := storage.SwapRemove(ai, ci)
	return err
}
