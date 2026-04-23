// bridge.c — CGO bridge between Go and Box2D v3 for physics2d plugin.
// Single global world, entity→body hash table (uint32_map), shape tracking.

#include "box2d/box2d.h"
#include "bridge.h"
#include "uint32_map.h"
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <stdbool.h>
#include <stdint.h>

// ---------------------------------------------------------------------------
// Diagnostic instrumentation — routes Box2D B2_ASSERT failures through the
// Go structured logger instead of letting them reach __builtin_trap(). See
// diagnostics.go for the full rationale.
//
// Usage: any bridge_* function that can reach a Box2D assertion should call
// BRIDGE_OP(label, entity_id, shape_index) at entry to record context for
// the assert handler. g_cur_op is _Thread_local and is not cleared on exit:
// it is only read from inside the assert handler, which only runs while we
// are inside a bridge_* call, so stale values between calls are harmless.
// ---------------------------------------------------------------------------

// cgo maps *C.char to char* (not const char*), so const strings are cast at
// the call site inside bridge_assert_handler.
extern void bridgeOnBox2DAssert(
    char* condition, char* file_name, int line_number,
    char* op, uint32_t entity_id, int32_t shape_index);

static _Thread_local struct {
    const char* op;
    uint32_t    entity_id;
    int32_t     shape_idx;
} g_cur_op = { "none", 0, -1 };

#define BRIDGE_OP(name, eid, sidx) do {   \
    g_cur_op.op = (name);                 \
    g_cur_op.entity_id = (uint32_t)(eid); \
    g_cur_op.shape_idx = (int32_t)(sidx); \
} while (0)

// bridgeOnBox2DAssert calls os.Exit after logging, so this never returns in
// practice. We still return 1 as a safety net: if Go's exit path is ever
// broken, Box2D will __builtin_trap() rather than silently resume past a
// failed precondition.
static int bridge_assert_handler(const char* condition,
                                  const char* file_name,
                                  int line_number) {
    bridgeOnBox2DAssert(
        (char*)condition, (char*)file_name, line_number,
        (char*)g_cur_op.op, g_cur_op.entity_id, g_cur_op.shape_idx);
    return 1;
}

void bridge_init_diagnostics(void) {
    b2SetAssertFcn(bridge_assert_handler);
}

// ---------------------------------------------------------------------------
// Body type mapping: ECS constants → Box2D v3 enum
// ---------------------------------------------------------------------------

static b2BodyType ecs_to_b2_body_type(uint8_t ecs_type) {
    switch (ecs_type) {
    case BRIDGE_BODY_DYNAMIC:   return b2_dynamicBody;
    case BRIDGE_BODY_KINEMATIC: return b2_kinematicBody;
    default:                    return b2_staticBody;
    }
}

// ---------------------------------------------------------------------------
// Body entry — per-entity storage on the C side
//
// Shapes and chains are stored in grow-on-append dynamic buffers so no
// legitimate game hits an arbitrary per-body ceiling (a static "world"
// body holding thousands of terrain chain strips is a supported pattern).
// MAX_SHAPES_HARD / MAX_CHAINS_HARD are sanity caps only — they exist to
// catch runaway attach-in-a-loop bugs before they exhaust shard memory,
// not to express an expected limit. A body that legitimately needs more
// is rare enough to warrant bumping the constant.
// ---------------------------------------------------------------------------

#define MAX_SHAPES_HARD 4096
#define MAX_CHAINS_HARD 4096

typedef struct {
    uint32_t  entity_id;
    b2BodyId  body_id;
    uint8_t   ecs_body_type;

    b2ShapeId* shapes;
    int        shape_count;
    int        shape_cap;

    b2ChainId* chains;
    int32_t*   chain_shape_indices; // parallel to chains: which shape_index each chain maps to
    int        chain_count;
    int        chain_cap;

    bool      occupied;
} BodyEntry;

// ---------------------------------------------------------------------------
// Global state
//
// body_map is an entity_id -> index-into-bodies[] lookup, implemented by
// uint32_map.c.
// ---------------------------------------------------------------------------

static struct {
    b2WorldId world_id;
    bool      world_exists;

    BodyEntry* bodies;
    int        body_count;
    int        body_cap;

    U32Map     body_map;
} g;

// ---------------------------------------------------------------------------
// Body array helpers
// ---------------------------------------------------------------------------

// Resolve entity_id to a live BodyEntry, or NULL if unknown / torn down.
static BodyEntry* get_entry(uint32_t entity_id) {
    int idx = u32map_find(&g.body_map, entity_id);
    if (idx < 0 || idx >= g.body_count) return NULL;
    BodyEntry* e = &g.bodies[idx];
    if (!e->occupied) return NULL;
    return e;
}

// Grow g.bodies when full (first allocation 64 slots, then double).
static void ensure_body_cap(void) {
    if (g.body_count >= g.body_cap) {
        int new_cap = g.body_cap == 0 ? 64 : g.body_cap * 2;
        g.bodies = (BodyEntry*)realloc(g.bodies, sizeof(BodyEntry) * (size_t)new_cap);
        g.body_cap = new_cap;
    }
}

// Grow entry->shapes so it has room for at least `needed` elements.
// Doubles capacity starting from 4, clamped to MAX_SHAPES_HARD. Returns
// false if `needed` exceeds the hard cap — callers treat that as a
// rejected attach, which fails the offending bridge call loudly instead
// of silently growing unbounded memory on a runaway attach bug.
static bool grow_shapes(BodyEntry* e, int needed) {
    if (needed <= e->shape_cap) return true;
    if (needed > MAX_SHAPES_HARD) return false;
    int new_cap = e->shape_cap == 0 ? 4 : e->shape_cap;
    while (new_cap < needed) new_cap *= 2;
    if (new_cap > MAX_SHAPES_HARD) new_cap = MAX_SHAPES_HARD;
    e->shapes = (b2ShapeId*)realloc(e->shapes, sizeof(b2ShapeId) * (size_t)new_cap);
    e->shape_cap = new_cap;
    return true;
}

// Grow entry->chains (and its parallel chain_shape_indices array) in
// lockstep. Same doubling / hard-cap discipline as grow_shapes.
static bool grow_chains(BodyEntry* e, int needed) {
    if (needed <= e->chain_cap) return true;
    if (needed > MAX_CHAINS_HARD) return false;
    int new_cap = e->chain_cap == 0 ? 2 : e->chain_cap;
    while (new_cap < needed) new_cap *= 2;
    if (new_cap > MAX_CHAINS_HARD) new_cap = MAX_CHAINS_HARD;
    e->chains = (b2ChainId*)realloc(e->chains, sizeof(b2ChainId) * (size_t)new_cap);
    e->chain_shape_indices = (int32_t*)realloc(e->chain_shape_indices, sizeof(int32_t) * (size_t)new_cap);
    e->chain_cap = new_cap;
    return true;
}

// Release a body entry's per-body heap buffers. Safe to call on an
// already-empty entry (free(NULL) is a no-op). Nulls the pointers and
// resets counts/caps so the struct is in a consistent zero state — the
// next create_body memset will overwrite it anyway, but explicit reset
// keeps the ownership story clear at destroy-time.
static void free_body_buffers(BodyEntry* e) {
    free(e->shapes);
    free(e->chains);
    free(e->chain_shape_indices);
    e->shapes = NULL;
    e->chains = NULL;
    e->chain_shape_indices = NULL;
    e->shape_count = 0;
    e->shape_cap = 0;
    e->chain_count = 0;
    e->chain_cap = 0;
}

// Shrink backing storage when utilisation drops below 25%. Called after
// destroying a body so a spike of short-lived entities (projectile burst,
// crowd wave) doesn't permanently inflate the shard's resident set. A 25%
// threshold plus halving leaves >= 50% headroom after the shrink, so a
// burst immediately followed by steady-state growth does not thrash.
// A minimum cap of 64 avoids rapid churn at small entity counts.
static void maybe_shrink_storage(void) {
    if (g.body_cap > 64 && g.body_count * 4 < g.body_cap) {
        int new_cap = g.body_cap / 2;
        g.bodies = (BodyEntry*)realloc(g.bodies, sizeof(BodyEntry) * (size_t)new_cap);
        g.body_cap = new_cap;
    }
    // u32map_maybe_shrink rehashes every live entry; safe to call here
    // because we're outside any cluster walk in u32map_remove.
    u32map_maybe_shrink(&g.body_map);
}

// ---------------------------------------------------------------------------
// Shape user data packing: store shape_index as void*
// Body user data packing: store entity_id as void*
// ---------------------------------------------------------------------------

static void* pack_uint32(uint32_t v)  { return (void*)(uintptr_t)v; }
static uint32_t unpack_uint32(void* p) { return (uint32_t)(uintptr_t)p; }
static void* pack_int32(int32_t v)    { return (void*)(uintptr_t)(uint32_t)v; }
static int32_t unpack_int32(void* p)  { return (int32_t)(uint32_t)(uintptr_t)p; }

// ---------------------------------------------------------------------------
// Common shape def setup
// ---------------------------------------------------------------------------

static b2ShapeDef make_shape_def(int32_t shape_index, bool is_sensor,
                                  float friction, float restitution, float density,
                                  uint64_t cat, uint64_t mask, int32_t group) {
    b2ShapeDef def = b2DefaultShapeDef();
    def.userData = pack_int32(shape_index);
    def.material.friction = friction;
    def.material.restitution = restitution;
    def.density = density;
    def.isSensor = is_sensor;
    def.enableSensorEvents = true;
    def.enableContactEvents = true;
    def.filter.categoryBits = cat;
    def.filter.maskBits = mask;
    def.filter.groupIndex = group;
    return def;
}

// Register a shape in the body entry. Returns true if successful.
// Rejects out-of-range shape_index (negative or past the hard cap) and
// destroys the already-created b2Shape so Box2D state stays in sync with
// the bridge's shapes[] tracking array.
static bool register_shape(BodyEntry* entry, int32_t shape_index, b2ShapeId shape_id) {
    if (shape_index < 0 || shape_index >= MAX_SHAPES_HARD) {
        b2DestroyShape(shape_id, false);
        return false;
    }
    if (!grow_shapes(entry, shape_index + 1)) {
        b2DestroyShape(shape_id, false);
        return false;
    }
    // Grow shape_count up to shape_index, filling gaps with null sentinels.
    while (entry->shape_count <= shape_index) {
        entry->shapes[entry->shape_count] = b2_nullShapeId;
        entry->shape_count++;
    }
    entry->shapes[shape_index] = shape_id;
    return true;
}

// ---------------------------------------------------------------------------
// World management
// ---------------------------------------------------------------------------

void bridge_create_world(float gx, float gy) {
    BRIDGE_OP("create_world", 0, -1);
    if (g.world_exists) return;
    b2WorldDef def = b2DefaultWorldDef();
    def.gravity = (b2Vec2){gx, gy};
    g.world_id = b2CreateWorld(&def);
    g.world_exists = true;
}

// Declared below with the gather_live_contacts helpers; forward decl so
// bridge_destroy_world can release the persistent dedup buffer.
static void gather_dedup_free(void);

void bridge_destroy_world(void) {
    BRIDGE_OP("destroy_world", 0, -1);
    if (!g.world_exists) return;
    b2DestroyWorld(g.world_id);
    g.world_exists = false;

    // Free per-body sub-buffers before releasing the flat body array.
    for (int i = 0; i < g.body_count; i++) {
        if (g.bodies[i].occupied) {
            free_body_buffers(&g.bodies[i]);
        }
    }
    free(g.bodies);
    g.bodies = NULL;
    g.body_count = 0;
    g.body_cap = 0;

    // Free hash table
    u32map_free(&g.body_map);

    // Free persistent gather-live-contacts dedup scratch
    gather_dedup_free();
}

void bridge_set_gravity(float gx, float gy) {
    BRIDGE_OP("set_gravity", 0, -1);
    if (!g.world_exists) return;
    b2World_SetGravity(g.world_id, (b2Vec2){gx, gy});
}

bool bridge_world_exists(void) {
    return g.world_exists;
}

uint32_t bridge_get_world_id(void) {
    if (!g.world_exists) return 0;
    return b2StoreWorldId(g.world_id);
}

// ---------------------------------------------------------------------------
// Body management
// ---------------------------------------------------------------------------

bool bridge_create_body(
    uint32_t entity_id, uint8_t body_type,
    float px, float py, float angle,
    float vx, float vy, float av,
    float linear_damping, float angular_damping, float gravity_scale,
    bool enabled, bool awake, bool sleep_enabled,
    bool bullet, bool fixed_rotation)
{
    BRIDGE_OP("create_body", entity_id, -1);
    if (!g.world_exists) return false;
    if (u32map_find(&g.body_map, entity_id) >= 0) return false; // already exists

    ensure_body_cap();

    b2BodyDef def = b2DefaultBodyDef();
    def.type = ecs_to_b2_body_type(body_type);
    def.position = (b2Vec2){px, py};
    def.rotation = b2MakeRot(angle);
    def.linearVelocity = (b2Vec2){vx, vy};
    def.angularVelocity = av;
    def.linearDamping = linear_damping;
    def.angularDamping = angular_damping;
    def.gravityScale = gravity_scale;
    def.userData = pack_uint32(entity_id);
    def.enableSleep = sleep_enabled;
    def.isAwake = awake;
    def.motionLocks.angularZ = fixed_rotation;
    def.isBullet = bullet;
    def.isEnabled = enabled;

    b2BodyId body_id = b2CreateBody(g.world_id, &def);

    int idx = g.body_count;
    g.body_count++;

    BodyEntry* entry = &g.bodies[idx];
    memset(entry, 0, sizeof(BodyEntry));
    entry->entity_id = entity_id;
    entry->body_id = body_id;
    entry->ecs_body_type = body_type;
    entry->shape_count = 0;
    entry->chain_count = 0;
    entry->occupied = true;

    u32map_insert(&g.body_map, entity_id, idx);
    return true;
}

void bridge_destroy_body(uint32_t entity_id) {
    BRIDGE_OP("destroy_body", entity_id, -1);
    if (!g.world_exists) return;
    int idx = u32map_find(&g.body_map, entity_id);
    if (idx < 0) return;

    BodyEntry* entry = &g.bodies[idx];
    if (!entry->occupied) return;

    b2DestroyBody(entry->body_id);
    free_body_buffers(entry);
    entry->occupied = false;

    u32map_remove(&g.body_map, entity_id);

    // Swap-remove compaction. bodies[last] is guaranteed occupied by the
    // invariant that bodies[0..body_count-1] are all live — the only entry
    // we just marked unoccupied is bodies[idx], which is not bodies[last]
    // in this branch. Using u32map_update (not u32map_insert) here makes
    // the intent explicit: we are rewriting an existing key's value, not
    // inserting a new one, so no resize can run.
    //
    // The struct copy transfers ownership of bodies[last]'s shape/chain
    // buffers into bodies[idx]; we then zero bodies[last] so the now-
    // aliased pointers can't be freed a second time if the slot is reused
    // or walked during shrink/teardown.
    int last = g.body_count - 1;
    if (idx != last) {
        g.bodies[idx] = g.bodies[last];
        memset(&g.bodies[last], 0, sizeof(BodyEntry));
        u32map_update(&g.body_map, g.bodies[idx].entity_id, idx);
    }
    g.body_count--;

    maybe_shrink_storage();
}

void bridge_destroy_all_bodies(void) {
    BRIDGE_OP("destroy_all_bodies", 0, -1);
    if (!g.world_exists) return;
    for (int i = 0; i < g.body_count; i++) {
        if (g.bodies[i].occupied) {
            b2DestroyBody(g.bodies[i].body_id);
            free_body_buffers(&g.bodies[i]);
        }
    }
    g.body_count = 0;
    u32map_clear(&g.body_map);
}

// ---------------------------------------------------------------------------
// Shape attachment
// ---------------------------------------------------------------------------

bool bridge_add_circle_shape(
    uint32_t entity_id, int32_t shape_index,
    float offset_x, float offset_y, float radius,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group)
{
    BRIDGE_OP("add_circle_shape", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return false;

    b2ShapeDef def = make_shape_def(shape_index, is_sensor, friction, restitution, density, cat, mask, group);
    b2Circle circle = { .center = {offset_x, offset_y}, .radius = radius };
    b2ShapeId sid = b2CreateCircleShape(entry->body_id, &def, &circle);
    return register_shape(entry, shape_index, sid);
}

bool bridge_add_box_shape(
    uint32_t entity_id, int32_t shape_index,
    float offset_x, float offset_y,
    float half_w, float half_h, float local_rotation,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group)
{
    BRIDGE_OP("add_box_shape", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return false;

    b2ShapeDef def = make_shape_def(shape_index, is_sensor, friction, restitution, density, cat, mask, group);
    b2Vec2 center = {offset_x, offset_y};
    b2Rot rot = b2MakeRot(local_rotation);
    b2Polygon box = b2MakeOffsetBox(half_w, half_h, center, rot);
    b2ShapeId sid = b2CreatePolygonShape(entry->body_id, &def, &box);
    return register_shape(entry, shape_index, sid);
}

bool bridge_add_polygon_shape(
    uint32_t entity_id, int32_t shape_index,
    const BridgeVec2* vertices, int32_t vertex_count,
    float offset_x, float offset_y, float local_rotation,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group)
{
    BRIDGE_OP("add_polygon_shape", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return false;
    if (vertex_count < 3 || vertex_count > B2_MAX_POLYGON_VERTICES) return false;

    b2ShapeDef def = make_shape_def(shape_index, is_sensor, friction, restitution, density, cat, mask, group);

    // Transform vertices from shape-local space to body space using offset and rotation
    float cos_r = cosf(local_rotation);
    float sin_r = sinf(local_rotation);
    b2Vec2 points[B2_MAX_POLYGON_VERTICES];
    for (int i = 0; i < vertex_count; i++) {
        float lx = vertices[i].x;
        float ly = vertices[i].y;
        points[i].x = cos_r * lx - sin_r * ly + offset_x;
        points[i].y = sin_r * lx + cos_r * ly + offset_y;
    }

    b2Hull hull = b2ComputeHull(points, vertex_count);
    if (hull.count == 0) return false; // degenerate polygon

    b2Polygon polygon = b2MakePolygon(&hull, 0.0f);
    b2ShapeId sid = b2CreatePolygonShape(entry->body_id, &def, &polygon);
    return register_shape(entry, shape_index, sid);
}

bool bridge_add_chain_shape(
    uint32_t entity_id, int32_t shape_index,
    const BridgeVec2* points, int32_t point_count, bool is_loop,
    float friction, float restitution,
    uint64_t cat, uint64_t mask, int32_t group)
{
    BRIDGE_OP("add_chain_shape", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return false;
    if (!grow_chains(entry, entry->chain_count + 1)) return false;

    // Copy points to b2Vec2 array
    b2Vec2* b2pts = (b2Vec2*)malloc(sizeof(b2Vec2) * (size_t)point_count);
    for (int i = 0; i < point_count; i++) {
        b2pts[i] = (b2Vec2){points[i].x, points[i].y};
    }

    b2ChainDef def = b2DefaultChainDef();
    def.points = b2pts;
    def.count = point_count;
    def.isLoop = is_loop;
    b2SurfaceMaterial chain_mat = b2DefaultSurfaceMaterial();
    chain_mat.friction = friction;
    chain_mat.restitution = restitution;
    def.materials = &chain_mat;
    def.materialCount = 1;
    def.filter.categoryBits = cat;
    def.filter.maskBits = mask;
    def.filter.groupIndex = group;

    b2ChainId cid = b2CreateChain(entry->body_id, &def);
    free(b2pts);

    int ci = entry->chain_count;
    entry->chains[ci] = cid;
    entry->chain_shape_indices[ci] = shape_index;
    entry->chain_count++;
    return true;
}

bool bridge_add_segment_shape(
    uint32_t entity_id, int32_t shape_index,
    float v1x, float v1y, float v2x, float v2y,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group)
{
    BRIDGE_OP("add_segment_shape", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return false;

    b2ShapeDef def = make_shape_def(shape_index, is_sensor, friction, restitution, density, cat, mask, group);
    b2Segment seg = { .point1 = {v1x, v1y}, .point2 = {v2x, v2y} };
    b2ShapeId sid = b2CreateSegmentShape(entry->body_id, &def, &seg);
    return register_shape(entry, shape_index, sid);
}

bool bridge_add_capsule_shape(
    uint32_t entity_id, int32_t shape_index,
    float c1x, float c1y, float c2x, float c2y, float radius,
    bool is_sensor, float friction, float restitution, float density,
    uint64_t cat, uint64_t mask, int32_t group)
{
    BRIDGE_OP("add_capsule_shape", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return false;

    b2ShapeDef def = make_shape_def(shape_index, is_sensor, friction, restitution, density, cat, mask, group);
    b2Capsule capsule = { .center1 = {c1x, c1y}, .center2 = {c2x, c2y}, .radius = radius };
    b2ShapeId sid = b2CreateCapsuleShape(entry->body_id, &def, &capsule);
    return register_shape(entry, shape_index, sid);
}

void bridge_destroy_all_shapes(uint32_t entity_id) {
    BRIDGE_OP("destroy_all_shapes", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;

    // Order matters: b2DestroyChain frees all segment shapes owned by the
    // chain, so chains MUST be destroyed before the b2Body_GetShapes call
    // below. Doing it the other way around would enumerate chain-owned
    // segments and then b2DestroyChain would free them a second time —
    // double-destroy. After the chain destroys here, the subsequent shape
    // enumeration returns only non-chain shapes and is safe to walk.
    for (int i = 0; i < entry->chain_count; i++) {
        b2DestroyChain(entry->chains[i]);
    }
    entry->chain_count = 0;

    // Destroy regular (non-chain) shapes — query the actual shapes from Box2D.
    int count = b2Body_GetShapeCount(entry->body_id);
    if (count > 0) {
        b2ShapeId* sids = (b2ShapeId*)malloc(sizeof(b2ShapeId) * (size_t)count);
        int got = b2Body_GetShapes(entry->body_id, sids, count);
        for (int i = 0; i < got; i++) {
            b2DestroyShape(sids[i], false);
        }
        free(sids);
    }
    entry->shape_count = 0;

    // Recalculate mass after removing all shapes
    b2Body_ApplyMassFromShapes(entry->body_id);
}

// ---------------------------------------------------------------------------
// Body state setters
// ---------------------------------------------------------------------------

void bridge_set_transform(uint32_t entity_id, float px, float py, float angle) {
    BRIDGE_OP("set_transform", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetTransform(entry->body_id, (b2Vec2){px, py}, b2MakeRot(angle));
}

void bridge_set_linear_velocity(uint32_t entity_id, float vx, float vy) {
    BRIDGE_OP("set_linear_velocity", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetLinearVelocity(entry->body_id, (b2Vec2){vx, vy});
}

void bridge_set_angular_velocity(uint32_t entity_id, float av) {
    BRIDGE_OP("set_angular_velocity", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetAngularVelocity(entry->body_id, av);
}

void bridge_set_body_type(uint32_t entity_id, uint8_t body_type) {
    BRIDGE_OP("set_body_type", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    entry->ecs_body_type = body_type;
    b2Body_SetType(entry->body_id, ecs_to_b2_body_type(body_type));
}

void bridge_set_linear_damping(uint32_t entity_id, float damping) {
    BRIDGE_OP("set_linear_damping", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetLinearDamping(entry->body_id, damping);
}

void bridge_set_angular_damping(uint32_t entity_id, float damping) {
    BRIDGE_OP("set_angular_damping", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetAngularDamping(entry->body_id, damping);
}

void bridge_set_gravity_scale(uint32_t entity_id, float scale) {
    BRIDGE_OP("set_gravity_scale", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetGravityScale(entry->body_id, scale);
}

void bridge_set_body_enabled(uint32_t entity_id, bool enabled) {
    BRIDGE_OP("set_body_enabled", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    if (enabled && !b2Body_IsEnabled(entry->body_id)) {
        b2Body_Enable(entry->body_id);
    } else if (!enabled && b2Body_IsEnabled(entry->body_id)) {
        b2Body_Disable(entry->body_id);
    }
}

void bridge_set_awake(uint32_t entity_id, bool awake) {
    BRIDGE_OP("set_awake", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetAwake(entry->body_id, awake);
}

void bridge_set_sleep_enabled(uint32_t entity_id, bool enabled) {
    BRIDGE_OP("set_sleep_enabled", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_EnableSleep(entry->body_id, enabled);
}

void bridge_set_bullet(uint32_t entity_id, bool flag) {
    BRIDGE_OP("set_bullet", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_SetBullet(entry->body_id, flag);
}

void bridge_set_fixed_rotation(uint32_t entity_id, bool flag) {
    BRIDGE_OP("set_fixed_rotation", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2MotionLocks locks = b2Body_GetMotionLocks(entry->body_id);
    locks.angularZ = flag;
    b2Body_SetMotionLocks(entry->body_id, locks);
}

void bridge_reset_mass_data(uint32_t entity_id) {
    BRIDGE_OP("reset_mass_data", entity_id, -1);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2Body_ApplyMassFromShapes(entry->body_id);
}


// ---------------------------------------------------------------------------
// Per-shape mutable setters
// ---------------------------------------------------------------------------

// Resolve a shape_index to a b2ShapeId. Returns true if found.
static bool resolve_shape(BodyEntry* entry, int32_t shape_index, b2ShapeId* out) {
    if (shape_index >= 0 && shape_index < entry->shape_count) {
        b2ShapeId sid = entry->shapes[shape_index];
        if (B2_IS_NON_NULL(sid)) {
            *out = sid;
            return true;
        }
    }
    return false;
}

void bridge_set_shape_friction(uint32_t entity_id, int32_t shape_index, float friction) {
    BRIDGE_OP("set_shape_friction", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2ShapeId sid;
    if (!resolve_shape(entry, shape_index, &sid)) return;
    b2Shape_SetFriction(sid, friction);
}

void bridge_set_shape_restitution(uint32_t entity_id, int32_t shape_index, float restitution) {
    BRIDGE_OP("set_shape_restitution", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2ShapeId sid;
    if (!resolve_shape(entry, shape_index, &sid)) return;
    b2Shape_SetRestitution(sid, restitution);
}

void bridge_set_shape_density(uint32_t entity_id, int32_t shape_index, float density) {
    BRIDGE_OP("set_shape_density", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2ShapeId sid;
    if (!resolve_shape(entry, shape_index, &sid)) return;
    b2Shape_SetDensity(sid, density, true);
}

void bridge_set_shape_filter(uint32_t entity_id, int32_t shape_index,
                              uint64_t cat, uint64_t mask, int32_t group) {
    BRIDGE_OP("set_shape_filter", entity_id, shape_index);
    BodyEntry* entry = get_entry(entity_id);
    if (!entry) return;
    b2ShapeId sid;
    if (!resolve_shape(entry, shape_index, &sid)) return;
    b2Filter filter;
    filter.categoryBits = cat;
    filter.maskBits = mask;
    filter.groupIndex = group;
    b2Shape_SetFilter(sid, filter);
}

// ---------------------------------------------------------------------------
// Contact event helpers
// ---------------------------------------------------------------------------

// Fill a BridgeContactEvent from two shape IDs and a kind.
static void fill_contact_event(BridgeContactEvent* out, uint8_t kind,
                                b2ShapeId sidA, b2ShapeId sidB,
                                const b2Manifold* manifold) {
    b2BodyId bodyA = b2Shape_GetBody(sidA);
    b2BodyId bodyB = b2Shape_GetBody(sidB);

    out->kind = kind;
    out->entity_a = unpack_uint32(b2Body_GetUserData(bodyA));
    out->entity_b = unpack_uint32(b2Body_GetUserData(bodyB));
    out->shape_index_a = unpack_int32(b2Shape_GetUserData(sidA));
    out->shape_index_b = unpack_int32(b2Shape_GetUserData(sidB));
    out->is_sensor = b2Shape_IsSensor(sidA) || b2Shape_IsSensor(sidB);

    b2Filter filterA = b2Shape_GetFilter(sidA);
    b2Filter filterB = b2Shape_GetFilter(sidB);
    out->cat_a = filterA.categoryBits;
    out->mask_a = filterA.maskBits;
    out->group_a = filterA.groupIndex;
    out->cat_b = filterB.categoryBits;
    out->mask_b = filterB.maskBits;
    out->group_b = filterB.groupIndex;

    if (manifold && manifold->pointCount > 0) {
        out->normal_x = manifold->normal.x;
        out->normal_y = manifold->normal.y;
        out->normal_valid = true;
        out->point_x = manifold->points[0].anchorA.x;
        out->point_y = manifold->points[0].anchorA.y;
        out->point_valid = true;
        out->manifold_point_count = manifold->pointCount;
    } else {
        out->normal_x = 0;
        out->normal_y = 0;
        out->normal_valid = false;
        out->point_x = 0;
        out->point_y = 0;
        out->point_valid = false;
        out->manifold_point_count = 0;
    }
}

// ---------------------------------------------------------------------------
// Stepping
// ---------------------------------------------------------------------------

// Phase 1: advance the world and return exact counts for caller-buffer sizing.
//
// The counts are exact — we walk sensor-end events once and drop those that
// reference destroyed shapes (see Phase 2 for the rationale) so the Go side
// allocates only what drain will actually write. This lets bridge_step_drain
// operate under the hard guarantee "no dropped events".
//
// Static bodies are excluded from the body count. They never move, so the
// reconcile system has nothing to do for them — writing back their state
// every tick is pure waste proportional to the size of the static world.
// Phase 2 applies the identical filter so the counts match the writes.
BridgeStepCounts bridge_step_advance(float dt, int32_t sub_step_count) {
    BRIDGE_OP("step_advance", 0, -1);
    BridgeStepCounts counts = {0, 0};
    if (!g.world_exists) return counts;

    b2World_Step(g.world_id, dt, sub_step_count);

    b2ContactEvents events = b2World_GetContactEvents(g.world_id);
    b2SensorEvents sensors = b2World_GetSensorEvents(g.world_id);

    int n = events.beginCount + events.endCount + sensors.beginCount;
    for (int i = 0; i < sensors.endCount; i++) {
        if (b2Shape_IsValid(sensors.endEvents[i].sensorShapeId) &&
            b2Shape_IsValid(sensors.endEvents[i].visitorShapeId)) {
            n++;
        }
    }
    counts.contact_event_count = n;

    // Count only non-static bodies — must stay in lockstep with the drain
    // filter below.
    int live = 0;
    for (int i = 0; i < g.body_count; i++) {
        if (g.bodies[i].ecs_body_type != BRIDGE_BODY_STATIC) live++;
    }
    counts.body_count = live;
    return counts;
}

// Phase 2: copy contact events + body states into caller buffers.
//
// Must be called exactly once per bridge_step_advance, with caller buffers
// sized to the counts that advance returned. The Go wrapper upholds this
// invariant. If max_* is smaller we return -1 and write nothing useful; the
// wrapper panics on that to surface the Go-side sizing bug loudly.
//
// Box2D's internal event arrays are stable between b2World_Step calls, so
// we can re-read them here even though advance already read them once.
int32_t bridge_step_drain(
    BridgeBodyState* out_states, int32_t max_states,
    BridgeContactEvent* out_contacts, int32_t max_contacts)
{
    BRIDGE_OP("step_drain", 0, -1);
    if (!g.world_exists) return 0;

    b2ContactEvents events = b2World_GetContactEvents(g.world_id);
    b2SensorEvents sensors = b2World_GetSensorEvents(g.world_id);
    int ci = 0;

    // -- Begin events (contact) --
    for (int i = 0; i < events.beginCount; i++) {
        if (ci >= max_contacts) return -1;
        const b2Manifold* manifold = NULL;
        b2ContactData cd;
        if (b2Contact_IsValid(events.beginEvents[i].contactId)) {
            cd = b2Contact_GetData(events.beginEvents[i].contactId);
            manifold = &cd.manifold;
        }
        fill_contact_event(&out_contacts[ci], BRIDGE_CONTACT_BEGIN,
                           events.beginEvents[i].shapeIdA,
                           events.beginEvents[i].shapeIdB,
                           manifold);
        ci++;
    }

    // -- End events (contact) --
    for (int i = 0; i < events.endCount; i++) {
        if (ci >= max_contacts) return -1;
        fill_contact_event(&out_contacts[ci], BRIDGE_CONTACT_END,
                           events.endEvents[i].shapeIdA,
                           events.endEvents[i].shapeIdB,
                           NULL);
        ci++;
    }

    // -- Begin events (sensor) --
    for (int i = 0; i < sensors.beginCount; i++) {
        if (ci >= max_contacts) return -1;
        fill_contact_event(&out_contacts[ci], BRIDGE_CONTACT_BEGIN,
                           sensors.beginEvents[i].sensorShapeId,
                           sensors.beginEvents[i].visitorShapeId,
                           NULL);
        ci++;
    }

    // -- End events (sensor) --
    // Skip sensor ends that reference destroyed shapes; advance applied the
    // same filter when counting, so the filtered count matches the buffer
    // the caller allocated.
    for (int i = 0; i < sensors.endCount; i++) {
        if (!b2Shape_IsValid(sensors.endEvents[i].sensorShapeId) ||
            !b2Shape_IsValid(sensors.endEvents[i].visitorShapeId)) {
            continue;
        }
        if (ci >= max_contacts) return -1;
        fill_contact_event(&out_contacts[ci], BRIDGE_CONTACT_END,
                           sensors.endEvents[i].sensorShapeId,
                           sensors.endEvents[i].visitorShapeId,
                           NULL);
        ci++;
    }

    // Write back body states for non-static bodies only. Statics never move,
    // so reading their transform/velocity every tick is wasted work
    // proportional to the static world size (walls, terrain chains, etc).
    // The filter here MUST match the one in bridge_step_advance so the
    // caller-supplied buffer is sized correctly.
    int bi = 0;
    for (int i = 0; i < g.body_count; i++) {
        BodyEntry* entry = &g.bodies[i];
        if (entry->ecs_body_type == BRIDGE_BODY_STATIC) continue;
        if (bi >= max_states) return -1;

        b2Vec2 pos = b2Body_GetPosition(entry->body_id);
        float angle = b2Rot_GetAngle(b2Body_GetRotation(entry->body_id));
        b2Vec2 lv = b2Body_GetLinearVelocity(entry->body_id);
        float av = b2Body_GetAngularVelocity(entry->body_id);

        out_states[bi].entity_id = entry->entity_id;
        out_states[bi].body_type = entry->ecs_body_type;
        out_states[bi].px = pos.x;
        out_states[bi].py = pos.y;
        out_states[bi].angle = angle;
        out_states[bi].vx = lv.x;
        out_states[bi].vy = lv.y;
        out_states[bi].av = av;
        bi++;
    }

    return 0;
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

// --- Raycast ---

typedef struct {
    BridgeRaycastResult result;
    bool include_sensors;
} RaycastCtx;

static float raycast_callback(b2ShapeId shapeId, b2Vec2 point, b2Vec2 normal,
                                float fraction, void* context) {
    RaycastCtx* ctx = (RaycastCtx*)context;
    if (!ctx->include_sensors && b2Shape_IsSensor(shapeId)) {
        return -1.0f; // skip sensors
    }

    b2BodyId bodyId = b2Shape_GetBody(shapeId);
    ctx->result.hit = true;
    ctx->result.entity_id = unpack_uint32(b2Body_GetUserData(bodyId));
    ctx->result.shape_index = unpack_int32(b2Shape_GetUserData(shapeId));
    ctx->result.px = point.x;
    ctx->result.py = point.y;
    ctx->result.nx = normal.x;
    ctx->result.ny = normal.y;
    ctx->result.fraction = fraction;
    return fraction; // keep searching for closer hits
}

BridgeRaycastResult bridge_raycast(
    float ox, float oy, float ex, float ey,
    uint64_t cat, uint64_t mask, bool include_sensors)
{
    BRIDGE_OP("raycast", 0, -1);
    RaycastCtx ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.include_sensors = include_sensors;

    if (!g.world_exists) return ctx.result;

    b2Vec2 origin = {ox, oy};
    b2Vec2 translation = {ex - ox, ey - oy};
    b2QueryFilter filter = {cat, mask};

    b2World_CastRay(g.world_id, origin, translation, filter, raycast_callback, &ctx);
    return ctx.result;
}

// --- AABB Overlap ---

typedef struct {
    BridgeOverlapHit* hits;
    int32_t count;
    int32_t max_hits;
    bool include_sensors;
} OverlapCtx;

static bool overlap_callback(b2ShapeId shapeId, void* context) {
    OverlapCtx* ctx = (OverlapCtx*)context;
    if (!ctx->include_sensors && b2Shape_IsSensor(shapeId)) {
        return true; // skip sensor, continue
    }
    if (ctx->count >= ctx->max_hits) return false; // buffer full, stop

    b2BodyId bodyId = b2Shape_GetBody(shapeId);
    ctx->hits[ctx->count].entity_id = unpack_uint32(b2Body_GetUserData(bodyId));
    ctx->hits[ctx->count].shape_index = unpack_int32(b2Shape_GetUserData(shapeId));
    ctx->count++;
    return true; // continue
}

int32_t bridge_overlap_aabb(
    float minx, float miny, float maxx, float maxy,
    uint64_t cat, uint64_t mask, bool include_sensors,
    BridgeOverlapHit* out_hits, int32_t max_hits)
{
    BRIDGE_OP("overlap_aabb", 0, -1);
    if (!g.world_exists) return 0;

    OverlapCtx ctx;
    ctx.hits = out_hits;
    ctx.count = 0;
    ctx.max_hits = max_hits;
    ctx.include_sensors = include_sensors;

    b2AABB aabb = {{minx, miny}, {maxx, maxy}};
    b2QueryFilter filter = {cat, mask};

    b2World_OverlapAABB(g.world_id, aabb, filter, overlap_callback, &ctx);
    return ctx.count;
}

// --- Circle Sweep ---

typedef struct {
    BridgeCircleSweepResult result;
    bool include_sensors;
} CircleSweepCtx;

static float circle_sweep_callback(b2ShapeId shapeId, b2Vec2 point, b2Vec2 normal,
                                     float fraction, void* context) {
    CircleSweepCtx* ctx = (CircleSweepCtx*)context;
    if (!ctx->include_sensors && b2Shape_IsSensor(shapeId)) {
        return -1.0f; // skip sensors
    }

    b2BodyId bodyId = b2Shape_GetBody(shapeId);
    ctx->result.hit = true;
    ctx->result.entity_id = unpack_uint32(b2Body_GetUserData(bodyId));
    ctx->result.shape_index = unpack_int32(b2Shape_GetUserData(shapeId));
    ctx->result.px = point.x;
    ctx->result.py = point.y;
    ctx->result.nx = normal.x;
    ctx->result.ny = normal.y;
    ctx->result.fraction = fraction;
    return fraction; // keep searching for closer hits
}

BridgeCircleSweepResult bridge_circle_sweep(
    float sx, float sy, float ex, float ey, float radius,
    uint64_t cat, uint64_t mask, bool include_sensors,
    float max_fraction)
{
    BRIDGE_OP("circle_sweep", 0, -1);
    CircleSweepCtx ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.include_sensors = include_sensors;

    if (!g.world_exists) return ctx.result;

    // A circle is a single point at the origin with non-zero radius.
    b2ShapeProxy proxy;
    proxy.points[0] = (b2Vec2){sx, sy};
    proxy.count = 1;
    proxy.radius = radius;
    b2Vec2 translation = {(ex - sx) * max_fraction, (ey - sy) * max_fraction};
    b2QueryFilter filter = {cat, mask};

    b2World_CastShape(g.world_id, &proxy, translation, filter,
                       circle_sweep_callback, &ctx);
    return ctx.result;
}

// ---------------------------------------------------------------------------
// Live contact gathering (for post-rebuild diff)
// ---------------------------------------------------------------------------

// Persistent scratch buffers for bridge_gather_live_contacts.
//
// The function is called on post-rebuild reconcile — not every tick, but
// potentially several times in quick succession during a structural diff.
// Re-allocating both buffers on every call was showing up in profiles, so
// they are promoted to file-scope statics that grow monotonically and are
// reused across calls. They are never freed: the shard owns them for its
// lifetime, matching g.bodies / g.body_map. Safe under the same single-thread
// assumption as the rest of this file.
#define GATHER_CONTACT_DATA_CAP 256
static b2ContactData g_gather_cd_buf[GATHER_CONTACT_DATA_CAP];

// Pair-dedup hash set: normalized (entityA, shapeA) vs (entityB, shapeB)
// lexicographically, packed into two uint64_t words. The empty sentinel is
// {0, 0}, which relies on the invariant that Cardinal ECS never issues
// entity id 0 — so a legitimate pair can never hash to {0, 0} because the
// upper 32 bits of at least one of pk.a / pk.b carry a non-zero entity id.
// If that invariant ever changes, this sentinel must too (switch to a
// parallel occupancy bitmap or use a dedicated empty-sentinel like uint32_map).
typedef struct { uint64_t a; uint64_t b; } PairKey;
static PairKey* g_gather_dedup = NULL;
static int      g_gather_dedup_cap = 0;

static void gather_dedup_reset(void) {
    if (g_gather_dedup_cap == 0) {
        g_gather_dedup_cap = 1024;
        g_gather_dedup = (PairKey*)calloc((size_t)g_gather_dedup_cap, sizeof(PairKey));
    } else {
        memset(g_gather_dedup, 0, sizeof(PairKey) * (size_t)g_gather_dedup_cap);
    }
}

static void gather_dedup_free(void) {
    free(g_gather_dedup);
    g_gather_dedup = NULL;
    g_gather_dedup_cap = 0;
}

static void gather_dedup_grow(void) {
    int new_cap = g_gather_dedup_cap * 2;
    PairKey* new_dedup = (PairKey*)calloc((size_t)new_cap, sizeof(PairKey));
    for (int k = 0; k < g_gather_dedup_cap; k++) {
        if (g_gather_dedup[k].a == 0 && g_gather_dedup[k].b == 0) continue;
        uint32_t h = (uint32_t)(g_gather_dedup[k].a ^ g_gather_dedup[k].b) * 2654435761u;
        h &= (uint32_t)(new_cap - 1);
        for (int p = 0; p < new_cap; p++) {
            uint32_t slot = (h + (uint32_t)p) & (uint32_t)(new_cap - 1);
            if (new_dedup[slot].a == 0 && new_dedup[slot].b == 0) {
                new_dedup[slot] = g_gather_dedup[k];
                break;
            }
        }
    }
    free(g_gather_dedup);
    g_gather_dedup = new_dedup;
    g_gather_dedup_cap = new_cap;
}

int32_t bridge_gather_live_contacts(
    BridgeContactEvent* out_contacts, int32_t max_contacts)
{
    BRIDGE_OP("gather_live_contacts", 0, -1);
    if (!g.world_exists) return 0;

    // Iterate all bodies, call b2Body_GetContactData for each, and dedup
    // the (A, B) and (B, A) reports that Box2D naturally produces from both
    // sides of each contact. Normalization picks the lex-smaller pair as
    // the canonical form.

    gather_dedup_reset();
    int dedup_count = 0;
    int count = 0;

    for (int i = 0; i < g.body_count; i++) {
        BodyEntry* entry = &g.bodies[i];
        if (!entry->occupied) continue;

        int n = b2Body_GetContactData(entry->body_id, g_gather_cd_buf, GATHER_CONTACT_DATA_CAP);
        for (int j = 0; j < n; j++) {
            b2ShapeId sidA = g_gather_cd_buf[j].shapeIdA;
            b2ShapeId sidB = g_gather_cd_buf[j].shapeIdB;
            b2BodyId bodyA = b2Shape_GetBody(sidA);
            b2BodyId bodyB = b2Shape_GetBody(sidB);

            uint32_t eA = unpack_uint32(b2Body_GetUserData(bodyA));
            uint32_t eB = unpack_uint32(b2Body_GetUserData(bodyB));
            int32_t sA = unpack_int32(b2Shape_GetUserData(sidA));
            int32_t sB = unpack_int32(b2Shape_GetUserData(sidB));

            // Normalize so (eA, sA) < (eB, sB) lexicographically
            if (eA > eB || (eA == eB && sA > sB)) {
                uint32_t te = eA; eA = eB; eB = te;
                int32_t ts = sA; sA = sB; sB = ts;
                b2ShapeId tsid = sidA; sidA = sidB; sidB = tsid;
            }

            PairKey pk;
            pk.a = ((uint64_t)eA << 32) | (uint32_t)sA;
            pk.b = ((uint64_t)eB << 32) | (uint32_t)sB;

            // Grow dedup if load factor reached 70%
            if (dedup_count * 10 >= g_gather_dedup_cap * 7) {
                gather_dedup_grow();
            }

            // Probe for existing
            uint32_t h = (uint32_t)(pk.a ^ pk.b) * 2654435761u;
            h &= (uint32_t)(g_gather_dedup_cap - 1);
            bool found = false;
            for (int p = 0; p < g_gather_dedup_cap; p++) {
                uint32_t slot = (h + (uint32_t)p) & (uint32_t)(g_gather_dedup_cap - 1);
                if (g_gather_dedup[slot].a == 0 && g_gather_dedup[slot].b == 0) {
                    // Empty slot: insert and emit
                    g_gather_dedup[slot] = pk;
                    dedup_count++;
                    break;
                }
                if (g_gather_dedup[slot].a == pk.a && g_gather_dedup[slot].b == pk.b) {
                    found = true;
                    break;
                }
            }
            if (found) continue;

            if (count >= max_contacts) return count;

            fill_contact_event(&out_contacts[count], BRIDGE_CONTACT_BEGIN,
                               sidA, sidB, &g_gather_cd_buf[j].manifold);
            count++;
        }
    }

    return count;
}
