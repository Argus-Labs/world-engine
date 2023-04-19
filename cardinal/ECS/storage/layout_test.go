package storage

import (
	"testing"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

func TestLayout(t *testing.T) {
	compType := NewMockComponentType(struct{}{}, nil)
	components := []component.IComponentType{compType}
	layout := NewLayout(components)

	if layout.HasComponent(compType) == false {
		t.Errorf("ArchLayout should have the component type")
	}
}
