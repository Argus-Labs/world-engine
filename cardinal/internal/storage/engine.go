package storage

import "github.com/argus-labs/cardinal/component"

type Engine interface {
	PushComponent(component component.IComponentType, index ArchetypeIndex) error
	Component(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte
	SetComponent(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex, compBz []byte)
	MoveComponent(source ArchetypeIndex, index ComponentIndex, dst ArchetypeIndex)
	SwapRemove(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) []byte
	Contains(archetypeIndex ArchetypeIndex, componentIndex ComponentIndex) bool
}
