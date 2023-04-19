package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
)

// Entry is a struct that contains an Ent and a location in an archetype.
type Entry struct {
	ID  entity.ID
	Ent Entity
	Loc *Location // TODO: this definitely doesnt need to be a pointer...
}

func NewEntry(id entity.ID, e entity.Entity, loc *Location) *Entry {
	return &Entry{
		ID:  id,
		Ent: e,
		Loc: loc,
	}
}

// Get returns the component from the entry
func Get[T any](w WorldAccessor, e *Entry, cType component.IComponentType) (*T, error) {
	var comp *T
	compBz, _ := e.Component(w, cType)
	var buf bytes.Buffer
	buf.Write(compBz)
	dec := gob.NewDecoder(&buf)
	err := dec.Decode(comp)
	return comp, err
}

// Add adds the component to the entry.
func Add[T any](w WorldAccessor, e *Entry, cType component.IComponentType, component *T) error {
	bz, err := Encode(component)
	if err != nil {
		return err
	}
	e.AddComponent(w, cType, bz)
	return nil
}

// Set sets the component of the entry.
func Set[T any](w WorldAccessor, e *Entry, ctype component.IComponentType, component *T) error {
	bz, err := Encode(component)
	if err != nil {
		return err
	}
	e.SetComponent(w, ctype, bz)
	return nil
}

// SetValue sets the value of the component.
func SetValue[T any](w WorldAccessor, e *Entry, ctype component.IComponentType, value T) error {
	c, err := Get[T](w, e, ctype)
	if err != nil {
		return err
	}
	*c = value
	return nil
}

// Remove removes the component from the entry.
func Remove[T any](w WorldAccessor, e *Entry, ctype component.IComponentType) {
	e.RemoveComponent(w, ctype)
}

// Valid returns true if the entry is valid.
func Valid(w WorldAccessor, e *Entry) (bool, error) {
	if e == nil {
		return false, nil
	}
	ok, err := e.Valid(w)
	return ok, err
}

// Id returns the entity ID.
func (e *Entry) Id() entity.ID {
	return e.ID
}

// Entity returns the Entity.
func (e *Entry) Entity() Entity {
	return e.Ent
}

// Component returns the component.
func (e *Entry) Component(w WorldAccessor, cType component.IComponentType) ([]byte, error) {
	c := e.Loc.CompIndex
	a := e.Loc.ArchIndex
	return w.Component(cType, a, c)
}

// SetComponent sets the component.
func (e *Entry) SetComponent(w WorldAccessor, cType component.IComponentType, component []byte) error {
	c := e.Loc.CompIndex
	a := e.Loc.ArchIndex
	return w.SetComponent(cType, component, a, c)
}

// AddComponent adds the component to the Ent.
func (e *Entry) AddComponent(w WorldAccessor, cType component.IComponentType, components ...[]byte) error {
	if len(components) > 1 {
		panic("AddComponent: component argument must be a single value")
	}
	if !e.HasComponent(w, cType) {
		c := e.Loc.CompIndex
		a := e.Loc.ArchIndex

		baseLayout := w.GetLayout(a)
		targetArc := w.GetArchetypeForComponents(append(baseLayout, cType))
		if _, err := w.TransferArchetype(a, targetArc, c); err != nil {
			return err
		}

		ent, err := w.Entry(e.Ent)
		if err != nil {
			return err
		}
		w.SetEntryLocation(e.ID, *ent.Loc)
	}
	if len(components) == 1 {
		e.SetComponent(w, cType, components[0])
	}
	return nil
}

// RemoveComponent removes the component from the Ent.
func (e *Entry) RemoveComponent(w WorldAccessor, cType component.IComponentType) error {
	// if the entry doesn't even have this component, we can just return.
	if !e.Archetype(w).Layout().HasComponent(cType) {
		return nil
	}

	c := e.Loc.CompIndex
	a := e.Loc.ArchIndex

	baseLayout := w.GetLayout(a)
	targetLayout := make([]component.IComponentType, 0, len(baseLayout)-1)
	for _, c2 := range baseLayout {
		if c2 == cType {
			continue
		}
		targetLayout = append(targetLayout, c2)
	}

	targetArc := w.GetArchetypeForComponents(targetLayout)
	if _, err := w.TransferArchetype(e.Loc.ArchIndex, targetArc, c); err != nil {
		return err
	}

	ent, err := w.Entry(e.Ent)
	if err != nil {
		return err
	}
	w.SetEntryLocation(e.ID, *ent.Loc)
	// e.SetLocation(w.Entry(e.Ent).Loc)
	return nil
}

// Remove removes the entity from the world.
func (e *Entry) Remove(w WorldAccessor) error {
	return w.Remove(e.Ent)
}

// Valid returns true if the entry is valid.
func (e *Entry) Valid(w WorldAccessor) (bool, error) {
	ok, err := w.Valid(e.Ent)
	return ok, err
}

// Archetype returns the archetype.
func (e *Entry) Archetype(w WorldAccessor) ArchetypeStorage {
	a := e.Loc.ArchIndex
	return w.Archetype(a)
}

// HasComponent returns true if the Ent has the given component type.
func (e *Entry) HasComponent(w WorldAccessor, componentType component.IComponentType) bool {
	return e.Archetype(w).Layout().HasComponent(componentType)
}

func (e *Entry) String(w WorldAccessor) string {
	var out bytes.Buffer
	out.WriteString("Entry: {")
	out.WriteString(e.Entity().String())
	out.WriteString(", ")
	out.WriteString(e.Archetype(w).Layout().String())
	out.WriteString(", Valid: ")
	ok, _ := e.Valid(w)
	out.WriteString(fmt.Sprintf("%v", ok))
	out.WriteString("}")
	return out.String()
}
