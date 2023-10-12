// Package store allows for the saving and retrieving of an entity's component data, the creation and destruction of
// entities, and the mapping of archetype IDs to component sets.

package store

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rs/zerolog"
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

var _ IManager = &Manager{}

type Manager struct {
	store  storage.WorldStorage
	logger *log.Logger
}

func NewStoreManager(store storage.WorldStorage, logger *log.Logger) *Manager {
	return &Manager{
		store:  store,
		logger: logger,
	}
}

func (s *Manager) GetEntitiesForArchID(archID archetype.ID) []entity.ID {
	return s.store.ArchAccessor.Archetype(archID).Entities()
}

func (s *Manager) SearchFrom(filter filter.ComponentFilter, seen int) *storage.ArchetypeIterator {
	return s.store.ArchCompIdxStore.SearchFrom(filter, seen)
}

func (s *Manager) ArchetypeCount() int {
	return s.store.ArchAccessor.Count()
}

func (s *Manager) Close() error {
	return s.store.IO.Close()
}

func (s *Manager) InjectLogger(logger *log.Logger) {
	s.logger = logger
}

func (s *Manager) GetEntity(id entity.ID) (entity.Entity, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return storage.BadEntity, err
	}
	return storage.NewEntity(id, loc), nil
}

func (s *Manager) isValid(id entity.ID) (bool, error) {
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
	return id == s.store.ArchAccessor.Archetype(loc.ArchID).Entities()[loc.CompIndex], nil
}

func (s *Manager) removeAtLocation(id entity.ID, loc entity.Location) error {
	archetype := s.store.ArchAccessor.Archetype(loc.ArchID)
	archetype.SwapRemove(loc.CompIndex)
	err := s.store.CompStore.Remove(loc.ArchID, archetype.Components(), loc.CompIndex)
	if err != nil {
		return err
	}
	if int(loc.CompIndex) < len(archetype.Entities()) {
		swappedID := archetype.Entities()[loc.CompIndex]
		if err := s.store.EntityLocStore.SetLocation(swappedID, loc); err != nil {
			return err
		}
	}
	s.store.EntityMgr.Destroy(id)
	return nil
}

func (s *Manager) RemoveEntity(id entity.ID) error {
	ok, err := s.isValid(id)
	if err != nil {
		s.logger.Debug().Int("entity_id", int(id)).Msg("failed to remove")
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
	s.logger.Debug().Int("entity_id", int(id)).Msg("removed")
	return nil
}

func (s *Manager) CreateEntity(comps ...component.IComponentType) (entity.ID, error) {
	ids, err := s.CreateManyEntities(1, comps...)
	if err != nil {
		return storage.BadID, nil
	}
	return ids[0], nil
}

func (s *Manager) CreateManyEntities(num int, comps ...component.IComponentType) ([]entity.ID, error) {
	archetypeID, err := s.GetArchIDForComponents(comps)
	if err != nil {
		return nil, err
	}
	entities := make([]entity.ID, num)
	for i := range entities {
		e, err := s.createEntityFromArchetypeID(archetypeID)
		if err != nil {
			return nil, err
		}
		entities[i] = e
	}
	return entities, nil
}

func (s *Manager) createEntityFromArchetypeID(archID archetype.ID) (entity.ID, error) {
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
	entity := storage.NewEntity(nextEntityID, entity.NewLocation(archID, componentIndex))
	s.logger.LogEntity(zerolog.DebugLevel, entity, components)
	return nextEntityID, nil
}

func (s *Manager) getEntityLocation(id entity.ID) (entity.Location, error) {
	return s.store.EntityLocStore.GetLocation(id)
}

func (s *Manager) SetComponentForEntity(cType component.IComponentType, id entity.ID, value any) error {
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

func (s *Manager) GetComponentTypesForArchID(archID archetype.ID) []component.IComponentType {
	return s.store.ArchAccessor.Archetype(archID).Components()
}

func (s *Manager) GetComponentTypesForEntity(id entity.ID) ([]component.IComponentType, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return nil, err
	}
	return s.getComponentsForArchetype(loc.ArchID), nil
}

func (s *Manager) GetComponentForEntity(cType component.IComponentType, id entity.ID) (any, error) {
	bz, err := s.GetComponentForEntityInRawJson(cType, id)
	if err != nil {
		return nil, err
	}
	return cType.Decode(bz)
}

func (s *Manager) GetComponentForEntityInRawJson(cType component.IComponentType, id entity.ID) (json.RawMessage, error) {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return nil, err
	}
	return s.store.CompStore.Storage(cType).Component(loc.ArchID, loc.CompIndex)
}

func (s *Manager) RegisterComponents([]component.IComponentType) error {
	return nil
}

func (s *Manager) getComponentsForArchetype(archID archetype.ID) []component.IComponentType {
	return s.store.ArchAccessor.Archetype(archID).Components()
}

func (s *Manager) hasDuplicates(components []component.IComponentType) bool {
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

func (s *Manager) insertArchetype(components []component.IComponentType) archetype.ID {
	s.store.ArchCompIdxStore.Push(components)
	archID := archetype.ID(s.store.ArchAccessor.Count())

	s.store.ArchAccessor.PushArchetype(archID, components)
	s.logger.Debug().Int("archetype_id", int(archID)).Msg("created")
	return archID
}

func (s *Manager) GetArchIDForComponents(components []component.IComponentType) (archetype.ID, error) {
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

func (s *Manager) transferArchetype(from, to archetype.ID, idx component.Index) (component.Index, error) {
	if from == to {
		return idx, nil
	}
	fromArch := s.store.ArchAccessor.Archetype(from)
	toArch := s.store.ArchAccessor.Archetype(to)

	// move entity id
	id := fromArch.SwapRemove(idx)
	toArch.PushEntity(id)
	err := s.store.EntityLocStore.Insert(id, to, component.Index(len(toArch.Entities())-1))
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
	return component.Index(len(toArch.Entities()) - 1), nil
}

func (s *Manager) AddComponentToEntity(cType component.IComponentType, id entity.ID) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}

	currComponents := s.getComponentsForArchetype(loc.ArchID)
	if component.Contains(currComponents, cType) {
		return storage.ErrorComponentAlreadyOnEntity
	}
	targetComponents := append(currComponents, cType)
	targetArchID, err := s.GetArchIDForComponents(targetComponents)
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

func (s *Manager) RemoveComponentFromEntity(cType component.IComponentType, id entity.ID) error {
	loc, err := s.getEntityLocation(id)
	if err != nil {
		return err
	}

	currComponents := s.getComponentsForArchetype(loc.ArchID)
	if !component.Contains(currComponents, cType) {
		return storage.ErrorComponentNotOnEntity
	}
	targetComponents := make([]component.IComponentType, 0, len(currComponents)-1)
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
	newCompIndex, err := s.transferArchetype(loc.ArchID, targetArchID, loc.CompIndex)
	if err != nil {
		return err
	}
	loc.ArchID = targetArchID
	loc.CompIndex = newCompIndex
	return s.store.EntityLocStore.SetLocation(id, loc)
}
