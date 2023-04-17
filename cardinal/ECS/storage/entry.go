package storage

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/argus-labs/cardinal/ECS/component"
	"github.com/argus-labs/cardinal/ECS/entity"
)

// Entry is a struct that contains an Ent and a location in an archetype.
type Entry struct {
	ID  entity.ID
	Ent Entity
	Loc *Location // TODO(technicallyty): this definitely doesnt need to be a pointer...
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
	compBz := e.Component(w, cType)
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
func Valid(w WorldAccessor, e *Entry) bool {
	if e == nil {
		return false
	}
	return e.Valid(w)
}

// Id returns the Ent ID.
func (e *Entry) Id() entity.ID {
	return e.ID
}

// Entity returns the Entity.
func (e *Entry) Entity() Entity {
	return e.Ent
}

// Component returns the component.
func (e *Entry) Component(w WorldAccessor, cType component.IComponentType) []byte {
	c := e.Loc.CompIndex
	a := e.Loc.ArchIndex
	return w.Component(cType, a, c)
}

// SetComponent sets the component.
func (e *Entry) SetComponent(w WorldAccessor, cType component.IComponentType, component []byte) {
	c := e.Loc.CompIndex
	a := e.Loc.ArchIndex
	w.SetComponent(cType, component, a, c)
}

// AddComponent adds the component to the Ent.
func (e *Entry) AddComponent(w WorldAccessor, cType component.IComponentType, components ...[]byte) {
	if len(components) > 1 {
		panic("AddComponent: component argument must be a single value")
	}
	if !e.HasComponent(w, cType) {
		c := e.Loc.CompIndex
		a := e.Loc.ArchIndex

		baseLayout := w.GetLayout(a)
		targetArc := w.GetArchetypeForComponents(append(baseLayout, cType))
		w.TransferArchetype(a, targetArc, c)

		w.SetEntryLocation(e.ID, *w.Entry(e.Ent).Loc)
		//e.SetLocation(w.Entry(e.Ent).Loc)
	}
	if len(components) == 1 {
		e.SetComponent(w, cType, components[0])
	}
}

// RemoveComponent removes the component from the Ent.
func (e *Entry) RemoveComponent(w WorldAccessor, cType component.IComponentType) {
	// if the entry doesn't even have this component, we can just return.
	if !e.Archetype(w).Layout().HasComponent(cType) {
		return
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
	w.TransferArchetype(e.Loc.ArchIndex, targetArc, c)

	w.SetEntryLocation(e.ID, *w.Entry(e.Ent).Loc)
	// e.SetLocation(w.Entry(e.Ent).Loc)
}

// Remove removes the Ent from the world.
func (e *Entry) Remove(w WorldAccessor) {
	w.Remove(e.Ent)
}

// Valid returns true if the entry is valid.
func (e *Entry) Valid(w WorldAccessor) bool {
	return w.Valid(e.Ent)
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
	out.WriteString(fmt.Sprintf("%v", e.Valid(w)))
	out.WriteString("}")
	return out.String()
}
