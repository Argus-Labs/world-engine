package storage_test

import (
	storage2 "pkg.world.dev/world-engine/cardinal/engine/storage"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

func TestIndex(t *testing.T) {
	var (
		ca = storage2.NewMockComponentType(struct{}{}, nil)
		cb = storage2.NewMockComponentType(struct{}{}, nil)
		cc = storage2.NewMockComponentType(struct{}{}, nil)
	)

	index := storage2.NewArchetypeComponentIndex()

	compsA := []component.IComponentType{ca}
	compsB := []component.IComponentType{ca, cb}

	index.Push(compsA)
	index.Push(compsB)

	tests := []struct {
		filter   filter.ComponentFilter
		expected int
	}{
		{

			filter:   filter.Contains(ca),
			expected: 2,
		},
		{

			filter:   filter.Contains(cb),
			expected: 1,
		},
		{

			filter:   filter.Contains(cc),
			expected: 0,
		},
	}

	for _, tt := range tests {
		it := index.Search(tt.filter)
		if len(it.Values) != tt.expected {
			t.Errorf("Index should have %d archetypes", tt.expected)
		}
		if it.Current != 0 && it.HasNext() {
			t.Errorf("Index should have 0 as current")
		}
		if tt.expected == 0 && it.HasNext() {
			t.Errorf("Index should not have next")
		}
	}
}
