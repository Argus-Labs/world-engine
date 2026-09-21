package component

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

// ShapeStore is the plugin singleton's list of kept shapes. A kept shape is never swept,
// whether or not a body names it, until it is released.
type ShapeStore struct {
	Kept immutable.Slice[cardinal.EntityID] `json:"kept"`
}

// Name returns the ECS component name.
func (ShapeStore) Name() string { return "shape_store_2d" }

// Index returns the position of shape in Kept, or -1.
func (s ShapeStore) Index(shape cardinal.EntityID) int {
	return s.Kept.IndexFunc(func(k cardinal.EntityID) bool { return k == shape })
}
