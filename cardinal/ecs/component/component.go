package component

import (
	"encoding/json"

	"pkg.world.dev/world-engine/cardinal/ecs/entityid"
)

type IGetterForComponentsOnEntity interface {
	GetComponentForEntityInRawJson(cType IComponentType, id entityid.ID) (json.RawMessage, error)
	GetComponentForEntity(cType IComponentType, id entityid.ID) (any, error)
	GetComponentTypesForEntity(id entityid.ID) ([]IComponentType, error)
}

type (
	// Index represents the Index of component in an archetype.
	Index int

	TypeID int

	// IComponentType is a high level representation of a user defined component struct.
	IComponentType interface {
		// SetID sets the ID of this component. It must only be set once
		SetID(TypeID) error
		// ID returns the ID of the component.
		ID() TypeID
		// New returns the marshaled bytes of the default value for the component struct.
		New() ([]byte, error)
		// Name returns the name of the component.
		Name() string

		Decode([]byte) (any, error)
		Encode(any) ([]byte, error)
		GetRawJson(getter IGetterForComponentsOnEntity, id entityid.ID) (json.RawMessage, error)
	}
)

// Contains returns true if the given slice of components contains the given component. Components are the same if they
// have the same ID.
func Contains(components []IComponentType, cType IComponentType) bool {
	for _, c := range components {
		if cType.ID() == c.ID() {
			return true
		}
	}
	return false
}
