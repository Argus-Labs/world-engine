// Package e2e_test — filter_change_corruption_test.go pins the contact-end event
// ordering bug that arises when the reconciler applies a contact-destroying mutation
// (SetShapeFilter, SetBodyType, DisableBody) while two bodies remain touching.
//
// Without the fix, the Box2D-internal End written by destroyContact during
// ReconcileFromECS survives the IsShapeValid filter in bufferContactEventsFromWorld
// (the shapes are not destroyed by these mutable mutations, only their broad-phase
// proxy/contacts), so it lands AFTER the Begin event that the next Step emits when
// the pair re-pairs. The buffer becomes [Begin, End] and FlushBufferedContacts —
// which mutates ActiveContacts and emits the game-facing event in the same iteration —
// leaves the consumer latched "not touching" while Box2D still reports the pair as
// touching, and the persisted ActiveContacts component loses the pair.
//
// A structural rebuild (geometry change) is the reference: the existing
// PruneActiveContactsInvolvingEntity + IsShapeValid mechanism already produces
// [End_synthesized, Begin] for it, so the pair survives.
package e2e_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
	"github.com/stretchr/testify/require"
)

// allBits is the "no filtering" mask: every category bit is set, so any category is accepted.
const allBits = ^uint64(0)

// floorShape and boxShape build the two collider shapes used by every subtest. Both
// share an initial filter that allows collision; each subtest rewrites exactly one
// shape's filter (or the floor's BodyType, or one of the box's geometry fields) on
// tick 100, choosing the new value so the pair still collides post-change.
func floorShape(hw, hh float64) physics.ColliderShape {
	return physics.ColliderShape{
		ShapeType:    physics.ShapeTypeBox,
		HalfExtents:  physics.Vec2{X: hw, Y: hh},
		Density:      1,
		Friction:     0.6,
		Restitution:  0,
		CategoryBits: 0x1,
		MaskBits:     allBits,
		GroupIndex:   0,
	}
}

func boxShape(hw, hh float64) physics.ColliderShape {
	return physics.ColliderShape{
		ShapeType:    physics.ShapeTypeBox,
		HalfExtents:  physics.Vec2{X: hw, Y: hh},
		Density:      1,
		Friction:     0.6,
		Restitution:  0,
		CategoryBits: 0x1,
		MaskBits:     allBits,
		GroupIndex:   0,
	}
}

// TestFilterChangeCorruptsActiveContacts pins the End-led buffer ordering across every
// contact-destroying reconcile mutation. Each subtest spawns a dynamic box resting on a
// static floor, lets the pair settle, applies a single mutation (one filter field on one
// body, a BodyType change on the floor, or a geometry change for the structural reference),
// steps for a few more ticks, then asserts the pair is still in ActiveContacts (read via
// the harness PostCapture) plus that Begin/End counts stayed balanced.
//
// Pre-fix, every subtest except the structural-rebuild reference fails on the
// ActiveContacts-pair-present check.
func TestFilterChangeCorruptsActiveContacts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// mutate rewrites one tick's ECS state. The harness applies it next PreUpdate,
		// before the physics pipeline.
		mutate func(c *harness.Ctx, boxID, floorID cardinal.EntityID)
		// structural reports whether the mutation routes through the structural-rebuild
		// path (true) or the mutable path (false). The structural reference is the
		// already-working public path.
		structural bool
	}{
		// --- Filter changes on the dynamic box (single field each) ---
		{
			name: "filter/dynamic-CategoryBits",
			mutate: func(c *harness.Ctx, boxID, _ cardinal.EntityID) {
				c.EditShape(boxID, 0, func(sh *physics.ColliderShape) { sh.CategoryBits = 0x2 })
			},
		},
		{
			name: "filter/dynamic-MaskBits",
			mutate: func(c *harness.Ctx, boxID, _ cardinal.EntityID) {
				// Still names floor's category 0x1 so the pair keeps colliding.
				c.EditShape(boxID, 0, func(sh *physics.ColliderShape) {
					sh.MaskBits = 0x3
				})
			},
		},
		{
			name: "filter/dynamic-GroupIndex",
			mutate: func(c *harness.Ctx, boxID, _ cardinal.EntityID) {
				// Unequal group indices are ignored, so the masks still decide: collide.
				c.EditShape(boxID, 0, func(sh *physics.ColliderShape) { sh.GroupIndex = 1 })
			},
		},

		// --- Filter changes on the static floor (single field each) ---
		{
			name: "filter/static-CategoryBits",
			mutate: func(c *harness.Ctx, _, floorID cardinal.EntityID) {
				c.EditShape(floorID, 0, func(sh *physics.ColliderShape) { sh.CategoryBits = 0x2 })
			},
		},
		{
			name: "filter/static-MaskBits",
			mutate: func(c *harness.Ctx, _, floorID cardinal.EntityID) {
				// Still names box's category 0x1 so the pair keeps colliding.
				c.EditShape(floorID, 0, func(sh *physics.ColliderShape) {
					sh.MaskBits = 0x3
				})
			},
		},
		{
			name: "filter/static-GroupIndex",
			mutate: func(c *harness.Ctx, _, floorID cardinal.EntityID) {
				c.EditShape(floorID, 0, func(sh *physics.ColliderShape) { sh.GroupIndex = 1 })
			},
		},

		// --- BodyType change on the static floor (Static -> Kinematic) ---
		// Box stays Dynamic so shouldBodiesCollide returns true and the pair is recreated.
		{
			name: "body-type-change/floor-static-to-kinematic",
			mutate: func(c *harness.Ctx, _, floorID cardinal.EntityID) {
				c.EditBody(floorID, func(pb *physics.PhysicsBody2D) {
					pb.BodyType = physics.BodyTypeKinematic
				})
			},
		},

		// --- Structural-rebuild reference: a geometry change routes through
		// destroyAllShapesForEntity + AttachColliderFixtures + Prune, which already
		// orders the buffer correctly via PruneActiveContactsInvolvingEntity and
		// IsShapeValid. Pins that the discriminating invariant is the pair membership,
		// not the spurious event count (both paths emit "1 extra Begin, 1 extra End").
		{
			name: "structural-rebuild-reference/box-half-extents-x",
			mutate: func(c *harness.Ctx, boxID, _ cardinal.EntityID) {
				c.EditShape(boxID, 0, func(sh *physics.ColliderShape) {
					sh.HalfExtents = physics.Vec2{X: 0.6, Y: 0.5}
				})
			},
			structural: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				boxID, floorID cardinal.EntityID
				settledBegins  int
			)

			// dropY leaves a small gap that closes in the first few ticks so Begin fires
			// while the box is in motion (a suppressed build-time overlap would not).
			const (
				groundY    = -1.0 // matches floor HalfExtents.Y so its top sits at y=0
				dropY      = 1.2
				mutateTick = 100
				checkTick  = 105
			)

			scn := harness.Scenario{
				Name: tc.name,
				Setup: func(c *harness.Ctx) {
					floorID = c.Spawn("floor", 0, groundY,
						physcomp.NewPhysicsBody2D(physics.BodyTypeStatic, floorShape(20, 1)))
					boxID = c.Spawn("box", 0, dropY,
						physcomp.NewPhysicsBody2D(physics.BodyTypeDynamic, boxShape(0.5, 0.5)))
				},
				Steps: []harness.Step{
					// Settle: by mutateTick-1 the box has landed and the pair sits in
					// ActiveContacts with exactly one ContactBegin logged.
					{Tick: mutateTick - 1, Do: func(c *harness.Ctx) {
						settledBegins = c.CountBetween(harness.ContactBegin, boxID, floorID)
						c.IntAtLeast("the box lands on the floor before the mutation",
							settledBegins, 1)
						c.Int("the resting pair raises no ContactEnd before the mutation",
							c.CountBetween(harness.ContactEnd, boxID, floorID), 0)
					}},
					// Apply the mutated ECS state. The harness applies it next PreUpdate,
					// before the physics pipeline.
					{Tick: mutateTick, Do: func(c *harness.Ctx) {
						tc.mutate(c, boxID, floorID)
					}},
					// After enough steps for the recreate Begin to flush and the
					// during-reconcile End to be reordered ahead of it, assert the
					// pair's buffer stayed Begin-led (Begin count went up by one, End
					// count went up by one) and the consumer's latched state is
					// "touching" (no per-tick re-fire while the box rests).
					{Tick: checkTick, Do: func(c *harness.Ctx) {
						c.Int("the mutation flush emits exactly one extra ContactBegin",
							c.CountBetween(harness.ContactBegin, boxID, floorID), settledBegins+1)
						c.Int("the mutation flush emits exactly one extra ContactEnd",
							c.CountBetween(harness.ContactEnd, boxID, floorID), 1)
						c.Note("change flush produced 1 extra Begin, 1 extra End (same counts as the structural path)")
					}},
				},
			}

			var postCapture harness.Capture
			cfg := e2eConfig(t, 0)
			cfg.PostCapture = &postCapture
			runner, _, code := runSuite(t, []harness.Scenario{scn}, cfg)

			// Pair membership in the persisted ActiveContacts component is the
			// consumer-latched-state proxy: the flush mutates the map and emits the
			// game-facing event in the same iteration, so a surviving pair means the
			// last event the consumer saw was ContactBeginEvent ("touching"), and a
			// missing pair means the last event was ContactEndEvent ("not touching").
			pairPresent := false
			for _, p := range postCapture.Contacts {
				if (p.EntityA == boxID && p.EntityB == floorID) ||
					(p.EntityA == floorID && p.EntityB == boxID) {
					pairPresent = true
					break
				}
			}
			require.True(t, pairPresent,
				"%s: the box/floor pair must remain in ActiveContacts while Box2D reports it "+
					"as touching; a Begin-led mutation flush would have left the pair in the map, "+
					"an End-led one dropped it", tc.name)

			pass, fail, _ := runner.Report().Totals()
			t.Logf("%s: %d pass, %d fail, ActiveContacts pair present=%v (structural=%v)",
				tc.name, pass, fail, pairPresent, tc.structural)
			require.Zero(t, code, "%d in-scenario check(s) failed", fail)
		})
	}
}
