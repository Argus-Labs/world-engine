package storage

import "github.com/argus-labs/cardinal/component"

// ComponentIndex represents the index of component in an archetype.
type ComponentIndex int

// Components is a structure that stores data of components.
type Components struct {
	// storages is a slice of component storages. each storage in the slice represents all storages for a given component type.
	// storages are fetched via component type ID.
	//
	// example: if component Foo has ID 1, then storages[1] contains the storage for all components of type Foo.
	store            Storage
	componentIndices ComponentIndexStorage
}

// NewComponents creates a new empty structure that stores data of components.
func NewComponents(store Storage, idxStore ComponentIndexStorage) *Components {
	return &Components{
		store:            store,
		componentIndices: idxStore,
	}
}

// PushComponents stores the new data of the component in the archetype.
func (cs *Components) PushComponents(components []component.IComponentType, archetypeIndex ArchetypeIndex) (ComponentIndex, error) {
	for _, componentType := range components {
		v := cs.store.GetComponentStorage(componentType.ID())
		if v == nil {
			cs.store.InitializeComponentStorage(componentType.ID())
		}
		err := cs.store.GetComponentStorage(componentType.ID()).PushComponent(componentType, archetypeIndex)
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

// Storage returns the pointer to data of the component in the archetype.
func (cs *Components) Storage(c component.IComponentType) ComponentStorage {
	if storage := cs.store.GetComponentStorage(c.ID()); storage != nil {
		return storage
	}
	cs.store.InitializeComponentStorage(c.ID())
	return cs.store.GetComponentStorage(c.ID())
}

// Remove removes the component from the storage.
func (cs *Components) Remove(a *Archetype, ci ComponentIndex) {
	for _, ct := range a.Layout().Components() {
		cs.remove(ct, a.index, ci)
	}
	cs.componentIndices.DecrementIndex(a.index)
}

func (cs *Components) remove(ct component.IComponentType, ai ArchetypeIndex, ci ComponentIndex) {
	storage := cs.Storage(ct)
	storage.SwapRemove(ai, ci)
}
