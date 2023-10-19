package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component_metadata"
)

type CardinalFilter interface {
	ConvertToFilterable() ecs.Filterable
}

type and struct {
	filters []CardinalFilter
}

type or struct {
	filters []CardinalFilter
}

type not struct {
	filter CardinalFilter
}

type contains struct {
	components []component_metadata.Component
}

type exact struct {
	components []component_metadata.Component
}

func And(filters ...CardinalFilter) CardinalFilter {
	return &and{filters: filters}
}

func Or(filters ...CardinalFilter) CardinalFilter {
	return &or{filters: filters}
}

func Not(filter CardinalFilter) CardinalFilter {
	return &not{filter: filter}
}

func Contains(components ...component_metadata.Component) CardinalFilter {
	return &contains{components: components}
}

func Exact(components ...component_metadata.Component) CardinalFilter {
	return &exact{components: components}
}

func (s or) ConvertToFilterable() ecs.Filterable {
	acc := make([]ecs.Filterable, 0, len(s.filters))
	for _, internalFilter := range s.filters {
		f := internalFilter.ConvertToFilterable()
		acc = append(acc, f)
	}
	return ecs.Or(acc...)
}

func (s and) ConvertToFilterable() ecs.Filterable {
	acc := make([]ecs.Filterable, 0, len(s.filters))
	for _, internalFilter := range s.filters {
		f := internalFilter.ConvertToFilterable()
		acc = append(acc, f)
	}
	return ecs.And(acc...)
}

func (s not) ConvertToFilterable() ecs.Filterable {
	f := s.filter.ConvertToFilterable()
	return ecs.Not(f)

}

func (s contains) ConvertToFilterable() ecs.Filterable {
	return ecs.Contains(s.components...)
}

func (s exact) ConvertToFilterable() ecs.Filterable {
	return ecs.Exact(s.components...)
}
