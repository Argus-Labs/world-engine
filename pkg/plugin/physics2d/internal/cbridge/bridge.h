#ifndef PHYSICS2D_BRIDGE_H
#define PHYSICS2D_BRIDGE_H

#include <stdint.h>
#include <stdbool.h>

// ---------------------------------------------------------------------------
// Data types shared between C and Go via CGO.
// ---------------------------------------------------------------------------

typedef struct { float x, y; } BridgeVec2;

// Post-step body state for writeback.
typedef struct {
    uint32_t entity_id;
    uint8_t  body_type;   // ECS body type (for writeback filtering)
    float    px, py, angle;
    float    vx, vy, av;
} BridgeBodyState;

// Contact event emitted after step or gathered from live contacts.
#define BRIDGE_CONTACT_BEGIN 0
#define BRIDGE_CONTACT_END   1

typedef struct {
    uint8_t  kind;        // BRIDGE_CONTACT_BEGIN or BRIDGE_CONTACT_END
    uint32_t entity_a, entity_b;
    int32_t  shape_index_a, shape_index_b;
    bool     is_sensor;
    uint64_t cat_a, mask_a;
    int32_t  group_a;
    uint64_t cat_b, mask_b;
    int32_t  group_b;
    float    normal_x, normal_y;
    bool     normal_valid;
    float    point_x, point_y;
    bool     point_valid;
    int32_t  manifold_point_count;
} BridgeContactEvent;

// Counts returned by bridge_step_advance. The Go wrapper uses these values
// to size its caller buffers exactly so bridge_step_drain never overflows
// them — contact events are structurally impossible to drop in the split
// advance/drain design, because Box2D's internal event arrays are stable
// between b2World_Step calls and the drain phase only reads them.
typedef struct {
    int32_t body_count;
    int32_t contact_event_count;
} BridgeStepCounts;

// Raycast result.
typedef struct {
    bool     hit;
    uint32_t entity_id;
    int32_t  shape_index;
    float    px, py;
    float    nx, ny;
    float    fraction;
} BridgeRaycastResult;

// Single AABB overlap hit.
typedef struct {
    uint32_t entity_id;
    int32_t  shape_index;
} BridgeOverlapHit;

// Circle sweep result.
typedef struct {
    bool     hit;
    uint32_t entity_id;
    int32_t  shape_index;
    float    px, py;
    float    nx, ny;
    float    fraction;
} BridgeCircleSweepResult;

// ECS body type constants (must match component.BodyType* values).
#define BRIDGE_BODY_STATIC    1
#define BRIDGE_BODY_DYNAMIC   2
#define BRIDGE_BODY_KINEMATIC 3

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

// bridge_init_diagnostics installs the custom Box2D assert handler that
// routes B2_ASSERT failures through the Go-side structured logger, giving
// production crashes a readable message instead of an opaque SIGILL during
// cgo execution. Called automatically from cbridge's Go init(); safe to
// invoke multiple times (it just reinstalls the same handler). See the
// "Diagnostic instrumentation" block in bridge.c for full rationale.
void bridge_init_diagnostics(void);

// ---------------------------------------------------------------------------
// World management
// ---------------------------------------------------------------------------

void bridge_create_world(float gx, float gy);
void bridge_destroy_world(void);
void bridge_set_gravity(float gx, float gy);
bool bridge_world_exists(void);
// Returns the world id packed as uint32 (0 = no world).
uint32_t bridge_get_world_id(void);

// ---------------------------------------------------------------------------
// Body management
// ---------------------------------------------------------------------------

// Creates a body in the world. Returns true on success.
bool bridge_create_body(
    uint32_t entity_id, uint8_t body_type,
    float px, float py, float angle,
    float vx, float vy, float av,
    float linear_damping, float angular_damping, float gravity_scale,
    bool enabled, bool awake, bool sleep_enabled,
    bool bullet, bool fixed_rotation);

void bridge_destroy_body(uint32_t entity_id);
void bridge_destroy_all_bodies(void);

// ---------------------------------------------------------------------------
// Shape attachment (call after bridge_create_body)
// ---------------------------------------------------------------------------

bool bridge_add_circle_shape(
    uint32_t entity_id, int32_t shape_index,
    float offset_x, float offset_y, float radius,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group);

bool bridge_add_box_shape(
    uint32_t entity_id, int32_t shape_index,
    float offset_x, float offset_y,
    float half_w, float half_h, float local_rotation,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group);

bool bridge_add_polygon_shape(
    uint32_t entity_id, int32_t shape_index,
    const BridgeVec2* vertices, int32_t vertex_count,
    float offset_x, float offset_y, float local_rotation,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group);

bool bridge_add_chain_shape(
    uint32_t entity_id, int32_t shape_index,
    const BridgeVec2* points, int32_t point_count, bool is_loop,
    float friction, float restitution,
    uint64_t cat, uint64_t mask, int32_t group);

bool bridge_add_segment_shape(
    uint32_t entity_id, int32_t shape_index,
    float v1x, float v1y, float v2x, float v2y,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group);

bool bridge_add_capsule_shape(
    uint32_t entity_id, int32_t shape_index,
    float c1x, float c1y, float c2x, float c2y, float radius,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group);

// Destroy all shapes/chains on a body (for structural rebuild).
void bridge_destroy_all_shapes(uint32_t entity_id);

// ---------------------------------------------------------------------------
// Body state setters (individual CGO calls, used during reconcile diffs)
// ---------------------------------------------------------------------------

void bridge_set_transform(uint32_t entity_id, float px, float py, float angle);
void bridge_set_linear_velocity(uint32_t entity_id, float vx, float vy);
void bridge_set_angular_velocity(uint32_t entity_id, float av);
void bridge_set_body_type(uint32_t entity_id, uint8_t body_type);
void bridge_set_linear_damping(uint32_t entity_id, float damping);
void bridge_set_angular_damping(uint32_t entity_id, float damping);
void bridge_set_gravity_scale(uint32_t entity_id, float scale);
void bridge_set_body_enabled(uint32_t entity_id, bool enabled);
void bridge_set_awake(uint32_t entity_id, bool awake);
void bridge_set_sleep_enabled(uint32_t entity_id, bool enabled);
void bridge_set_bullet(uint32_t entity_id, bool flag);
void bridge_set_fixed_rotation(uint32_t entity_id, bool flag);
void bridge_reset_mass_data(uint32_t entity_id);

// ---------------------------------------------------------------------------
// Per-shape mutable setters (used for non-structural shape diffs)
// ---------------------------------------------------------------------------

void bridge_set_shape_friction(uint32_t entity_id, int32_t shape_index, float friction);
void bridge_set_shape_restitution(uint32_t entity_id, int32_t shape_index, float restitution);
void bridge_set_shape_density(uint32_t entity_id, int32_t shape_index, float density);
void bridge_set_shape_filter(
    uint32_t entity_id, int32_t shape_index,
    uint64_t cat, uint64_t mask, int32_t group);

// ---------------------------------------------------------------------------
// Stepping — two-phase: advance runs b2World_Step and returns exact event
// counts; drain copies body state + events into caller buffers sized from
// those counts. Splitting the call guarantees the caller buffer is always
// large enough — no dropped events — at the cost of one extra CGO crossing
// per tick (~100ns, negligible next to the physics step).
//
// Invariant the caller MUST uphold: nothing may call b2World_Step or mutate
// world state (CreateBody, DestroyBody, SetTransform, etc.) between
// bridge_step_advance and bridge_step_drain. The drain re-reads Box2D's
// internal event arrays, which are only stable until the next b2World_Step.
// In practice the physics system holds serialised access across both calls.
// ---------------------------------------------------------------------------

// bridge_step_advance runs the physics step and returns exact counts the
// caller must use to size its out_states / out_contacts buffers.
BridgeStepCounts bridge_step_advance(float dt, int32_t sub_step_count);

// bridge_step_drain copies body states + contact events into caller buffers.
// Returns 0 on success. Returns -1 if max_states or max_contacts is smaller
// than the counts from the most recent bridge_step_advance — caller is
// expected to panic on that, since it indicates a Go-side sizing bug.
int32_t bridge_step_drain(
    BridgeBodyState* out_states, int32_t max_states,
    BridgeContactEvent* out_contacts, int32_t max_contacts);

// ---------------------------------------------------------------------------
// Queries (individual CGO calls, infrequent)
// ---------------------------------------------------------------------------

BridgeRaycastResult bridge_raycast(
    float ox, float oy, float ex, float ey,
    uint64_t cat, uint64_t mask, bool include_sensors);

int32_t bridge_overlap_aabb(
    float minx, float miny, float maxx, float maxy,
    uint64_t cat, uint64_t mask, bool include_sensors,
    BridgeOverlapHit* out_hits, int32_t max_hits);

BridgeCircleSweepResult bridge_circle_sweep(
    float sx, float sy, float ex, float ey, float radius,
    uint64_t cat, uint64_t mask, bool include_sensors,
    float max_fraction);

// ---------------------------------------------------------------------------
// Live contact gathering (for post-rebuild diff against persisted state)
// ---------------------------------------------------------------------------

int32_t bridge_gather_live_contacts(
    BridgeContactEvent* out_contacts, int32_t max_contacts);

#endif // PHYSICS2D_BRIDGE_H
