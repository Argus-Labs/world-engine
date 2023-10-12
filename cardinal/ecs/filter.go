package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

type Filterable interface {
	ConvertToComponentFilter(world *World) (filter.ComponentFilter, error)
}

type and struct {
	filters []Filterable
}

type or struct {
	filters []Filterable
}

type not struct {
	filter Filterable
}

type contains struct {
	components []component.IAbstractComponent
}

type exact struct {
	components []component.IAbstractComponent
}

func And(filters ...Filterable) Filterable {
	return &and{filters: filters}
}

func Or(filters ...Filterable) Filterable {
	return &or{filters: filters}
}

func Not(filter Filterable) Filterable {
	return &not{filter: filter}
}

func Contains(components ...component.IAbstractComponent) Filterable {
	return &contains{components: components}
}

func Exact(components ...component.IAbstractComponent) Filterable {
	return &contains{components: components}
}

func (s or) ConvertToComponentFilter(world *World) (filter.ComponentFilter, error) {
	acc := make([]filter.ComponentFilter, 0, len(s.filters))
	for _, internalFilter := range s.filters {
		f, err := internalFilter.ConvertToComponentFilter(world)
		if err != nil {
			return nil, err
		}
		acc = append(acc, f)
	}
	return filter.Or(acc...), nil
}

func (s and) ConvertToComponentFilter(world *World) (filter.ComponentFilter, error) {
	acc := make([]filter.ComponentFilter, 0, len(s.filters))
	for _, internalFilter := range s.filters {
		f, err := internalFilter.ConvertToComponentFilter(world)
		if err != nil {
			return nil, err
		}
		acc = append(acc, f)
	}
	return filter.And(acc...), nil
}

func (s not) ConvertToComponentFilter(world *World) (filter.ComponentFilter, error) {
	f, err := s.filter.ConvertToComponentFilter(world)
	if err != nil {
		return nil, err
	}
	return filter.Not(f), nil

}

func (s contains) ConvertToComponentFilter(world *World) (filter.ComponentFilter, error) {
	acc := make([]IComponentType, 0, len(s.components))
	for _, internalComponent := range s.components {
		c, err := world.GetComponentByName(internalComponent.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	return filter.Contains(acc...), nil
}

func (s exact) ConvertToComponentFilter(world *World) (filter.ComponentFilter, error) {
	acc := make([]IComponentType, 0, len(s.components))
	for _, internalComponent := range s.components {
		c, err := world.GetComponentByName(internalComponent.Name())
		if err != nil {
			return nil, err
		}
		acc = append(acc, c)
	}
	return filter.Exact(acc...), nil
}
