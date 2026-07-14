// Package cbridge wraps Box2D v3 (C) via CGO for the physics2d plugin.
// All Box2D state lives on the C side; Go interacts through entity IDs.
package cbridge

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/box2d/include -I${SRCDIR}/../../third_party/box2d/src -O2
#cgo LDFLAGS: -lm -lpthread
#include "bridge.h"
*/
import "C"
import (
	"unsafe"
)

// ---------------------------------------------------------------------------
// Go types mirroring C bridge structs (exported for use by internal/).
// ---------------------------------------------------------------------------

type Vec2 struct{ X, Y float64 }

type BodyState struct {
	EntityID      uint32
	BodyType      uint8
	PX, PY, Angle float64
	VX, VY, AV    float64
}

const (
	ContactBegin uint8 = C.BRIDGE_CONTACT_BEGIN
	ContactEnd   uint8 = C.BRIDGE_CONTACT_END
)

type ContactEvent struct {
	Kind               uint8
	EntityA, EntityB   uint32
	ShapeIndexA        int
	ShapeIndexB        int
	IsSensor           bool
	CatA, MaskA        uint64
	GroupA             int32
	CatB, MaskB        uint64
	GroupB             int32
	NormalX, NormalY   float64
	NormalValid        bool
	PointX, PointY     float64
	PointValid         bool
	ManifoldPointCount int
}

type RaycastResult struct {
	Hit        bool
	EntityID   uint32
	ShapeIndex int
	PX, PY     float64
	NX, NY     float64
	Fraction   float64
}

type OverlapHit struct {
	EntityID   uint32
	ShapeIndex int
}

type CircleSweepResult struct {
	Hit        bool
	EntityID   uint32
	ShapeIndex int
	PX, PY     float64
	NX, NY     float64
	Fraction   float64
}

// ---------------------------------------------------------------------------
// World management
// ---------------------------------------------------------------------------

func CreateWorld(gx, gy float64) {
	C.bridge_create_world(C.float(gx), C.float(gy))
}

func DestroyWorld() {
	C.bridge_destroy_world()
}

func SetGravity(gx, gy float64) {
	C.bridge_set_gravity(C.float(gx), C.float(gy))
}

func WorldExists() bool {
	return bool(C.bridge_world_exists())
}

// GetWorldID returns the Box2D world id packed as uint32 (0 = no world).
func GetWorldID() uint32 {
	return uint32(C.bridge_get_world_id())
}

// ---------------------------------------------------------------------------
// Body management
// ---------------------------------------------------------------------------

func CreateBody(
	entityID uint32, bodyType uint8,
	px, py, angle float64,
	vx, vy, av float64,
	linearDamping, angularDamping, gravityScale float64,
	enabled, awake, sleepEnabled, bullet, fixedRotation bool,
) bool {
	return bool(C.bridge_create_body(
		C.uint32_t(entityID), C.uint8_t(bodyType),
		C.float(px), C.float(py), C.float(angle),
		C.float(vx), C.float(vy), C.float(av),
		C.float(linearDamping), C.float(angularDamping), C.float(gravityScale),
		C.bool(enabled), C.bool(awake), C.bool(sleepEnabled),
		C.bool(bullet), C.bool(fixedRotation),
	))
}

func DestroyBody(entityID uint32) {
	C.bridge_destroy_body(C.uint32_t(entityID))
}

func DestroyAllBodies() {
	C.bridge_destroy_all_bodies()
}

// ---------------------------------------------------------------------------
// Shape attachment
// ---------------------------------------------------------------------------

func AddCircleShape(
	entityID uint32, shapeIndex int,
	offsetX, offsetY, radius float64,
	isSensor bool, friction, restitution, density float64,
	cat, mask uint64, group int32,
) bool {
	return bool(C.bridge_add_circle_shape(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		C.float(offsetX), C.float(offsetY), C.float(radius),
		C.bool(isSensor), C.float(friction), C.float(restitution), C.float(density),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	))
}

func AddBoxShape(
	entityID uint32, shapeIndex int,
	offsetX, offsetY float64,
	halfW, halfH, localRot float64,
	isSensor bool, friction, restitution, density float64,
	cat, mask uint64, group int32,
) bool {
	return bool(C.bridge_add_box_shape(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		C.float(offsetX), C.float(offsetY),
		C.float(halfW), C.float(halfH), C.float(localRot),
		C.bool(isSensor), C.float(friction), C.float(restitution), C.float(density),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	))
}

func AddPolygonShape(
	entityID uint32, shapeIndex int,
	verts []Vec2,
	offsetX, offsetY, localRot float64,
	isSensor bool, friction, restitution, density float64,
	cat, mask uint64, group int32,
) bool {
	if len(verts) == 0 {
		return false
	}
	cverts := make([]C.BridgeVec2, len(verts))
	for i, v := range verts {
		cverts[i] = C.BridgeVec2{x: C.float(v.X), y: C.float(v.Y)}
	}
	return bool(C.bridge_add_polygon_shape(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		&cverts[0], C.int32_t(len(verts)),
		C.float(offsetX), C.float(offsetY), C.float(localRot),
		C.bool(isSensor), C.float(friction), C.float(restitution), C.float(density),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	))
}

func AddChainShape(
	entityID uint32, shapeIndex int,
	points []Vec2, isLoop bool,
	friction, restitution float64,
	cat, mask uint64, group int32,
) bool {
	if len(points) == 0 {
		return false
	}
	cpts := make([]C.BridgeVec2, len(points))
	for i, p := range points {
		cpts[i] = C.BridgeVec2{x: C.float(p.X), y: C.float(p.Y)}
	}
	return bool(C.bridge_add_chain_shape(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		&cpts[0], C.int32_t(len(points)), C.bool(isLoop),
		C.float(friction), C.float(restitution),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	))
}

func AddSegmentShape(
	entityID uint32, shapeIndex int,
	v1x, v1y, v2x, v2y float64,
	isSensor bool, friction, restitution, density float64,
	cat, mask uint64, group int32,
) bool {
	return bool(C.bridge_add_segment_shape(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		C.float(v1x), C.float(v1y), C.float(v2x), C.float(v2y),
		C.bool(isSensor), C.float(friction), C.float(restitution), C.float(density),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	))
}

func AddCapsuleShape(
	entityID uint32, shapeIndex int,
	c1x, c1y, c2x, c2y, radius float64,
	isSensor bool, friction, restitution, density float64,
	cat, mask uint64, group int32,
) bool {
	return bool(C.bridge_add_capsule_shape(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		C.float(c1x), C.float(c1y), C.float(c2x), C.float(c2y), C.float(radius),
		C.bool(isSensor), C.float(friction), C.float(restitution), C.float(density),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	))
}

func DestroyAllShapes(entityID uint32) {
	C.bridge_destroy_all_shapes(C.uint32_t(entityID))
}

// ---------------------------------------------------------------------------
// Body state setters
// ---------------------------------------------------------------------------

func SetTransform(entityID uint32, px, py, angle float64) {
	C.bridge_set_transform(C.uint32_t(entityID), C.float(px), C.float(py), C.float(angle))
}

func SetLinearVelocity(entityID uint32, vx, vy float64) {
	C.bridge_set_linear_velocity(C.uint32_t(entityID), C.float(vx), C.float(vy))
}

func SetAngularVelocity(entityID uint32, av float64) {
	C.bridge_set_angular_velocity(C.uint32_t(entityID), C.float(av))
}

func SetBodyType(entityID uint32, bodyType uint8) {
	C.bridge_set_body_type(C.uint32_t(entityID), C.uint8_t(bodyType))
}

func SetLinearDamping(entityID uint32, damping float64) {
	C.bridge_set_linear_damping(C.uint32_t(entityID), C.float(damping))
}

func SetAngularDamping(entityID uint32, damping float64) {
	C.bridge_set_angular_damping(C.uint32_t(entityID), C.float(damping))
}

func SetGravityScale(entityID uint32, scale float64) {
	C.bridge_set_gravity_scale(C.uint32_t(entityID), C.float(scale))
}

func SetBodyEnabled(entityID uint32, enabled bool) {
	C.bridge_set_body_enabled(C.uint32_t(entityID), C.bool(enabled))
}

func SetAwake(entityID uint32, awake bool) {
	C.bridge_set_awake(C.uint32_t(entityID), C.bool(awake))
}

func SetSleepEnabled(entityID uint32, enabled bool) {
	C.bridge_set_sleep_enabled(C.uint32_t(entityID), C.bool(enabled))
}

func SetBullet(entityID uint32, flag bool) {
	C.bridge_set_bullet(C.uint32_t(entityID), C.bool(flag))
}

func SetFixedRotation(entityID uint32, flag bool) {
	C.bridge_set_fixed_rotation(C.uint32_t(entityID), C.bool(flag))
}

func ResetMassData(entityID uint32) {
	C.bridge_reset_mass_data(C.uint32_t(entityID))
}

// ---------------------------------------------------------------------------
// Per-shape mutable setters
// ---------------------------------------------------------------------------

func SetShapeFriction(entityID uint32, shapeIndex int, friction float64) {
	C.bridge_set_shape_friction(C.uint32_t(entityID), C.int32_t(shapeIndex), C.float(friction))
}

func SetShapeRestitution(entityID uint32, shapeIndex int, restitution float64) {
	C.bridge_set_shape_restitution(C.uint32_t(entityID), C.int32_t(shapeIndex), C.float(restitution))
}

func SetShapeDensity(entityID uint32, shapeIndex int, density float64) {
	C.bridge_set_shape_density(C.uint32_t(entityID), C.int32_t(shapeIndex), C.float(density))
}

func SetShapeFilter(entityID uint32, shapeIndex int, cat, mask uint64, group int32) {
	C.bridge_set_shape_filter(
		C.uint32_t(entityID), C.int32_t(shapeIndex),
		C.uint64_t(cat), C.uint64_t(mask), C.int32_t(group),
	)
}

// ---------------------------------------------------------------------------
// Stepping — two CGO calls: advance (runs physics + returns counts) then
// drain (copies body states + contact events into caller buffers sized from
// those counts). This guarantees the caller buffer is always large enough,
// so no contact event is ever dropped.
// ---------------------------------------------------------------------------

// Step advances the active C-side world and returns body states and contact
// events. Not safe to call concurrently — cbridge owns single-world global
// state (the C-side world itself).
func Step(dt float64, subStepCount int) ([]BodyState, []ContactEvent) {
	counts := C.bridge_step_advance(C.float(dt), C.int32_t(subStepCount))
	nStates := int(counts.body_count)
	nContacts := int(counts.contact_event_count)

	stateBuf := make([]C.BridgeBodyState, nStates)
	contactBuf := make([]C.BridgeContactEvent, nContacts)

	// Empty slices have no addressable element — pass nil in that case so
	// cgo doesn't panic on &stateBuf[0].
	var statesPtr *C.BridgeBodyState
	var contactsPtr *C.BridgeContactEvent
	if nStates > 0 {
		statesPtr = (*C.BridgeBodyState)(unsafe.Pointer(&stateBuf[0]))
	}
	if nContacts > 0 {
		contactsPtr = (*C.BridgeContactEvent)(unsafe.Pointer(&contactBuf[0]))
	}

	rc := C.bridge_step_drain(
		statesPtr, C.int32_t(nStates),
		contactsPtr, C.int32_t(nContacts),
	)
	if rc < 0 {
		panic("physics2d/cbridge: bridge_step_drain reported a sizing mismatch — advance/drain invariant violated")
	}

	states := make([]BodyState, nStates)
	for i := range nStates {
		s := &stateBuf[i]
		states[i] = BodyState{
			EntityID: uint32(s.entity_id),
			BodyType: uint8(s.body_type),
			PX:       float64(s.px), PY: float64(s.py), Angle: float64(s.angle),
			VX: float64(s.vx), VY: float64(s.vy), AV: float64(s.av),
		}
	}

	contacts := make([]ContactEvent, nContacts)
	for i := range nContacts {
		contacts[i] = convertContactEvent(&contactBuf[i])
	}

	return states, contacts
}

func convertContactEvent(c *C.BridgeContactEvent) ContactEvent {
	return ContactEvent{
		Kind:        uint8(c.kind),
		EntityA:     uint32(c.entity_a),
		EntityB:     uint32(c.entity_b),
		ShapeIndexA: int(c.shape_index_a),
		ShapeIndexB: int(c.shape_index_b),
		IsSensor:    bool(c.is_sensor),
		CatA:        uint64(c.cat_a), MaskA: uint64(c.mask_a), GroupA: int32(c.group_a),
		CatB: uint64(c.cat_b), MaskB: uint64(c.mask_b), GroupB: int32(c.group_b),
		NormalX: float64(c.normal_x), NormalY: float64(c.normal_y),
		NormalValid: bool(c.normal_valid),
		PointX:      float64(c.point_x), PointY: float64(c.point_y),
		PointValid:         bool(c.point_valid),
		ManifoldPointCount: int(c.manifold_point_count),
	}
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

func Raycast(ox, oy, ex, ey float64, cat, mask uint64, includeSensors bool) RaycastResult {
	r := C.bridge_raycast(
		C.float(ox), C.float(oy), C.float(ex), C.float(ey),
		C.uint64_t(cat), C.uint64_t(mask), C.bool(includeSensors),
	)
	return RaycastResult{
		Hit: bool(r.hit), EntityID: uint32(r.entity_id), ShapeIndex: int(r.shape_index),
		PX: float64(r.px), PY: float64(r.py),
		NX: float64(r.nx), NY: float64(r.ny),
		Fraction: float64(r.fraction),
	}
}

func OverlapAABB(minX, minY, maxX, maxY float64, cat, mask uint64, includeSensors bool) []OverlapHit {
	buf := make([]C.BridgeOverlapHit, 1024)
	n := C.bridge_overlap_aabb(
		C.float(minX), C.float(minY), C.float(maxX), C.float(maxY),
		C.uint64_t(cat), C.uint64_t(mask), C.bool(includeSensors),
		(*C.BridgeOverlapHit)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)),
	)
	hits := make([]OverlapHit, int(n))
	for i := range int(n) {
		hits[i] = OverlapHit{
			EntityID:   uint32(buf[i].entity_id),
			ShapeIndex: int(buf[i].shape_index),
		}
	}
	return hits
}

func CircleSweep(sx, sy, ex, ey, radius float64, cat, mask uint64, includeSensors bool, maxFraction float64,
) CircleSweepResult {
	r := C.bridge_circle_sweep(
		C.float(sx), C.float(sy), C.float(ex), C.float(ey), C.float(radius),
		C.uint64_t(cat), C.uint64_t(mask), C.bool(includeSensors),
		C.float(maxFraction),
	)
	return CircleSweepResult{
		Hit: bool(r.hit), EntityID: uint32(r.entity_id), ShapeIndex: int(r.shape_index),
		PX: float64(r.px), PY: float64(r.py),
		NX: float64(r.nx), NY: float64(r.ny),
		Fraction: float64(r.fraction),
	}
}

// ---------------------------------------------------------------------------
// Live contact gathering (for post-rebuild diff)
// ---------------------------------------------------------------------------

func GatherLiveContacts() []ContactEvent {
	buf := make([]C.BridgeContactEvent, 4096)
	n := C.bridge_gather_live_contacts(
		(*C.BridgeContactEvent)(unsafe.Pointer(&buf[0])), C.int32_t(len(buf)),
	)
	contacts := make([]ContactEvent, int(n))
	for i := range int(n) {
		contacts[i] = convertContactEvent(&buf[i])
	}
	return contacts
}
