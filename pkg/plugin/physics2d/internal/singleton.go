package internal

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/rotisserie/eris"
)

// SingletonRow is the plugin's one bookkeeping entity: the contact baseline and the shape
// store. Spelled out with concrete fields so the wire generator sees the components.
type SingletonRow struct {
	Tag      cardinal.WithComponent[component.PhysicsSingletonTag]
	Contacts cardinal.WithComponent[component.ActiveContacts]
	Store    cardinal.WithComponent[component.ShapeStore]
}

// SingletonSearch is the Exact search over the singleton.
type SingletonSearch = cardinal.Exact[SingletonRow]

// EnsureSingleton returns the singleton entity, creating it when none exists. The plugin calls
// it from Init and every PreUpdate so a restore that skips Init still has one, and the store
// API calls it so a game Init system that runs before the plugin's can keep shapes.
func EnsureSingleton(s *SingletonSearch) cardinal.Entity {
	row, err := s.Iter().Single()
	if err == nil {
		return row
	}
	if errors.Is(err, cardinal.ErrSingleMultipleResult) {
		panic(eris.New("physics2d: more than one physics singleton entity (PhysicsSingletonTag)"))
	}
	if !errors.Is(err, cardinal.ErrSingleNoResult) {
		panic(eris.Wrap(err, "physics2d: singleton.Iter().Single()"))
	}
	return s.Create()
}
