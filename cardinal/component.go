package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

// AnyComponentType is implemented by the return value of NewComponentType and is used in RegisterComponents; any
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

// NewComponentType creates a new instance of a ComponentType. When this ComponentType is added to an EntityID,
// the zero value of the T struct will be saved with the entity.
func NewComponentType[T any]() *ComponentType[T] {
	return &ComponentType[T]{
		impl: ecs.NewComponentType[T](),
	}
}

// NewComponentTypeWithDefault creates a new instance of a ComponentType. When this ComponentType is added to
// an EntityID, the defaultVal will be saved with the entity.
func NewComponentTypeWithDefault[T any](defaultVal T) *ComponentType[T] {
	return &ComponentType[T]{
		impl: ecs.NewComponentType[T](ecs.WithDefault(defaultVal)),
	}
}

// Name returns the name of this component.
func (c *ComponentType[T]) Name() string {
	return c.impl.Name()
}

// RemoveFrom removes this component from the given entity.
func (c *ComponentType[T]) RemoveFrom(ctx SystemContext, id EntityID) error {
	return c.impl.RemoveFrom(ctx, id)
}

// AddTo adds this component to the given entity.
func (c *ComponentType[T]) AddTo(ctx SystemContext, id EntityID) error {
	return c.impl.AddTo(ctx, id)
}

// Get returns the component data that is associated with the given id. An error is returned if this entity
// is not actually associated with this component type.
func (c *ComponentType[T]) Get(ctx SystemContext, id EntityID) (comp T, err error) {
	return c.impl.Get(ctx, id)
}

// Set sets the component data for a specific EntityID.
func (c *ComponentType[T]) Set(ctx SystemContext, id EntityID, comp T) error {
	return c.impl.Set(ctx, id, comp)
}

// Update updates the component data that is associated with the given EntityID. It is a convenience wrapper
// for a Get followed by a Set.
func (c *ComponentType[T]) Update(ctx SystemContext, id EntityID, fn func(T) T) error {
	return c.impl.Update(ctx, id, fn)
}

// Convert implements the AnyComponentType interface which allows a ComponentType to be registered
// with a World via RegisterComponents.
func (c *ComponentType[T]) Convert() component.IComponentType {
	return c.impl
}
