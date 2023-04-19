package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"testing"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
)

func TestIndex(t *testing.T) {
	var (
		ca = storage.NewMockComponentType(struct{}{}, nil)
		cb = storage.NewMockComponentType(struct{}{}, nil)
		cc = storage.NewMockComponentType(struct{}{}, nil)
	)

	index := storage.NewArchetypeComponentIndex()

	layoutA := storage.NewLayout([]component.IComponentType{ca})
	layoutB := storage.NewLayout([]component.IComponentType{ca, cb})

	index.Push(layoutA)
	index.Push(layoutB)

	tests := []struct {
		filter   filter.LayoutFilter
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
