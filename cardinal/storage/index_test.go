package storage

import (
	"testing"

	"github.com/argus-labs/cardinal/component"
	"github.com/argus-labs/cardinal/filter"
)

func TestIndex(t *testing.T) {
	var (
		ca = NewMockComponentType(struct{}{}, nil)
		cb = NewMockComponentType(struct{}{}, nil)
		cc = NewMockComponentType(struct{}{}, nil)
	)

	index := NewArchetypeComponentIndex()

	layoutA := NewLayout([]component.IComponentType{ca})
	layoutB := NewLayout([]component.IComponentType{ca, cb})

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
		if len(it.values) != tt.expected {
			t.Errorf("Index should have %d archetypes", tt.expected)
		}
		if it.current != 0 && it.HasNext() {
			t.Errorf("Index should have 0 as current")
		}
		if tt.expected == 0 && it.HasNext() {
			t.Errorf("Index should not have next")
		}
	}
}
