package tests

import (
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

func TestLayout(t *testing.T) {
	compType := storage.NewMockComponentType(struct{}{}, nil)
	components := []component.IComponentType{compType}
	layout := storage.NewLayout(components)

	if layout.HasComponent(compType) == false {
		t.Errorf("ArchLayout should have the component type")
	}
}
