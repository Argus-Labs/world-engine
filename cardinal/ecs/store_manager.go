package ecs

import (
	"errors"
	"fmt"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

type StoreManager struct {
	store storage.WorldStorage
}

func NewStoreManager(store storage.WorldStorage) *StoreManager {
	return &StoreManager{store: store}
}

func (s *StoreManager) getEntity(id storage.EntityID) (storage.Entity, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return storage.BadEntity, err
	}
	return storage.NewEntity(id, loc), nil
}

func (s *StoreManager) getEntityLocation(id storage.EntityID) (storage.Location, error) {
	return s.store.EntityLocStore.GetLocation(id)
}

func (s *StoreManager) SetComponentForEntity(cType IComponentType, id storage.EntityID, value any) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}
	bz, err := cType.Encode(value)
	if err != nil {
		return err
	}
	return s.store.CompStore.Storage(cType).SetComponent(loc.ArchID, loc.CompIndex, bz)
}

func (s *StoreManager) GetComponentForEntity(cType IComponentType, id storage.EntityID) (any, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return nil, err
	}
	bz, err := s.store.CompStore.Storage(cType).Component(loc.ArchID, loc.CompIndex)
	if err != nil {
		return nil, err
	}
	return cType.Decode(bz)
}

func (s *StoreManager) getComponentsForArchetype(archID storage.ArchetypeID) *storage.Layout {
	return s.store.ArchAccessor.Archetype(archID).Layout()
}

func (s *StoreManager) hasDuplicates(components []IComponentType) bool {
	// check if there are duplicate values inside component slice
	for i := 0; i < len(components); i++ {
		for j := i + 1; j < len(components); j++ {
			if components[i] == components[j] {
				return true
			}
		}
	}
	return false
}

func (s *StoreManager) insertArchetype(layout *storage.Layout) storage.ArchetypeID {
	s.store.ArchCompIdxStore.Push(layout)
	archID := storage.ArchetypeID(s.store.ArchAccessor.Count())

	s.store.ArchAccessor.PushArchetype(archID, layout)
	// TODO: Decide on a good way to get this logger object into this StoreManager
	// s.logger.Debug().Int("archetype_id", int(archID)).Msg("created")
	return archID
}

func (s *StoreManager) getArchetypeIDForComponents(components []IComponentType) (storage.ArchetypeID, error) {
	if len(components) == 0 {
		return 0, errors.New("entities require at least 1 component")
	}

	if ii := s.store.ArchCompIdxStore.Search(filter.Exact(components...)); ii.HasNext() {
		return ii.Next(), nil
	}

	if s.hasDuplicates(components) {
		return 0, fmt.Errorf("duplicate component types: %v", components)
	}

	return s.insertArchetype(storage.NewLayout(components)), nil
}

func (s *StoreManager) transferArchetype(from, to storage.ArchetypeID, idx storage.ComponentIndex) (storage.ComponentIndex, error) {
	if from == to {
		return idx, nil
	}
	fromArch := s.store.ArchAccessor.Archetype(from)
	toArch := s.store.ArchAccessor.Archetype(to)

	// move entity id
	id := fromArch.SwapRemove(idx)
	toArch.PushEntity(id)
	err := s.store.EntityLocStore.Insert(id, to, storage.ComponentIndex(len(toArch.Entities())-1))
	if err != nil {
		return 0, err
	}

	if len(fromArch.Entities()) > int(idx) {
		movedID := fromArch.Entities()[idx]
		err := s.store.EntityLocStore.Insert(movedID, from, idx)
		if err != nil {
			return 0, err
		}
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Layout()
	toLayout := toArch.Layout()
	for _, componentType := range toLayout.Components() {
		if !fromLayout.HasComponent(componentType) {
			store := s.store.CompStore.Storage(componentType)
			if err := store.PushComponent(componentType, to); err != nil {
				return 0, err
			}
		}
	}

	// move component
	for _, componentType := range fromLayout.Components() {
		store := s.store.CompStore.Storage(componentType)
		if toLayout.HasComponent(componentType) {
			if err := store.MoveComponent(from, idx, to); err != nil {
				return 0, err
			}
		} else {
			_, err := store.SwapRemove(from, idx)
			if err != nil {
				return 0, err
			}
		}
	}
	err = s.store.CompStore.Move(from, to)
	if err != nil {
		return 0, err
	}
	return storage.ComponentIndex(len(toArch.Entities()) - 1), nil
}

func (s *StoreManager) AddComponentToEntity(cType IComponentType, id storage.EntityID) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}

	currComponents := s.getComponentsForArchetype(loc.ArchID)
	if currComponents.HasComponent(cType) {
		return storage.ErrorComponentAlreadyOnEntity
	}
	targetComponents := append(currComponents.Components(), cType)
	targetArchID, err := s.getArchetypeIDForComponents(targetComponents)
	if err != nil {
		return fmt.Errorf("unable to create new archetype: %w", err)
	}
	newCompIndex, err := s.transferArchetype(loc.ArchID, targetArchID, loc.CompIndex)
	if err != nil {
		return err
	}

	loc.ArchID = targetArchID
	loc.CompIndex = newCompIndex
	return s.store.EntityLocStore.SetLocation(id, loc)
}

func (s *StoreManager) RemoveComponentFromEntity(cType IComponentType, id storage.EntityID) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}

	currComponents := s.getComponentsForArchetype(loc.ArchID)
	if !currComponents.HasComponent(cType) {
		return storage.ErrorComponentNotOnEntity
	}
	targetComponents := make([]component.IComponentType, 0, len(currComponents.Components())-1)
	for _, c2 := range currComponents.Components() {
		if c2 == cType {
			continue
		}
		targetComponents = append(targetComponents, c2)
	}
	targetArchID, err := s.getArchetypeIDForComponents(targetComponents)
	if err != nil {
		return err
	}
	newCompIndex, err := s.transferArchetype(loc.ArchID, targetArchID, loc.CompIndex)
	if err != nil {
		return err
	}
	loc.ArchID = targetArchID
	loc.CompIndex = newCompIndex
	return s.store.EntityLocStore.SetLocation(id, loc)
}
