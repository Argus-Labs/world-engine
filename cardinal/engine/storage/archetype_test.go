package storage_test

import (
	storage2 "pkg.world.dev/world-engine/cardinal/engine/storage"
	"testing"

	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type (
	componentA = struct{}
	componentB = struct{}
)

func TestMatchesComponents(t *testing.T) {
	var (
		ca = storage2.NewMockComponentType(componentA{}, nil)
		cb = storage2.NewMockComponentType(componentB{}, nil)
	)

	components := []component.IComponentType{ca, cb}
	archetype := storage2.NewArchetype(0, components)
	if !archetype.ComponentsMatch(components) {
		t.Errorf("archetype should match the components")
	}
}

func TestPushEntity(t *testing.T) {
	var (
		ca = storage2.NewMockComponentType(struct{}{}, nil)
		cb = storage2.NewMockComponentType(struct{}{}, nil)
	)

	components := []component.IComponentType{ca, cb}
	archetype := storage2.NewArchetype(0, components)

	archetype.PushEntity(0)
	archetype.PushEntity(1)
	archetype.PushEntity(2)

	if len(archetype.Entities()) != 3 {
		t.Errorf("archetype should have 3 Entitys")
	}

	archetype.SwapRemove(1)
	if len(archetype.Entities()) != 2 {
		t.Errorf("archetype should have 2 Entitys")
	}

	expected := []int{0, 2}
	for i, entity := range archetype.Entities() {
		if int(entity) != expected[i] {
			t.Errorf("archetype should have Ent %d", expected[i])
		}
	}
}
