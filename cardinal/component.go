package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type AnyComponentType interface {
	Convert() component.IComponentType
}

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
func NewComponentType[T any]() *ComponentType[T] {
	return &ComponentType[T]{
		impl: ecs.NewComponentType[T](),
	}
}

func NewComponentTypeWithDefault[T any](defaultVal T) *ComponentType[T] {
	return &ComponentType[T]{
		impl: ecs.NewComponentType[T](ecs.WithDefault(defaultVal)),
	}
}

func (c *ComponentType[T]) Get(w *World, id EntityID) (comp T, err error) {
	return c.impl.Get(w.impl, id)
}

func (c *ComponentType[T]) Set(w *World, id EntityID, comp T) error {
	return c.impl.Set(w.impl, id, comp)
}

func (c *ComponentType[T]) Update(w *World, id EntityID, fn func(T) T) error {
	return c.impl.Update(w.impl, id, fn)
}

func (c *ComponentType[T]) Convert() component.IComponentType {
	return c.impl
}
