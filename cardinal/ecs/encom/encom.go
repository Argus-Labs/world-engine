package encom

import (
	"errors"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

var (
	ErrorReadOnlyEncomStorageCannotChangeState = errors.New("read only entity/component storage cannot change state")
)

type EncomStorage struct {
	store    storage.WorldStorage
	readOnly bool
}

func NewEncomStorage(store storage.WorldStorage) *EncomStorage {
	return &EncomStorage{
		store:    store,
		readOnly: false,
	}
}

func (e *EncomStorage) AsReadOnly() *EncomStorage {
	return &EncomStorage{
		store:    e.store,
		readOnly: true,
	}
}

func (e *EncomStorage) GetEntity(id storage.EntityID) (storage.Entity, error) {
	loc, err := e.store.EntityLocStore.GetLocation(id)
	if err != nil {
		return storage.BadEntity, err
	}
	return storage.NewEntity(id, loc), nil
}

func hasDuplicates(components []component.IComponentType) bool {
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

func (e *EncomStorage) getComponentsForArchetype(archID storage.ArchetypeID) ([]component.IComponentType, error) {
	return e.store.ArchAccessor.Archetype(archID).Layout().Components(), nil
}

func (e *EncomStorage) getArchetypeForComponents(components []component.IComponentType) (storage.ArchetypeID, error) {
	if len(components) == 0 {
		return 0, errors.New("components must not be empty")
	}
	if ii := e.store.ArchCompIdxStore.Search(filter.Exact(components...)); ii.HasNext() {
		return ii.Next(), nil
	}
	if hasDuplicates(components) {
		return 0, errors.New("duplicate component found")
	}
	layout := storage.NewLayout(components)
	e.store.ArchCompIdxStore.Push(layout)
	archID := storage.ArchetypeID(e.store.ArchAccessor.Count())

	e.store.ArchAccessor.PushArchetype(archID, layout)
	log.Logger.Debug().Int("archetype_id", int(archID)).Msg("created")
	return archID, nil
}

func (e *EncomStorage) transferArchetype(from storage.ArchetypeID, to storage.ArchetypeID, idx storage.ComponentIndex) (storage.ComponentIndex, error) {
	if from == to {
		return idx, nil
	}
	fromArch := e.store.ArchAccessor.Archetype(from)
	toArch := e.store.ArchAccessor.Archetype(to)
	id := fromArch.SwapRemove(idx)
	toArch.PushEntity(id)

	err := e.store.EntityLocStore.Insert(id, to, storage.ComponentIndex(len(toArch.Entities())-1))
	if err != nil {
		return 0, err
	}

	if len(fromArch.Entities()) > int(idx) {
		movedID := fromArch.Entities()[idx]
		err := e.store.EntityLocStore.Insert(movedID, from, idx)
		if err != nil {
			return 0, err
		}
	}

	// creates component if not exists in new layout
	fromLayout := fromArch.Layout()
	toLayout := toArch.Layout()
	for _, componentType := range toLayout.Components() {
		if !fromLayout.HasComponent(componentType) {
			store := e.store.CompStore.Storage(componentType)
			if err := store.PushComponent(componentType, to); err != nil {
				return 0, err
			}
		}
	}

	// move component
	for _, componentType := range fromLayout.Components() {
		store := e.store.CompStore.Storage(componentType)
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

	err = e.store.CompStore.Move(from, to)
	if err != nil {
		return 0, err
	}

	return storage.ComponentIndex(len(toArch.Entities()) - 1), nil
}

func (e *EncomStorage) setEntityLocation(id storage.EntityID, loc storage.Location) error {
	if err := e.store.EntityLocStore.SetLocation(id, loc); err != nil {
		return err
	}
	return nil
}

func (e *EncomStorage) RemoveComponentFrom(cType component.IComponentType, id storage.EntityID) error {
	if e.readOnly {
		return ErrorReadOnlyEncomStorageCannotChangeState
	}
	loc, err := e.getLocation(id)
	if err != nil {
		return err
	}
	layout := e.store.ArchAccessor.Archetype(loc.ArchID).Layout()
	if !layout.HasComponent(cType) {
		return storage.ErrorComponentNotOnEntity
	}

	baseLayout := layout.Components()
	targetLayout := make([]component.IComponentType, 0, len(baseLayout)-1)
	for _, currCType := range baseLayout {
		if currCType == cType {
			continue
		}
		targetLayout = append(targetLayout, currCType)
	}

	targetArch, err := e.getArchetypeForComponents(targetLayout)
	if err != nil {
		return err
	}
	newCompIndex, err := e.transferArchetype(loc.ArchID, targetArch, loc.CompIndex)

	return e.setEntityLocation(id, storage.NewLocation(targetArch, newCompIndex))
}

func (e *EncomStorage) AddComponentToEntity(cType component.IComponentType, id storage.EntityID) error {
	if e.readOnly {
		return ErrorReadOnlyEncomStorageCannotChangeState
	}
	loc, err := e.getLocation(id)
	if err != nil {
		return err
	}
	layout := e.store.ArchAccessor.Archetype(loc.ArchID).Layout()
	if layout.HasComponent(cType) {
		return storage.ErrorComponentAlreadyOnEntity
	}
	components := layout.Components()
	components = append(components, cType)

	targetArch, err := e.getArchetypeForComponents(components)
	if err != nil {
		return err
	}

	newCompID, err := e.transferArchetype(loc.ArchID, targetArch, loc.CompIndex)
	if err != nil {
		return err
	}

	newLoc := storage.NewLocation(targetArch, newCompID)
	if err := e.setEntityLocation(id, newLoc); err != nil {
		return err
	}
	return nil
}

func (e *EncomStorage) SetComponent(cType component.IComponentType, id storage.EntityID, value any) error {
	if e.readOnly {
		return ErrorReadOnlyEncomStorageCannotChangeState
	}
	loc, err := e.getLocation(id)
	if err != nil {
		return err
	}

	bz, err := cType.Marshal(value)
	if err != nil {
		return err
	}
	return e.store.CompStore.Storage(cType).SetComponent(loc.ArchID, loc.CompIndex, bz)
}

func (e *EncomStorage) getLocation(id storage.EntityID) (storage.Location, error) {
	return e.store.EntityLocStore.GetLocation(id)
}

func (e *EncomStorage) GetComponent(cType component.IComponentType, id storage.EntityID) (any, error) {
	loc, err := e.getLocation(id)
	if err != nil {
		return storage.BadEntity, err
	}
	bz, err := e.store.CompStore.Storage(cType).Component(loc.ArchID, loc.CompIndex)
	if err != nil {
		return nil, err
	}
	return cType.Unmarshal(bz)
}

func (e *EncomStorage) GetComponentsForEntity(id storage.EntityID) ([]component.IComponentType, error) {
	entity, err := e.GetEntity(id)
	if err != nil {
		return nil, err
	}
	return e.getComponentsForArchetype(entity.Loc.ArchID)
}

func (e *EncomStorage) isValid(id storage.EntityID) (bool, error) {
	if id == storage.BadID {
		return false, nil
	}
	ok, err := e.store.EntityLocStore.ContainsEntity(id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	loc, err := e.getLocation(id)
	if err != nil {
		return false, err
	}
	a := loc.ArchID
	c := loc.CompIndex
	// If the version of the entity is not the same as the version of the archetype,
	// the entity is invalid (it means the entity is already destroyed).
	return id == e.store.ArchAccessor.Archetype(a).Entities()[c], nil
}

func (e *EncomStorage) removeAtLocation(id storage.EntityID, loc storage.Location) error {
	archID := loc.ArchID
	componentIndex := loc.CompIndex
	archetype := e.store.ArchAccessor.Archetype(archID)
	archetype.SwapRemove(componentIndex)
	err := e.store.CompStore.Remove(archID, archetype.Layout().Components(), componentIndex)
	if err != nil {
		return err
	}
	if int(componentIndex) < len(archetype.Entities()) {
		swappedID := archetype.Entities()[componentIndex]
		if err := e.store.EntityLocStore.SetLocation(swappedID, loc); err != nil {
			return err
		}
	}
	e.store.EntityMgr.Destroy(id)
	return nil
}

func (e *EncomStorage) RemoveEntity(id storage.EntityID) error {
	if e.readOnly {
		return ErrorReadOnlyEncomStorageCannotChangeState
	}
	if ok, err := e.isValid(id); err != nil {
		log.Logger.Debug().Int("entity_id", int(id)).Msg("failed to remove")
		return err
	} else if ok {
		loc, err := e.getLocation(id)
		if err != nil {
			return err
		}
		if err := e.store.EntityLocStore.Remove(id); err != nil {
			return err
		}
		if err := e.removeAtLocation(id, loc); err != nil {
			return err
		}
	}
	log.Logger.Debug().Int("entity_id", int(id)).Msg("removed")
	return nil
}

func (e *EncomStorage) createEntity(archetypeID storage.ArchetypeID) (storage.EntityID, error) {
	nextEntityID, err := e.store.EntityMgr.NewEntity()
	if err != nil {
		return 0, err
	}
	archetype := e.store.ArchAccessor.Archetype(archetypeID)
	componentIndex, err := e.store.CompStore.PushComponents(archetype.Layout().Components(), archetypeID)
	if err != nil {
		return 0, err
	}
	err = e.store.EntityLocStore.Insert(nextEntityID, archetypeID, componentIndex)
	if err != nil {
		return 0, err
	}
	archetype.PushEntity(nextEntityID)

	// TODO: Fix entity creation logging in encom.EncomStorage
	//	log.Logger.LogEntity(w, zerolog.DebugLevel, nextEntityID)
	return nextEntityID, err

}

func (e *EncomStorage) CreateManyEntities(num int, components ...component.IComponentType) ([]storage.EntityID, error) {
	archetypeID, err := e.getArchetypeForComponents(components)
	if err != nil {
		return nil, err
	}
	entities := make([]storage.EntityID, 0, num)
	for i := 0; i < num; i++ {
		e, err := e.createEntity(archetypeID)
		if err != nil {
			return nil, err
		}

		entities = append(entities, e)
	}
	return entities, nil
}

func (e *EncomStorage) CreateEntity(components ...component.IComponentType) (storage.EntityID, error) {
	entities, err := e.CreateManyEntities(1, components...)
	if err != nil {
		return 0, err
	}
	return entities[0], nil
}
