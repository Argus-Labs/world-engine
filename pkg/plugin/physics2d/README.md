# physics2d

Box2D v3–backed 2D physics plugin for Cardinal. All simulation state lives
on the C side; Cardinal drives it through three ECS components attached to
the entities you want simulated.

Registration:

```go
world := cardinal.NewWorld(cardinal.WorldOptions{TickRate: 60})
cardinal.RegisterPlugin(world, physics2d.NewPlugin(physics2d.Config{
    Gravity:      physics2d.Vec2{X: 0, Y: -9.8},
    TickRate:     60, // match WorldOptions.TickRate
    SubStepCount: 4,
}))
world.StartGame()
```

## Enrolling an entity into physics

An entity participates in the simulation when it carries **all three** of
these components. Missing any one → the reconciler skips the entity and no
body is created on the C side.

| Component | Purpose |
|---|---|
| [`Transform2D`](component/spatial.go) | World-space position + rotation (authoritative pose) |
| [`Velocity2D`](component/spatial.go) | Linear + angular velocity |
| [`PhysicsBody2D`](component/physics_body.go) | Body kind, damping, flags, and the compound collider (`Shapes`) |

Use the `NewPhysicsBody2D` constructor — bare struct literals leave
`Active`, `Awake`, `SleepingAllowed` at `false` and `GravityScale` at `0`,
which produces an inactive, sleeping, gravity-less body.

### Example: a dynamic circle

```go
import (
    "github.com/argus-labs/world-engine/pkg/cardinal"
    "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

func SpawnBallSystem(ctx cardinal.WorldContext) error {
    id, err := cardinal.Create(ctx,
        physics2d.Transform2D{
            Position: physics2d.Vec2{X: 0, Y: 10},
            Rotation: 0,
        },
        physics2d.Velocity2D{
            Linear:  physics2d.Vec2{X: 0, Y: 0},
            Angular: 0,
        },
        physics2d.NewPhysicsBody2D(
            physics2d.BodyTypeDynamic,
            physics2d.ColliderShape{
                ShapeType:    physics2d.ShapeTypeCircle,
                Radius:       0.5,
                Density:      1.0,
                Friction:     0.3,
                Restitution:  0.2,
                CategoryBits: 0x0001,
                MaskBits:     0xFFFF,
            },
        ),
    )
    _ = id
    return err
}
```

### Example: a static box (world geometry)

```go
cardinal.Create(ctx,
    physics2d.Transform2D{Position: physics2d.Vec2{X: 0, Y: 0}},
    physics2d.Velocity2D{},
    physics2d.NewPhysicsBody2D(
        physics2d.BodyTypeStatic,
        physics2d.ColliderShape{
            ShapeType:    physics2d.ShapeTypeBox,
            HalfExtents:  physics2d.Vec2{X: 25, Y: 1},
            Friction:     0.5,
            CategoryBits: 0x0002,
            MaskBits:     0xFFFF,
        },
    ),
)
```

### Body-type cheat sheet

- **`BodyTypeStatic`** — immovable world geometry. No writeback.
- **`BodyTypeDynamic`** — full simulation: forces, gravity, collisions. Writeback updates `Transform2D`/`Velocity2D` each tick.
- **`BodyTypeKinematic`** — velocity-driven; Box2D integrates position. Writeback on.
- **`BodyTypeManual`** — gameplay owns position/velocity; Box2D is used only for contact detection. No writeback; the reconciler pushes ECS → Box2D each tick. Use for characters/enemies driven by input or AI.

### Compound colliders

`PhysicsBody2D.Shapes` is a slice — each entry is a child fixture with its
own `LocalOffset`, `LocalRotation`, material, and filter. Shape identity is
by index (slot `i` in `Shapes` ↔ fixture slot `i`), so don't reorder shapes
after creation if you care about per-shape references in contact events.

## Built-in queries

Use these first; they cover most needs and don't require CGO:

- `physics2d.Raycast(RaycastRequest) RaycastResult`
- `physics2d.OverlapAABB(AABBOverlapRequest) AABBOverlapResult`
- `physics2d.CircleSweep(CircleSweepRequest) CircleSweepResult`

All three return zero results when no C-side world exists yet (e.g. before
the first reconcile, or right after `ResetRuntime`).

## Custom queries via CGO

If you need a Box2D feature the plugin doesn't expose (joints, shape casts,
custom query filters, sensor-only overlap, etc.), call Box2D directly from
your own CGO package. The plugin exposes the raw world handle via
[`physics2d.WorldID()`](plugin.go), which returns the Box2D v3 `b2WorldId`
packed as a `uint32`. Reconstruct it in C with `b2LoadWorldId`.

### Userdata encoding

The bridge stuffs identity into Box2D userdata pointers (see [bridge.c:214-217](internal/cbridge/bridge.c#L214-L217)):

- **Body userdata** = entity ID, packed as `(void*)(uintptr_t)entity_id` — unpack with `(uint32_t)(uintptr_t)b2Body_GetUserData(bodyId)`.
- **Shape userdata** = shape slot index (the index into `PhysicsBody2D.Shapes`), packed the same way but as `int32_t`.

So inside any Box2D callback you can recover the ECS entity with one line.

### Example: custom AABB overlap that returns every hit, including sensors

```go
package myphysics

/*
#cgo CFLAGS: -I${SRCDIR}/../../vendor/world-engine/pkg/plugin/physics2d/third_party/box2d/include
#include "box2d/box2d.h"
#include <stdint.h>

static bool overlap_cb(b2ShapeId shapeId, void* ctx) {
    // Recover the ECS entity ID from body userdata.
    b2BodyId body = b2Shape_GetBody(shapeId);
    uint32_t* out = (uint32_t*)ctx;
    // ... append unpack_uint32(b2Body_GetUserData(body)) to your buffer ...
    (void)out;
    return true; // keep going
}

static int my_overlap_all(
    uint32_t world_id_packed,
    float minX, float minY, float maxX, float maxY,
    uint32_t* out_entities, int32_t cap
) {
    b2WorldId world = b2LoadWorldId(world_id_packed);
    if (!b2World_IsValid(world)) return 0;

    b2AABB aabb = { {minX, minY}, {maxX, maxY} };
    b2QueryFilter filter = b2DefaultQueryFilter(); // matches everything
    b2World_OverlapAABB(world, aabb, filter, overlap_cb, out_entities);
    // ... return fill count ...
    return 0;
}
*/
import "C"

import "github.com/argus-labs/world-engine/pkg/plugin/physics2d"

func OverlapAll(minX, minY, maxX, maxY float64) []uint32 {
    worldID := physics2d.WorldID()
    if worldID == 0 {
        return nil // no world yet: before init or after ResetRuntime
    }
    buf := make([]uint32, 256)
    n := C.my_overlap_all(
        C.uint32_t(worldID),
        C.float(minX), C.float(minY), C.float(maxX), C.float(maxY),
        (*C.uint32_t)(&buf[0]), C.int32_t(len(buf)),
    )
    return buf[:int(n)]
}
```

### Rules of engagement

- **Always null-check `WorldID()`** — it returns `0` before the first
  `PreUpdate` reconcile and after `ResetRuntime`. Treat `0` as "no world,
  skip the query."
- **Never mutate world state from a system.** `WorldID()` gives you a raw
  Box2D handle; calling `b2Body_SetTransform` / `b2DestroyBody` / etc.
  directly will desync the bridge's entity→body map and the reconciler
  will fight you next tick. For mutations, go through ECS components — the
  reconciler pushes changes to Box2D before each step.
- **Queries are fine.** Raycasts, overlaps, shape casts, sensor iteration,
  contact walks — read-only Box2D calls are safe to make any time after
  you've confirmed `WorldID() != 0`.
- **Don't cache the handle across ticks.** `WorldID()` is cheap; call it
  at the top of each query. After `ResetRuntime` (e.g. snapshot restore)
  the old id is invalid.

## Contact events

Contacts and triggers flow through Cardinal's system-event bus. Register an
emitter with `physics2d.SetStepContactEmitter` and the plugin flushes
`ContactBeginEvent` / `ContactEndEvent` / `TriggerBeginEvent` /
`TriggerEndEvent` each tick. The events carry both entity IDs and both
shape indices, so you can look up the exact `ColliderShape` that produced
the contact.
