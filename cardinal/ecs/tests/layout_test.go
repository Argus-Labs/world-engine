package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"testing"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

func TestLayout(t *testing.T) {
	compType := storage.NewMockComponentType(struct{}{}, nil)
	components := []component.IComponentType{compType}
	layout := storage.NewLayout(components)

	if layout.HasComponent(compType) == false {
		t.Errorf("ArchLayout should have the component type")
	}
}
