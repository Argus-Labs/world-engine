package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
)

// AnyComponent is implemented by the return value of NewComponentMetaData and is used in RegisterComponents; any
// component created by NewComponentMetaData can be registered with a World object via RegisterComponents.
type AnyComponentType interface {
	Convert() component_metadata.IComponentMetaData
	Name() string
}

// ComponentType represents an accessor that can get and set a specific kind of data (T) using an EntityID.
type ComponentType[T any] struct {
	impl *component_metadata.ComponentMetaData[T]
}

func toIComponentType(ins []AnyComponentType) []component_metadata.IComponentMetaData {
	out := make([]component_metadata.IComponentMetaData, 0, len(ins))
	for _, c := range ins {
		out = append(out, c.Convert())
	}
	return out
}

// Name returns the name of this component.
func (c *ComponentType[T]) Name() string {
	return c.impl.Name()
}

// Convert implements the AnyComponentType interface which allows a ComponentType to be registered
// with a World via RegisterComponents.
func (c *ComponentType[T]) Convert() component_metadata.IComponentMetaData {
	return c.impl
}
