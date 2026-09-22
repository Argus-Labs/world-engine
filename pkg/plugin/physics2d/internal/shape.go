package internal

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// Geometry is the set of geometry components a shape entity may carry, exactly one per
// entity. The method set repeats cardinal's component contract so Entity.Get[G] can be
// instantiated from outside the cardinal package.
type Geometry interface {
	component.CircleGeom | component.BoxGeom | component.PolygonGeom |
		component.ChainGeom | component.EdgeGeom | component.CapsuleGeom
	Name() string
	Validate() error
	SizeWire() int
	AppendWire([]byte) []byte
	MarshalWire() []byte
	UnmarshalWire([]byte) (any, error)
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

func (k ShapeKind) String() string {
	switch k {
	case ShapeKindCircle:
		return "circle"
	case ShapeKindBox:
		return "box"
	case ShapeKindPolygon:
		return "polygon"
	case ShapeKindChain:
		return "chain"
	case ShapeKindEdge:
		return "edge"
	case ShapeKindCapsule:
		return "capsule"
	}
	return "no geometry"
}

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

// Validate runs the component validators: material, the geometry the shape carries, then
// the rules that need both.
func (s ResolvedShape) Validate() error {
	if err := s.Common.Validate(); err != nil {
		return err
	}
	if err := s.ValidateGeometry(); err != nil {
		return err
	}
	return s.ValidateSensorSupport()
}

// ValidateSensorSupport rejects a sensor on a kind Box2D cannot build as one. A chain is many
// segment shapes made from a single ChainDef, and that def carries no sensor flag, so a
// sensor chain would come out solid and quietly block whatever it was meant to watch.
func (s ResolvedShape) ValidateSensorSupport() error {
	if s.Common.IsSensor && s.Kind == ShapeKindChain {
		return errors.New("cannot be a sensor (Box2D builds chain segments solid)")
	}
	return nil
}

// ValidateGeometry runs the validator of the geometry kind the shape carries.
func (s ResolvedShape) ValidateGeometry() error {
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

// Equal reports whether two shapes have the same kind, geometry, material and sensor flag.
// Spawn de-duplicates on it, so a sensor and a solid that agree on everything else stay two
// shapes, which they must: Box2D cannot toggle isSensor on a live fixture.
func (s ResolvedShape) Equal(o ResolvedShape) bool {
	return s.sameExceptPoints(o) && pointsEqual(s.Chain.Points, o.Chain.Points)
}

// pointsEqual compares chain points, identity first. A column hands back the same header every
// read, so re-reading one entity settles in a pointer compare instead of walking the points
// every tick; spawn's de-duplication, which compares two entities, falls through to the walk.
func pointsEqual(a, b immutable.Slice[component.Vec2]) bool {
	return immutable.SameBacking(a, b) || immutable.Equal(a, b)
}

// sameExceptPoints reports whether two shapes are equal apart from chain points, which are
// the one field a plain == cannot cover. Callers pair it with immutable.Equal on the points.
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
// mirrored shapes changed (dirtyShapes) so the bodies using them get re-diffed once. Ids first
// seen this tick are queued for the sweep, which keeps them only if a body names them by the
// end of the reconcile pass. Ids no longer present are dropped and marked structural: a body
// still naming one must lose its fixtures now and fail loudly, the same as it would after a
// restore, instead of keeping a fixture built from a shape that no longer exists.
//
// A shape entity carrying two geometry components is an invariant violation, not a runtime
// condition: Spawn creates exactly one, and once the components move behind the plugin's
// API nothing else can. It is asserted (dev builds panic, release builds keep the first
// kind gathered, which is deterministic).
func (rt *Runtime) SyncShapes(entries []ShapeEntry) {
	clear(rt.dirtyShapes)
	if rt.shapeSeenScratch == nil {
		rt.shapeSeenScratch = make(map[cardinal.EntityID]struct{}, len(entries))
	}
	seen := rt.shapeSeenScratch
	clear(seen)

	for i := range entries {
		e := &entries[i]
		if _, twice := seen[e.EntityID]; twice {
			// Only here: boxing the id for the message on every shape, every tick, allocates.
			assert.That(false, "physics2d: shape entity %d carries more than one geometry component", e.EntityID)
			continue
		}
		seen[e.EntityID] = struct{}{}
		rt.mirrorShape(e.EntityID, e.Shape)
	}
	if len(seen) != len(rt.ShapeMirror) {
		for id := range rt.ShapeMirror {
			if _, ok := seen[id]; !ok {
				delete(rt.ShapeMirror, id)
				rt.dirtyShapes[id] = shapeChangeStructural
			}
		}
	}
}

// mirrorShape stores one shape entity's value, marking it dirty (material or structural)
// when it differs from the mirrored value. Chain points are never edited in place, so sharing
// them with the component is safe; they are still compared, because an id can come back
// carrying different points (a restore over the ids init built, a destroy and respawn in one
// tick) and the fixture would otherwise keep the geometry the id had before.
func (rt *Runtime) mirrorShape(id cardinal.EntityID, shape ResolvedShape) {
	prev, known := rt.ShapeMirror[id]
	if !known {
		rt.ShapeMirror[id] = shape
		return
	}
	// Spelled out rather than prev.Equal(shape): ResolvedShape is 304 bytes and Equal is over
	// the inlining budget, so the call would copy the receiver and the argument every shape,
	// every tick. Both of these inline.
	if prev.sameExceptPoints(shape) && pointsEqual(prev.Chain.Points, shape.Chain.Points) {
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
func (rt *Runtime) resolveSlot(slot component.ShapeRef) (ResolvedShape, error) {
	sh, ok := rt.ShapeMirror[slot.Shape]
	if !ok {
		return ResolvedShape{}, fmt.Errorf(
			"shape entity %d not found (the entity must exist and carry ShapeCommon plus one geometry component)",
			slot.Shape)
	}
	return sh, nil
}

// slotsDirty reports whether any slot references a shape entity that changed this tick.
func (rt *Runtime) slotsDirty(slots immutable.Slice[component.ShapeRef]) bool {
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
// geometry and sensor flag. A previous shape entity that has left the mirror, or whose own
// geometry changed this tick, forces a rebuild.
func (rt *Runtime) slotsStructuralEqual(prev, live immutable.Slice[component.ShapeRef]) bool {
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
		// The comparison below stands in for the live fixture, which was built from p.Shape
		// as it was on an earlier tick. A p.Shape whose geometry changed this tick no longer
		// describes that fixture, so there is nothing safe to compare: rebuild.
		if rt.dirtyShapes[p.Shape] == shapeChangeStructural {
			return false
		}
		a, okA := rt.ShapeMirror[p.Shape]
		b, okB := rt.ShapeMirror[l.Shape]
		if !okA || !okB || !a.structuralEqual(b) {
			return false
		}
	}
	return true
}
