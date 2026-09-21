package internal

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/rotisserie/eris"
)

// SingletonRow is the plugin's one bookkeeping entity: the contact baseline and the shape
// store. Spelled out with concrete fields so the wire generator sees the components, and
// registered as an archetype by Plugin.Register since no search declares the whole row.
type SingletonRow struct {
	Tag      cardinal.WithComponent[component.PhysicsSingletonTag]
	Contacts cardinal.WithComponent[component.ActiveContacts]
	Store    cardinal.WithComponent[component.ShapeStore]
}

// SingletonSearch finds the singleton by its tag alone, never by the whole row. A snapshot
// written before a component joined SingletonRow restores a singleton without it, and a search
// for the full row would not recognise that entity: the plugin would create a second singleton
// beside it and read an empty contact baseline out of the new one, replaying every live contact
// as a fresh Begin. EnsureSingleton fills in what is missing instead.
type SingletonSearch = cardinal.Contains[struct {
	Tag cardinal.WithComponent[component.PhysicsSingletonTag]
}]

// EnsureSingleton returns the singleton entity, creating it when none exists and adding any
// component it does not already carry. The plugin calls it from Init and every PreUpdate so a
// restore that skips Init still has one, and the store API calls it so a game Init system that
// runs before the plugin's can keep shapes.
func EnsureSingleton(s *SingletonSearch) cardinal.Entity {
	row, err := s.Iter().Single()
	switch {
	case err == nil:
	case errors.Is(err, cardinal.ErrSingleMultipleResult):
		panic(eris.New("physics2d: more than one physics singleton entity (PhysicsSingletonTag)"))
	case errors.Is(err, cardinal.ErrSingleNoResult):
		row = s.Create()
	default:
		panic(eris.Wrap(err, "physics2d: singleton.Iter().Single()"))
	}
	if !row.Has[component.ActiveContacts]() {
		row.Set(component.ActiveContacts{})
	}
	if !row.Has[component.ShapeStore]() {
		row.Set(component.ShapeStore{})
	}
	return row
}
