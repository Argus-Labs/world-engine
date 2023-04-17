package entity

import "fmt"

// Entity is identifier of an entity.
// The first 32 bits are the entity id.
// The last 32 bits are the version.
// The version is incremented when the entity is destroyed.
type Entity uint64

// ID is a unique identifier for an entity.
type ID uint32

const idMask Entity = 0xFFFFFFFF00000000
const versionMask Entity = 0xFFFFFFF

// NewEntity creates a new entity.
// The id is a unique identifier for the entity.
// To reuse the id, the id should be passed from the world that created the entity.
func NewEntity(id ID) Entity {
	return Entity(uint64(id)<<32) & idMask
}

// Null represents a invalid entity.
var Null = Entity(0)

// ID returns the entity id.
func (e Entity) ID() ID {
	return ID(e)
}

// Version returns the entity version.
func (e Entity) Version() uint32 {
	return uint32(e & Entity(versionMask))
}

// IncVersion increments the entity version.
func (e Entity) IncVersion() Entity {
	return e&idMask | ((e + 1) & versionMask)
}

func (e Entity) String() string {
	return fmt.Sprintf("Entity: {id: %d, version: %d}", e.ID(), e.Version())
}
