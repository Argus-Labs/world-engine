package interfaces

type IEntity interface {
	EntityID() EntityID
	GetArchID() ArchetypeID
}

type EntityID uint64

type ILocation interface {
	GetArchID() ArchetypeID
	GetCompIndex() ComponentIndex
	SetCompIndex(index ComponentIndex)
	SetArchID(id ArchetypeID)
}
