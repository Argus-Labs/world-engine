package internal

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// Geometry is the set of geometry components a shape entity may carry, exactly one per
// entity. The method set repeats cardinal's component contract so Ref[G] can be instantiated
// from outside the cardinal package.
type Geometry interface {
	component.CircleGeom | component.BoxGeom | component.PolygonGeom |
		component.ChainGeom | component.EdgeGeom | component.CapsuleGeom
	Name() string
	MarshalWire() []byte
	UnmarshalWire([]byte) (any, error)
}

// ShapeRow is the search row for shape entities of one geometry kind: the shared
// ShapeCommon plus that kind's geometry component.
type ShapeRow[G Geometry] struct {
	Common cardinal.Ref[component.ShapeCommon]
	Geom   cardinal.Ref[G]
}

// ShapeKind says which geometry component a mirrored shape entity carries.
type ShapeKind uint8

// Shape kinds, one per geometry component.
const (
	ShapeKindCircle ShapeKind = iota + 1
	ShapeKindBox
	ShapeKindPolygon
	ShapeKindChain
	ShapeKindEdge
	ShapeKindCapsule
)

// ResolvedShape is one shape entity's components as mirrored by the runtime. Only the
// geometry field matching Kind is set; the rest stay zero. Chain points are the one list:
// the mirror keeps the value it first saw and treats it as fixed for the entity's lifetime.
type ResolvedShape struct {
	Kind    ShapeKind
	Common  component.ShapeCommon
	Circle  component.CircleGeom
	Box     component.BoxGeom
	Polygon component.PolygonGeom
	Chain   component.ChainGeom
	Edge    component.EdgeGeom
	Capsule component.CapsuleGeom
}

// Resolve packs a shape entity's components into a ResolvedShape, picking Kind from the
// geometry component's type.
func Resolve[G Geometry](common component.ShapeCommon, geom G) ResolvedShape {
	out := ResolvedShape{Common: common}
	switch g := any(geom).(type) {
	case component.CircleGeom:
		out.Kind, out.Circle = ShapeKindCircle, g
	case component.BoxGeom:
		out.Kind, out.Box = ShapeKindBox, g
	case component.PolygonGeom:
		out.Kind, out.Polygon = ShapeKindPolygon, g
	case component.ChainGeom:
		out.Kind, out.Chain = ShapeKindChain, g
	case component.EdgeGeom:
		out.Kind, out.Edge = ShapeKindEdge, g
	case component.CapsuleGeom:
		out.Kind, out.Capsule = ShapeKindCapsule, g
	}
	return out
}

// validate runs the component validators for the kind the shape carries.
func (s ResolvedShape) validate() error {
	if err := s.Common.Validate(); err != nil {
		return err
	}
	switch s.Kind {
	case ShapeKindCircle:
		return s.Circle.Validate()
	case ShapeKindBox:
		return s.Box.Validate()
	case ShapeKindPolygon:
		return s.Polygon.Validate()
	case ShapeKindChain:
		return s.Chain.Validate()
	case ShapeKindEdge:
		return s.Edge.Validate()
	case ShapeKindCapsule:
		return s.Capsule.Validate()
	default:
		return errors.New("shape entity carries no geometry component")
	}
}

// structuralEqual reports whether two shapes produce the same Box2D fixture definition:
// same kind, same geometry, same sensor flag (Box2D v3 cannot toggle isSensor in place).
// Material and filter differences are not structural; fixture setters apply those.
func (s ResolvedShape) structuralEqual(o ResolvedShape) bool {
	return s.Kind == o.Kind &&
		s.Common.IsSensor == o.Common.IsSensor &&
		s.Circle == o.Circle &&
		s.Box == o.Box &&
		s.Polygon == o.Polygon &&
		s.Chain.Loop == o.Chain.Loop &&
		immutable.Equal(s.Chain.Points, o.Chain.Points) &&
		s.Edge == o.Edge &&
		s.Capsule == o.Capsule
}

// sameExceptPoints reports whether two mirrored values of one entity are equal, chain points
// aside (those are fixed once mirrored, so they never count as a change).
func (s ResolvedShape) sameExceptPoints(o ResolvedShape) bool {
	return s.Kind == o.Kind &&
		s.Common == o.Common &&
		s.Circle == o.Circle &&
		s.Box == o.Box &&
		s.Polygon == o.Polygon &&
		s.Chain.Loop == o.Chain.Loop &&
		s.Edge == o.Edge &&
		s.Capsule == o.Capsule
}

// ShapeEntry is one shape entity as gathered from ECS for SyncShapes.
type ShapeEntry struct {
	EntityID cardinal.EntityID
	Shape    ResolvedShape
}

// shapeChange classifies how a mirrored shape entity changed this tick.
type shapeChange uint8

const (
	// shapeChangeMaterial: friction, restitution, density or filter changed; fixture setters.
	shapeChangeMaterial shapeChange = iota + 1
	// shapeChangeStructural: kind, geometry or sensor flag changed; fixtures are rebuilt.
	shapeChangeStructural
)

// ShapeEntriesScratch returns the runtime-owned buffer for the per-tick shape gather,
// emptied and ready to append into. Pair with KeepShapeEntriesScratch.
func (rt *Runtime) ShapeEntriesScratch() []ShapeEntry {
	return rt.shapeEntriesScratch[:0]
}

// KeepShapeEntriesScratch stores the gathered slice back on the runtime and clears everything
// past its length. Returns it for chaining.
func (rt *Runtime) KeepShapeEntriesScratch(entries []ShapeEntry) []ShapeEntry {
	rt.shapeEntriesScratch = clearScratchTail(entries)
	return entries
}

// SyncShapes reconciles the ShapeMirror with this tick's shape entities and records which
// mirrored shapes changed (dirtyShapes) so the bodies using them get re-diffed once. Ids no
// longer present are dropped. A shape entity carrying two geometry components is mirrored
// with the first one gathered and reported as an error.
func (rt *Runtime) SyncShapes(entries []ShapeEntry) error {
	clear(rt.dirtyShapes)
	if rt.shapeSeenScratch == nil {
		rt.shapeSeenScratch = make(map[cardinal.EntityID]struct{}, len(entries))
	}
	seen := rt.shapeSeenScratch
	clear(seen)

	var dup error
	for i := range entries {
		e := &entries[i]
		if _, twice := seen[e.EntityID]; twice {
			if dup == nil {
				dup = fmt.Errorf("physics2d: shape entity %d carries more than one geometry component", e.EntityID)
			}
			continue
		}
		seen[e.EntityID] = struct{}{}
		rt.mirrorShape(e.EntityID, e.Shape)
	}
	if len(seen) != len(rt.ShapeMirror) {
		for id := range rt.ShapeMirror {
			if _, ok := seen[id]; !ok {
				delete(rt.ShapeMirror, id)
			}
		}
	}
	return dup
}

// mirrorShape stores one shape entity's value, marking it dirty (material or structural)
// when it differs from the mirrored value. Chain points are immutable, so sharing them with
// the component is safe; a known entity keeps the points it was first seen with.
func (rt *Runtime) mirrorShape(id cardinal.EntityID, shape ResolvedShape) {
	prev, known := rt.ShapeMirror[id]
	if !known {
		rt.ShapeMirror[id] = shape
		return
	}
	shape.Chain.Points = prev.Chain.Points
	if prev.sameExceptPoints(shape) {
		return
	}
	change := shapeChangeMaterial
	if !prev.structuralEqual(shape) {
		change = shapeChangeStructural
	}
	rt.dirtyShapes[id] = change
	rt.ShapeMirror[id] = shape
}

// resolveSlot looks a slot's shape entity up in the mirror.
func (rt *Runtime) resolveSlot(slot component.ShapeSlot) (ResolvedShape, error) {
	sh, ok := rt.ShapeMirror[slot.Shape]
	if !ok {
		return ResolvedShape{}, fmt.Errorf(
			"shape entity %d not found (the entity must exist and carry ShapeCommon plus one geometry component)",
			slot.Shape)
	}
	return sh, nil
}

// slotsDirty reports whether any slot references a shape entity that changed this tick.
func (rt *Runtime) slotsDirty(slots immutable.Slice[component.ShapeSlot]) bool {
	if len(rt.dirtyShapes) == 0 {
		return false
	}
	for slot := range slots.Values() {
		if _, ok := rt.dirtyShapes[slot.Shape]; ok {
			return true
		}
	}
	return false
}

// slotsStructuralEqual reports whether the live slots can be applied to the fixtures built
// from prev without recreating them: same count, same local transforms, and per slot either
// the same shape entity (not structurally dirty) or a different shape entity with the same
// geometry and sensor flag. A previous shape entity that has left the mirror forces a rebuild.
func (rt *Runtime) slotsStructuralEqual(prev, live immutable.Slice[component.ShapeSlot]) bool {
	if prev.Len() != live.Len() {
		return false
	}
	for i, l := range live.All() {
		p := prev.At(i)
		if !vec2Equal(p.LocalOffset, l.LocalOffset) || p.LocalRotation != l.LocalRotation {
			return false
		}
		if p.Shape == l.Shape {
			if rt.dirtyShapes[l.Shape] == shapeChangeStructural {
				return false
			}
			continue
		}
		a, okA := rt.ShapeMirror[p.Shape]
		b, okB := rt.ShapeMirror[l.Shape]
		if !okA || !okB || !a.structuralEqual(b) {
			return false
		}
	}
	return true
}
