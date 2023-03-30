package storage

import "github.com/argus-labs/cardinal/component"

// ComponentIndex represents the index of component in an archetype.
type ComponentIndex int

// Components is a structure that stores data of components.
type Components struct {
	storages []*SliceStorage
	// TODO: optimize to use slice instead of map for performance
	componentIndices map[ArchetypeIndex]ComponentIndex
}

// NewComponents creates a new empty structure that stores data of components.
func NewComponents() *Components {
	return &Components{
		storages:         make([]*SliceStorage, 512), // TODO: expand as the number of component types increases
		componentIndices: make(map[ArchetypeIndex]ComponentIndex),
	}
}

// PushComponents stores the new data of the component in the archetype.
func (cs *Components) PushComponents(components []component.IComponentType, archetypeIndex ArchetypeIndex) (ComponentIndex, error) {
	for _, componentType := range components {
		if v := cs.storages[componentType.ID()]; v == nil {
			cs.storages[componentType.ID()] = NewSliceStorage()
		}
		err := cs.storages[componentType.ID()].PushComponent(componentType, archetypeIndex)
		if err != nil {
			return 0, err
		}
	}
	if _, ok := cs.componentIndices[archetypeIndex]; !ok {
		cs.componentIndices[archetypeIndex] = 0
	} else {
		cs.componentIndices[archetypeIndex]++
	}
	return cs.componentIndices[archetypeIndex], nil
}

// Move moves the bytes of data of the component in the archetype.
func (cs *Components) Move(src ArchetypeIndex, dst ArchetypeIndex) {
	cs.componentIndices[src]--
	cs.componentIndices[dst]++
}

// SliceStorage returns the pointer to data of the component in the archetype.
func (cs *Components) Storage(c component.IComponentType) *SliceStorage {
	if storage := cs.storages[c.ID()]; storage != nil {
		return storage
	}
	cs.storages[c.ID()] = NewSliceStorage()
	return cs.storages[c.ID()]
}

// Remove removes the component from the storage.
func (cs *Components) Remove(a *Archetype, ci ComponentIndex) {
	for _, ct := range a.Layout().Components() {
		cs.remove(ct, a.index, ci)
	}
	cs.componentIndices[a.index]--
}

func (cs *Components) remove(ct component.IComponentType, ai ArchetypeIndex, ci ComponentIndex) {
	storage := cs.Storage(ct)
	storage.SwapRemove(ai, ci)
}
