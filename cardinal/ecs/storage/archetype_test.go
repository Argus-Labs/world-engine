package storage

import (
	"testing"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

type (
	componentA = struct{}
	componentB = struct{}
)

func TestMatchesLayout(t *testing.T) {
	var (
		ca = NewMockComponentType(componentA{}, nil)
		cb = NewMockComponentType(componentB{}, nil)
	)

	components := []component.IComponentType{ca, cb}
	archetype := NewArchetype(0, NewLayout(components))
	if !archetype.LayoutMatches(components) {
		t.Errorf("archetype should match the ArchLayout")
	}
}

func TestPushEntity(t *testing.T) {
	var (
		ca = NewMockComponentType(struct{}{}, nil)
		cb = NewMockComponentType(struct{}{}, nil)
	)

	components := []component.IComponentType{ca, cb}
	archetype := NewArchetype(0, NewLayout(components))

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
