package storage

import "github.com/argus-labs/cardinal/component"

type ComponentTypeMap interface {
	Register(component.IComponentType)
	ComponentType(component.TypeID) component.IComponentType
}

var _ ComponentTypeMap = &componentTypeMap{}

type componentTypeMap struct {
	typeMap map[component.TypeID]component.IComponentType
}

func NewComponentTypeMap() ComponentTypeMap {
	return &componentTypeMap{typeMap: make(map[component.TypeID]component.IComponentType)}
}

func (c *componentTypeMap) Register(ct component.IComponentType) {
	c.typeMap[ct.ID()] = ct
}

func (c *componentTypeMap) ComponentType(cid component.TypeID) component.IComponentType {
	return c.typeMap[cid]
}
