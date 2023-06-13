package storage

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

type WorldAccessor interface {
	Component(componentType component.IComponentType, archID ArchetypeID, componentIndex ComponentIndex) ([]byte, error)
	SetComponent(component.IComponentType, []byte, ArchetypeID, ComponentIndex) error
	GetLayout(archID ArchetypeID) []component.IComponentType
	GetArchetypeForComponents([]component.IComponentType) ArchetypeID
	TransferArchetype(ArchetypeID, ArchetypeID, ComponentIndex) (ComponentIndex, error)
	Entity(id EntityID) (Entity, error)
	Remove(id EntityID) error
	Valid(id EntityID) (bool, error)
	Archetype(ArchetypeID) ArchetypeStorage
	SetEntityLocation(id EntityID, location Location) error
}

var _ EntityManager = &entityMgrImpl{}

func NewEntityManager() EntityManager {
	return &entityMgrImpl{destroyed: make([]EntityID, 0, 256), nextID: 0}
}

type entityMgrImpl struct {
	destroyed []EntityID
	nextID    EntityID
}

func (e *entityMgrImpl) GetNextEntityID() EntityID {
	e.nextID++
	return e.nextID
}

func (e *entityMgrImpl) shrink() {
	e.destroyed = e.destroyed[:len(e.destroyed)-1]
}

func (e *entityMgrImpl) NewEntity() (EntityID, error) {
	if len(e.destroyed) == 0 {
		id := e.GetNextEntityID()
		return id, nil
	}
	newEntity := e.destroyed[(len(e.destroyed) - 1)]
	e.shrink()
	return newEntity, nil
}

func (e *entityMgrImpl) Destroy(id EntityID) {
	e.destroyed = append(e.destroyed, id)
}

type EntityID uint64

// Entity is a struct that contains an EntityID and a location in an archetype.
type Entity struct {
	ID  EntityID
	Loc Location
}

func NewEntity(id EntityID, loc Location) Entity {
	return Entity{
		ID:  id,
		Loc: loc,
	}
}

var (
	BadID     EntityID = math.MaxUint64
	BadEntity Entity   = Entity{BadID, Location{}}
)

// Get returns the component from the entity
func Get[T any](w WorldAccessor, id EntityID, cType component.IComponentType) (*T, error) {
	e, err := w.Entity(id)
	if err != nil {
		return nil, err
	}
	compBz, err := e.Component(w, cType)
	if err != nil {
		return nil, err
	}
	return Decode[*T](compBz)
}

// Add adds the component to the entity.
func Add[T any](w WorldAccessor, id EntityID, cType component.IComponentType, component *T) error {
	e, err := w.Entity(id)
	if err != nil {
		return err
	}
	bz, err := Encode(component)
	if err != nil {
		return err
	}
	e.AddComponent(w, cType, bz)
	return nil
}

// Set sets the component of the entity.
func Set[T any](w WorldAccessor, id EntityID, ctype component.IComponentType, component *T) error {
	e, err := w.Entity(id)
	if err != nil {
		return err
	}
	bz, err := Encode(component)
	if err != nil {
		return err
	}
	e.SetComponent(w, ctype, bz)
	return nil
}

// SetValue sets the value of the component.
func SetValue[T any](w WorldAccessor, id EntityID, ctype component.IComponentType, value T) error {
	c, err := Get[T](w, id, ctype)
	if err != nil {
		return err
	}
	*c = value
	return nil
}

// Remove removes the component from the entity.
func Remove[T any](w WorldAccessor, id EntityID, ctype component.IComponentType) error {
	e, err := w.Entity(id)
	if err != nil {
		return err
	}
	return e.RemoveComponent(w, ctype)
}

// Valid returns true if the entity is valid.
func Valid(w WorldAccessor, id EntityID) (bool, error) {
	if id == BadID {
		return false, nil
	}
	e, err := w.Entity(id)
	if err != nil {
		return false, err
	}
	ok, err := e.Valid(w)
	return ok, err
}

// EntityID returns the Entity.
func (e Entity) EntityID() EntityID {
	return e.ID
}

// Component returns the component.
func (e Entity) Component(w WorldAccessor, cType component.IComponentType) ([]byte, error) {
	c := e.Loc.CompIndex
	a := e.Loc.ArchID
	return w.Component(cType, a, c)
}

// SetComponent sets the component.
func (e Entity) SetComponent(w WorldAccessor, cType component.IComponentType, component []byte) error {
	c := e.Loc.CompIndex
	a := e.Loc.ArchID
	return w.SetComponent(cType, component, a, c)
}

var (
	ErrorComponentAlreadyOnEntity = errors.New("component already on entity")
	ErrorComponentNotOnEntity     = errors.New("component not on entity")
)

// AddComponent adds the component to the Ent.
func (e Entity) AddComponent(w WorldAccessor, cType component.IComponentType, components ...[]byte) error {
	if len(components) > 1 {
		panic("AddComponent: component argument must be a single value")
	}
	if e.HasComponent(w, cType) {
		return ErrorComponentAlreadyOnEntity
	}

	c := e.Loc.CompIndex
	a := e.Loc.ArchID

	baseLayout := w.GetLayout(a)
	targetArc := w.GetArchetypeForComponents(append(baseLayout, cType))
	if _, err := w.TransferArchetype(a, targetArc, c); err != nil {
		return err
	}

	ent, err := w.Entity(e.ID)
	if err != nil {
		return err
	}
	w.SetEntityLocation(e.ID, ent.Loc)

	if len(components) == 1 {
		e.SetComponent(w, cType, components[0])
	}
	return nil
}

// RemoveComponent removes the component from the Ent.
func (e Entity) RemoveComponent(w WorldAccessor, cType component.IComponentType) error {
	// if the entity doesn't even have this component, we can just return.
	if !e.Archetype(w).Layout().HasComponent(cType) {
		return ErrorComponentNotOnEntity
	}

	c := e.Loc.CompIndex
	a := e.Loc.ArchID

	baseLayout := w.GetLayout(a)
	targetLayout := make([]component.IComponentType, 0, len(baseLayout)-1)
	for _, c2 := range baseLayout {
		if c2 == cType {
			continue
		}
		targetLayout = append(targetLayout, c2)
	}

	targetArc := w.GetArchetypeForComponents(targetLayout)
	compIndex, err := w.TransferArchetype(e.Loc.ArchID, targetArc, c)
	if err != nil {
		return err
	}

	ent, err := w.Entity(e.ID)
	if err != nil {
		return err
	}
	ent.Loc.ArchID = targetArc
	ent.Loc.CompIndex = compIndex
	w.SetEntityLocation(e.ID, ent.Loc)
	return nil
}

// Remove removes the entity from the world.
func (e Entity) Remove(w WorldAccessor) error {
	return w.Remove(e.ID)
}

// Valid returns true if the entity is valid.
func (e Entity) Valid(w WorldAccessor) (bool, error) {
	ok, err := w.Valid(e.ID)
	return ok, err
}

// Archetype returns the archetype.
func (e Entity) Archetype(w WorldAccessor) ArchetypeStorage {
	a := e.Loc.ArchID
	return w.Archetype(a)
}

// HasComponent returns true if the Ent has the given component type.
func (e Entity) HasComponent(w WorldAccessor, componentType component.IComponentType) bool {
	return e.Archetype(w).Layout().HasComponent(componentType)
}

func (e Entity) StringXY(w WorldAccessor) string {
	var out bytes.Buffer
	out.WriteString("Entity: {")
	out.WriteString(e.StringXX())
	out.WriteString(", ")
	out.WriteString(e.Archetype(w).Layout().String())
	out.WriteString(", Valid: ")
	ok, _ := e.Valid(w)
	out.WriteString(fmt.Sprintf("%v", ok))
	out.WriteString("}")
	return out.String()
}

func (e Entity) StringXX() string {
	return fmt.Sprintf("ID: %d, Loc: %+v", e.ID, e.Loc)
}

var _ StateStorage = &stateStorageImpl{}

func NewStateStorage() StateStorage {
	return &stateStorageImpl{
		data: map[string][]byte{},
	}
}

type stateStorageImpl struct {
	data map[string][]byte
}

func (s stateStorageImpl) Save(key string, data []byte) error {
	s.data[key] = data
	return nil
}

func (s stateStorageImpl) Load(key string) (data []byte, ok bool, err error) {
	buf, ok := s.data[key]
	return buf, ok, nil
}
