package storage

import (
	"bytes"
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

// Layout represents a ArchLayout of components.
type Layout struct {
	ComponentTypes []component.IComponentType
}

// NewLayout creates a new Ent ArchLayout.
func NewLayout(components []component.IComponentType) *Layout {
	layout := &Layout{
		ComponentTypes: []component.IComponentType{},
	}

	for _, ct := range components {
		layout.ComponentTypes = append(layout.ComponentTypes, ct)
	}

	return layout
}

// Components returns the components of the ArchLayout.
func (l *Layout) Components() []component.IComponentType {
	return l.ComponentTypes
}

// HasComponent returns true if the ArchLayout has the given component type.
func (l *Layout) HasComponent(componentType component.IComponentType) bool {
	for _, ct := range l.ComponentTypes {
		if ct == componentType {
			return true
		}
	}
	return false
}

func (l *Layout) String() string {
	var out bytes.Buffer
	out.WriteString("Layout: {")
	for i, ct := range l.ComponentTypes {
		if i != 0 {
			out.WriteString(", ")
		}
		out.WriteString(fmt.Sprintf("%s", ct))
	}
	out.WriteString("}")
	return out.String()
}
