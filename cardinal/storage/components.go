package storage

import (
	"bytes"
	"encoding/gob"

	"github.com/argus-labs/cardinal/component"
)

// ComponentIndex represents the Index of component in an archetype.
type ComponentIndex int

// Components is a structure that facilitates the storage and retrieval of component data.
// TODO(technicallyty): this kinda sucks.. should probs refactor this.
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
	if _, ok := cs.componentIndices.ComponentIndex(archetypeIndex); !ok {
		cs.componentIndices.SetIndex(archetypeIndex, 0)
	} else {
		cs.componentIndices.IncrementIndex(archetypeIndex)
	}
	idx, _ := cs.componentIndices.ComponentIndex(archetypeIndex)
	return idx, nil
}

// Move moves the bytes of data of the component in the archetype.
func (cs *Components) Move(src ArchetypeIndex, dst ArchetypeIndex) {
	cs.componentIndices.DecrementIndex(src)
	cs.componentIndices.IncrementIndex(dst)
}

// Storage returns the component data storage accessor.
func (cs *Components) Storage(c component.IComponentType) ComponentStorage {
	return cs.store.GetComponentStorage(c.ID())
}

func (cs *Components) GetComponentIndexStorage(c component.IComponentType) ComponentIndexStorage {
	return cs.store.GetComponentIndexStorage(c.ID())
}

// Remove removes the component from the storage.
func (cs *Components) Remove(ai ArchetypeIndex, comps []component.IComponentType, ci ComponentIndex) {
	for _, ct := range comps {
		cs.remove(ct, ai, ci)
	}
	cs.componentIndices.DecrementIndex(ai)
}

func (cs *Components) remove(ct component.IComponentType, ai ArchetypeIndex, ci ComponentIndex) {
	storage := cs.Storage(ct)
	storage.SwapRemove(ai, ci)
}

func MarshalComponent[T any](comp *T) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(*comp)
	return buf.Bytes(), err
}
