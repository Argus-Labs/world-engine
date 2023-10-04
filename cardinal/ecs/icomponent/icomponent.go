package icomponent

import "pkg.world.dev/world-engine/cardinal/ecs/component_types"

type (
	// Index represents the Index of component in an archetype.

	// IComponentType is a high level representation of a user defined component struct.
	IComponentType interface {
		// SetID sets the ID of this component. It must only be set once
		SetID(component_types.TypeID) error
		// ID returns the ID of the component.
		ID() component_types.TypeID
		// New returns the marshaled bytes of the default value for the component struct.
		New() ([]byte, error)
		// Name returns the name of the component.
		Name() string

		Decode([]byte) (any, error)
		Encode(any) ([]byte, error)
	}
)
