// Package store allows for the saving and retrieving of an entity's component data, the creation and destruction of
// entities, and the mapping of archetype IDs to component sets.

package store

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type Manager struct {
	store  storage.WorldStorage
	logger interfaces.IWorldLogger
}

func NewStoreManager(store storage.WorldStorage, logger *log.Logger) interfaces.IStoreManager {
	return &Manager{
		store:  store,
		logger: logger,
	}
}

func (s *Manager) GetArchAccessor() interfaces.ArchetypeAccessor {
	return s.store.ArchAccessor
}

func (s *Manager) GetArchCompIdxStore() interfaces.ArchetypeComponentIndex {
	return s.store.ArchCompIdxStore
}

func (s *Manager) Close() error {
	return s.store.IO.Close()
}

func (s *Manager) InjectLogger(logger interfaces.IWorldLogger) {
	s.logger = logger
}

func (s *Manager) GetEntity(id interfaces.EntityID) (interfaces.IEntity, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return &storage.BadEntity, err
	}
	return storage.NewEntity(id, loc), nil
}

func (s *Manager) isValid(id interfaces.EntityID) (bool, error) {
	if id == storage.BadID {
		return false, errors.New("invalid id: id is the bad ID sentinel")
	}
	ok, err := s.store.EntityLocStore.ContainsEntity(id)
	if err != nil {
		return false, fmt.Errorf("invalid id: failed to find the entity in the entity location store: %w", err)
	}
	if !ok {
		return false, fmt.Errorf("invalid id: id is not in the entity location store")
	}
	loc, err := s.store.EntityLocStore.GetLocation(id)
	if err != nil {
		return false, fmt.Errorf("invalid id: failed to get the entity's location")
	}
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return id == s.store.ArchAccessor.Archetype(loc.GetArchID()).Entities()[loc.GetCompIndex()], nil
}

func (s *Manager) removeAtLocation(id interfaces.EntityID, loc interfaces.ILocation) error {
	archetype := s.store.ArchAccessor.Archetype(loc.GetArchID())
	archetype.SwapRemove(loc.GetCompIndex())
	err := s.store.CompStore.Remove(loc.GetArchID(), archetype.Components(), loc.GetCompIndex())
	if err != nil {
		return err
	}
	if int(loc.GetCompIndex()) < len(archetype.Entities()) {
		swappedID := archetype.Entities()[loc.GetCompIndex()]
		if err := s.store.EntityLocStore.SetLocation(swappedID, loc); err != nil {
			return err
		}
	}
	s.store.EntityMgr.Destroy(id)
	return nil
}

func (s *Manager) RemoveEntity(id interfaces.EntityID) error {
	ok, err := s.isValid(id)
	if err != nil {
		s.logger.GetZeroLogger().Debug().Int("entity_id", int(id)).Msg("failed to remove")
		return err
	}
	if ok {
		loc, err := s.store.EntityLocStore.GetLocation(id)
		if err != nil {
			return err
		}
		if err := s.store.EntityLocStore.Remove(id); err != nil {
			return err
		}
		if err := s.removeAtLocation(id, loc); err != nil {
			return err
		}
	}
	s.logger.GetZeroLogger().Debug().Int("entity_id", int(id)).Msg("removed")
	return nil
}

func (s *Manager) CreateEntity(comps ...interfaces.IComponentType) (interfaces.EntityID, error) {
	ids, err := s.CreateManyEntities(1, comps...)
	if err != nil {
		return storage.BadID, nil
	}
	return ids[0], nil
}

func (s *Manager) CreateManyEntities(num int, comps ...interfaces.IComponentType) ([]interfaces.EntityID, error) {
	archetypeID, err := s.GetArchIDForComponents(comps)
	if err != nil {
		return nil, err
	}
	entities := make([]interfaces.EntityID, num)
	for i := range entities {
		e, err := s.createEntityFromArchetypeID(archetypeID)
		if err != nil {
			return nil, err
		}
		entities[i] = e
	}
	return entities, nil
}

func (s *Manager) createEntityFromArchetypeID(archID interfaces.ArchetypeID) (interfaces.EntityID, error) {
	nextEntityID, err := s.store.EntityMgr.NewEntity()
	if err != nil {
		return storage.BadID, err
	}
	archetype := s.store.ArchAccessor.Archetype(archID)
	components := archetype.Components()
	componentIndex, err := s.store.CompStore.PushComponents(components, archID)
	if err != nil {
		return storage.BadID, err
	}
	if err := s.store.EntityLocStore.Insert(nextEntityID, archID, componentIndex); err != nil {
		return storage.BadID, err
	}
	archetype.PushEntity(nextEntityID)
	newEntity := storage.NewEntity(nextEntityID, entity.NewLocation(archID, componentIndex))
	s.logger.LogEntity(zerolog.DebugLevel, newEntity, components)
	return nextEntityID, nil
}

func (s *Manager) getEntityLocation(id interfaces.EntityID) (interfaces.ILocation, error) {
	return s.store.EntityLocStore.GetLocation(id)
}

func (s *Manager) SetComponentForEntity(cType interfaces.IComponentType, id interfaces.EntityID, value any) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}
	bz, err := cType.Encode(value)
	if err != nil {
		return err
	}
	return s.store.CompStore.Storage(cType).SetComponent(loc.GetArchID(), loc.GetCompIndex(), bz)
}

func (s *Manager) GetComponentTypesForArchID(archID interfaces.ArchetypeID) []interfaces.IComponentType {
	return s.store.ArchAccessor.Archetype(archID).Components()
}

func (s *Manager) GetComponentTypesForEntity(id interfaces.EntityID) ([]interfaces.IComponentType, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return nil, err
	}
	return s.getComponentsForArchetype(loc.GetArchID()), nil
}

func (s *Manager) GetComponentForEntity(cType interfaces.IComponentType, id interfaces.EntityID) (any, error) {
	bz, err := s.GetComponentForEntityInRawJson(cType, id)
	if err != nil {
		return nil, err
	}
	return cType.Decode(bz)
}

func (s *Manager) GetComponentForEntityInRawJson(cType interfaces.IComponentType, id interfaces.EntityID) (json.RawMessage, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return nil, err
	}
	return s.store.CompStore.Storage(cType).Component(loc.GetArchID(), loc.GetCompIndex())
}

func (s *Manager) getComponentsForArchetype(archID interfaces.ArchetypeID) []interfaces.IComponentType {
	return s.store.ArchAccessor.Archetype(archID).Components()
}

func (s *Manager) hasDuplicates(components []interfaces.IComponentType) bool {
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

func (s *Manager) insertArchetype(components []interfaces.IComponentType) interfaces.ArchetypeID {
	s.store.ArchCompIdxStore.Push(components)
	archID := interfaces.ArchetypeID(s.store.ArchAccessor.Count())

	s.store.ArchAccessor.PushArchetype(archID, components)
	s.logger.GetZeroLogger().Debug().Int("archetype_id", int(archID)).Msg("created")
	return archID
}

func (s *Manager) GetArchIDForComponents(components []interfaces.IComponentType) (interfaces.ArchetypeID, error) {
	if len(components) == 0 {
		return 0, errors.New("entities require at least 1 component")
	}

	if ii := s.store.ArchCompIdxStore.Search(filter.Exact(components...)); ii.HasNext() {
		return ii.Next(), nil
	}

	if s.hasDuplicates(components) {
		return 0, fmt.Errorf("duplicate component types: %v", components)
	}

	return s.insertArchetype(components), nil
}

func (s *Manager) transferArchetype(from, to interfaces.ArchetypeID, idx interfaces.ComponentIndex) (interfaces.ComponentIndex, error) {
	if from == to {
		return idx, nil
	}
	fromArch := s.store.ArchAccessor.Archetype(from)
	toArch := s.store.ArchAccessor.Archetype(to)

	// move entity id
	id := fromArch.SwapRemove(idx)
	toArch.PushEntity(id)
	err := s.store.EntityLocStore.Insert(id, to, interfaces.ComponentIndex(len(toArch.Entities())-1))
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

	// creates component if not exists in new set of components
	fromComps := fromArch.Components()
	toComps := toArch.Components()
	for _, componentType := range toComps {
		if !component.Contains(fromComps, componentType) {
			store := s.store.CompStore.Storage(componentType)
			if err := store.PushComponent(componentType, to); err != nil {
				return 0, err
			}
		}
	}

	// move component
	for _, componentType := range fromComps {
		store := s.store.CompStore.Storage(componentType)
		if component.Contains(toComps, componentType) {
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
	return interfaces.ComponentIndex(len(toArch.Entities()) - 1), nil
}

func (s *Manager) AddComponentToEntity(cType interfaces.IComponentType, id interfaces.EntityID) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}

	currComponents := s.getComponentsForArchetype(loc.GetArchID())
	if component.Contains(currComponents, cType) {
		return storage.ErrorComponentAlreadyOnEntity
	}
	targetComponents := append(currComponents, cType)
	targetArchID, err := s.GetArchIDForComponents(targetComponents)
	if err != nil {
		return fmt.Errorf("unable to create new archetype: %w", err)
	}
	newCompIndex, err := s.transferArchetype(loc.GetArchID(), targetArchID, loc.GetCompIndex())
	if err != nil {
		return err
	}

	loc.SetArchID(targetArchID)
	loc.SetCompIndex(newCompIndex)
	return s.store.EntityLocStore.SetLocation(id, loc)
}

func (s *Manager) RemoveComponentFromEntity(cType interfaces.IComponentType, id interfaces.EntityID) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}

	currComponents := s.getComponentsForArchetype(loc.GetArchID())
	if !component.Contains(currComponents, cType) {
		return storage.ErrorComponentNotOnEntity
	}
	targetComponents := make([]interfaces.IComponentType, 0, len(currComponents)-1)
	for _, c2 := range currComponents {
		if c2 == cType {
			continue
		}
		targetComponents = append(targetComponents, c2)
	}
	targetArchID, err := s.GetArchIDForComponents(targetComponents)
	if err != nil {
		return err
	}
	newCompIndex, err := s.transferArchetype(loc.GetArchID(), targetArchID, loc.GetCompIndex())
	if err != nil {
		return err
	}
	loc.SetArchID(targetArchID)
	loc.SetCompIndex(newCompIndex)
	return s.store.EntityLocStore.SetLocation(id, loc)
}
