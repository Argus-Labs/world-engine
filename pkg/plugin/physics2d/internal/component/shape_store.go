package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

// KeptShape is one store entry: a name and the shape entity it keeps alive.
type KeptShape struct {
	Name  string            `json:"name"`
	Shape cardinal.EntityID `json:"shape"`
}

// ShapeStore is the plugin singleton's list of kept shapes. A kept shape is never swept,
// whether or not a body names it, until it is released. Names are unique within the store.
type ShapeStore struct {
	Kept immutable.Slice[KeptShape] `json:"kept"`
}

// Name returns the ECS component name.
func (ShapeStore) Name() string { return "shape_store_2d" }

// Index returns the position of name in Kept, or -1.
func (s ShapeStore) Index(name string) int {
	if name == "" {
		return -1
	}
	return s.Kept.IndexFunc(func(k KeptShape) bool { return k.Name == name })
}
