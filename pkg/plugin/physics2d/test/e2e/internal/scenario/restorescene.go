package scenario

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// RestoreScene is the world used by the crash-restore check. It is a scene, not
// a set of assertions: the check builds it in two worlds, snapshots one, restores
// it into the other, and compares.
//
// Every body here carries at least one value that differs from the default it
// would fall back to. That is the whole design. A field that is dropped by
// MarshalWire, or defaulted by UnmarshalJSON when it should not be, or lost in
// the ECS -> Box2D rebuild, shows up as that field reverting — which the
// comparison names exactly.
//
// So: no body has Active, Awake and SleepingAllowed all true; no body is left at
// GravityScale 1 alone; filters use high bits and non-zero group indices;
// materials avoid the defaults; and every shape type is present, because shape
// geometry travels as JSON too.
func RestoreScene() harness.Scenario {
	return harness.Scenario{
		Name:  "restore",
		Setup: buildRestoreScene,
	}
}

func buildRestoreScene(c *harness.Ctx) {
	// Layout rule for this scene: every dynamic body either has a floor under it
	// or has gravity switched off, and no two bodies share a column. A body that
	// falls out of the world, or lands on another body by accident, would make
	// any divergence after the restore impossible to attribute.

	// --- Live contacts: a settled stack, so ActiveContacts has something in it.
	c.Spawn("floor", 0, groundY,
		body(physics.BodyTypeStatic, withFriction(box(20, 1), 0.55)))
	for i := range 3 {
		c.Spawn(stackLabel(i), 0, 0.5+float64(i), body(physics.BodyTypeDynamic,
			withRestitution(withFriction(box(0.5, 0.5), 0.44), 0.11)))
	}

	// --- Body flags, one deviation each. The ones not testing gravity have it
	// switched off so they stay on their row for the whole run.
	hover := func(pb physics.PhysicsBody2D) physics.PhysicsBody2D {
		pb.GravityScale = 0
		return pb
	}

	inactive := body(physics.BodyTypeDynamic, circle(0.5))
	inactive.Active = false
	c.Spawn("flag-inactive", -30, 20, inactive)

	// Spawned asleep with gravity on, and nothing above it: it must still be
	// hanging in mid-air when the snapshot is taken.
	asleep := body(physics.BodyTypeDynamic, circle(0.5))
	asleep.Awake = false
	c.Spawn("flag-asleep", -26, 20, asleep)

	noSleep := hover(body(physics.BodyTypeDynamic, circle(0.5)))
	noSleep.SleepingAllowed = false
	c.Spawn("flag-never-sleeps", -22, 20, noSleep)

	bullet := hover(body(physics.BodyTypeDynamic, circle(0.25)))
	bullet.Bullet = true
	c.Spawn("flag-bullet", -18, 20, bullet)

	locked := hover(body(physics.BodyTypeDynamic, box(0.5, 0.5)))
	locked.FixedRotation = true
	c.SpawnSpinning("flag-fixed-rotation", -14, 20, 3, locked)

	// --- Gravity scales, on their own floor.
	c.Spawn("gravity-floor", -8, 30, body(physics.BodyTypeStatic, box(10, 1)))

	heavyG := body(physics.BodyTypeDynamic, circle(0.5))
	heavyG.GravityScale = 2.5
	c.Spawn("flag-gravity-2.5", -10, 40, heavyG)

	// Rises for the whole run with nothing above it.
	upwards := body(physics.BodyTypeDynamic, circle(0.5))
	upwards.GravityScale = -0.75
	c.Spawn("flag-gravity-negative", -6, 45, upwards)

	// Damping brings this one to a halt well before it leaves its row.
	damped := hover(body(physics.BodyTypeDynamic, circle(0.5)))
	damped.LinearDamping = 0.35
	damped.AngularDamping = 0.65
	c.SpawnMoving("flag-damped", -2, 45, 4, 0, damped)

	// --- Body types, on their own floor. Manual and kinematic have different
	// writeback rules, so both have to come back as the right kind or their
	// poses drift apart.
	c.Spawn("kind-floor", 16, 50, body(physics.BodyTypeStatic, box(14, 1)))
	c.Spawn("kind-static", 10, 60, body(physics.BodyTypeStatic, box(1, 1)))
	c.Spawn("kind-dynamic", 14, 60, body(physics.BodyTypeDynamic, box(0.5, 0.5)))
	c.SpawnMoving("kind-kinematic", 18, 65, 0.4, 0,
		body(physics.BodyTypeKinematic, box(0.5, 0.5)))
	// Manual: ECS owns the pose and nothing drives it, so it must stay put and
	// keep the velocity below, which Box2D never sees.
	c.SpawnMoving("kind-manual", 24, 65, 7, -3, body(physics.BodyTypeManual, box(0.5, 0.5)))

	// --- Every shape type, because geometry is serialized too. The four that
	// carry mass get a floor; chains, loops and edges are static by nature.
	c.Spawn("shape-floor", -24, 70, body(physics.BodyTypeStatic, box(8, 1)))
	c.Spawn("shape-circle", -30, 80, body(physics.BodyTypeDynamic, circle(0.65)))
	c.Spawn("shape-box", -26, 80, body(physics.BodyTypeDynamic, box(0.4, 0.7)))
	c.Spawn("shape-polygon", -22, 80, body(physics.BodyTypeDynamic,
		polygon(vec(-0.6, -0.4), vec(0.6, -0.4), vec(0.3, 0.5), vec(-0.3, 0.5))))
	c.Spawn("shape-capsule", -18, 80, body(physics.BodyTypeDynamic,
		capsule(vec(-0.45, 0), vec(0.45, 0), 0.28)))
	c.Spawn("shape-chain", -10, 80, body(physics.BodyTypeStatic,
		chain(vec(3, 0), vec(1, 0), vec(-1, 0), vec(-3, 0))))
	c.Spawn("shape-chain-loop", 0, 80, body(physics.BodyTypeStatic,
		chainLoop(vec(-2, -2), vec(-2, 2), vec(2, 2), vec(2, -2))))
	c.Spawn("shape-edge", 8, 80, body(physics.BodyTypeStatic,
		edge(vec(-2.5, 0), vec(2.5, 0))))

	// --- A compound body: offsets, a local rotation, per-child materials and
	// per-child filters, all of which have to survive as a group and in order.
	c.Spawn("compound-floor", 20, 90, body(physics.BodyTypeStatic, box(6, 1)))
	c.Spawn("compound", 20, 100, body(physics.BodyTypeDynamic,
		withDensity(atOffset(box(0.4, 0.4), -1.2, 0), 3),
		rotatedBy(atOffset(box(0.9, 0.15), 0, 0.6), math.Pi/6),
		withFilter(asSensor(atOffset(circle(1.1), 1.2, 0)), 1<<41, 1<<41, 0),
	))

	// --- Filters worth losing: bits above 32, and both signs of group index.
	c.Spawn("filter-high-bits", -30, 120, body(physics.BodyTypeStatic,
		withFilter(box(1, 1), 1<<40, (1<<40)|(1<<55), 0)))
	c.Spawn("filter-negative-group", -24, 120, body(physics.BodyTypeStatic,
		withFilter(box(1, 1), 0x20, 0xFF, -9)))
	c.Spawn("filter-positive-group", -18, 120, body(physics.BodyTypeStatic,
		withFilter(box(1, 1), 0x40, 0xFF, 12)))

	// --- A live sensor overlap, so the restored ActiveContacts baseline has a
	// trigger pair in it as well as a contact pair.
	c.Spawn("sensor-gate", 0, 140, body(physics.BodyTypeStatic, asSensor(box(3, 3))))
	visitor := hover(body(physics.BodyTypeDynamic, circle(0.5)))
	visitor.SleepingAllowed = false
	c.Spawn("sensor-visitor", 0, 140, visitor)

	// --- The awake-drift pair.
	//
	// PhysicsBody2D.Awake is a command the reconciler pushes into Box2D; writeback
	// never mirrors Box2D's real awake state back into the component. So a body
	// spawned asleep and later woken by something landing on it keeps Awake=false
	// in ECS forever while it is wide awake and moving in the simulation, and any
	// rebuild recreates it from that stale field.
	//
	// The waker is very bouncy so it rebounds clear after the hit, leaving the
	// sleeper falling on its own. It has to still be falling when the snapshot is
	// taken: a body that has come to rest would be frozen in the right place
	// anyway and the restore would look fine.
	c.Spawn("awake-drift-floor", 40, -80, body(physics.BodyTypeStatic, box(6, 1)))
	dormant := body(physics.BodyTypeDynamic, box(0.6, 0.6))
	dormant.Awake = false
	c.Spawn("awake-drift-sleeper", 40, 200, dormant)
	c.Spawn("awake-drift-waker", 40, 212,
		body(physics.BodyTypeDynamic, withRestitution(box(0.6, 0.6), 0.95)))
}

func stackLabel(i int) string {
	return [...]string{"stack-0", "stack-1", "stack-2"}[i]
}
