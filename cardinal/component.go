package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

// AnyComponent is implemented by the return value of NewComponentType and is used in RegisterComponents; any
// component created by NewComponentType can be registered with a World object via RegisterComponents.
type AnyComponentType interface {
	Convert() component.IComponentType
	Name() string
}

// ComponentType represents an accessor that can get and set a specific kind of data (T) using an EntityID.
type ComponentType[T any] struct {
	impl *ecs.ComponentType[T]
}

func toIComponentType(ins []AnyComponentType) []component.IComponentType {
	out := make([]component.IComponentType, 0, len(ins))
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
func (c *ComponentType[T]) Convert() component.IComponentType {
	return c.impl
}
