package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
)

type Filter interface {
	convertToFilterable() ecs.Filterable
}

type all struct {
}

type and struct {
	filters []Filter
}

type or struct {
	filters []Filter
}

type not struct {
	filter Filter
}

type contains struct {
	components []metadata.Component
}

type exact struct {
	components []metadata.Component
}

func All() Filter {
	return &all{}
}

func And(filters ...Filter) Filter {
	return &and{filters: filters}
}

func Or(filters ...Filter) Filter {
	return &or{filters: filters}
}

func Not(filter Filter) Filter {
	return &not{filter: filter}
}

func Contains(components ...metadata.Component) Filter {
	return &contains{components: components}
}

func Exact(components ...metadata.Component) Filter {
	return &exact{components: components}
}

func (s or) convertToFilterable() ecs.Filterable {
	acc := make([]ecs.Filterable, 0, len(s.filters))
	for _, internalFilter := range s.filters {
		f := internalFilter.convertToFilterable()
		acc = append(acc, f)
	}
	return ecs.Or(acc...)
}

func (s and) convertToFilterable() ecs.Filterable {
	acc := make([]ecs.Filterable, 0, len(s.filters))
	for _, internalFilter := range s.filters {
		f := internalFilter.convertToFilterable()
		acc = append(acc, f)
	}
	return ecs.And(acc...)
}

func (s not) convertToFilterable() ecs.Filterable {
	f := s.filter.convertToFilterable()
	return ecs.Not(f)
}

func (s contains) convertToFilterable() ecs.Filterable {
	return ecs.Contains(s.components...)
}

func (s exact) convertToFilterable() ecs.Filterable {
	return ecs.Exact(s.components...)
}

func (a all) convertToFilterable() ecs.Filterable {
	return ecs.All()
}
