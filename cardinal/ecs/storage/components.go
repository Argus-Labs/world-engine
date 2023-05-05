package storage

import (
	"context"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

// ComponentIndex represents the Index of component in an archetype.
type ComponentIndex int

// Components is a structure that facilitates the storage and retrieval of component data.
// TODO: this kinda sucks.. could prob refactor this and make it prettier for devs.
type Components struct {
	Store ComponentStorageManager
}

// NewComponents creates a new empty structure that stores data of components.
func NewComponents(store ComponentStorageManager) Components {
	return Components{
		Store: store,
	}
}

// PushRawComponents pushes components to storage, and returns the component index.
func (cs *Components) PushRawComponents(comps []*anypb.Any, archetypeIndex ArchetypeIndex) (ComponentIndex, error) {
	// TODO(technicallyty): get index!!
	ctx := context.Background()
	idx, err := cs.Store.GetNextIndex(ctx, archetypeIndex)
	if err != nil {
		return 0, err
	}
	for _, c := range comps {
		v := cs.Store.GetComponentStorage(component.ID(c))
		err := v.PushRawComponent(c, archetypeIndex, idx)
		if err != nil {
			return 0, err
		}
	}
	return 0, nil
}

// Move moves the bytes of data of the component in the archetype.
func (cs *Components) Move(src ArchetypeIndex, dst ArchetypeIndex) error {
	// TODO(technicallyty): do we need this???
	return nil
}

func (cs *Components) StorageFromAny(anyComp *anypb.Any) ComponentStorage {
	return cs.Store.GetComponentStorage(component.ID(anyComp))
}

func (cs *Components) StorageFromID(id string) ComponentStorage {
	return cs.Store.GetComponentStorage(id)
}

// Remove removes the component from the storage.
func (cs *Components) Remove(ai ArchetypeIndex, comps []*anypb.Any, ci ComponentIndex) error {
	for _, ct := range comps {
		err := cs.remove(ct, ai, ci)
		if err != nil {
			return err
		}
	}
	return nil // TODO(technicallyty): FIX
}

func (cs *Components) remove(ct *anypb.Any, ai ArchetypeIndex, ci ComponentIndex) error {
	storage := cs.StorageFromAny(ct)
	err := storage.RemoveComponent(ai, ci)
	return err
}
