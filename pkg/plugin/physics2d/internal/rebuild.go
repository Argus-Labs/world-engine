package internal

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// PhysicsRebuildEntry is one entity's authoritative physics components as read from ECS.
type PhysicsRebuildEntry struct {
	EntityID    cardinal.EntityID
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// FullRebuildFromECS replaces all derived physics state on this runtime in one
// deterministic pass: destroys every body in the Box2D world, clears maps and contact buffer,
// optionally applies gravity, recreates bodies/shapes from entries, then writes shadow
// snapshots. Entries are sorted by EntityID before processing; creation order follows that sort.
//
// World is created on first rebuild using gravity; later rebuilds reuse the world and call
// SetGravity.
//
// On any creation error, bodies created in this pass are destroyed and runtime maps are left
// empty (same as post-clear); the world remains allocated with no bodies.
//
// After a successful rebuild, Emitter is cleared and SuppressContactsStep is set true for the
// next simulation step. The step driver must call SetStepEmitter before the step and
// FlushBufferedContacts after; that flush clears SuppressContactsStep automatically.
//
// Bodies keep their component Awake value; those in the persisted ActiveContacts baseline are
// woken before the suppressed step instead (see Runtime.wakePersistedContactEntities).
func (rt *Runtime) FullRebuildFromECS(gravity component.Vec2, entries []PhysicsRebuildEntry) error {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b PhysicsRebuildEntry) int {
		return cmp.Compare(a.EntityID, b.EntityID)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].EntityID == sorted[i-1].EntityID {
			return fmt.Errorf("physics2d: duplicate entity_id %d in rebuild entries", sorted[i].EntityID)
		}
	}

	// Destroy all existing bodies (sorted for deterministic destruction order).
	if rt.World != nil {
		ids := make([]cardinal.EntityID, 0, len(rt.Bodies))
		for id := range rt.Bodies {
			ids = append(ids, id)
		}
		slices.SortFunc(ids, cmp.Compare)
		for _, id := range ids {
			rt.DestroyEntityBody(id)
		}
	}
	clear(rt.Bodies)
	clear(rt.Shapes)
	clear(rt.Chains)
	clear(rt.KnownEntities)
	clear(rt.Shadow)
	rt.BufferedContacts = rt.BufferedContacts[:0]
	// Force reload of active-contact baseline from the ECS singleton on the next step. If we
	// kept the in-memory map, the post-rebuild suppressed diff would compare against stale
	// runtime state instead of the persisted component.
	rt.ActiveContacts = nil
	rt.ActiveContactsDirty = false
	rt.NoPersistedActiveContactsBaseline = false
	// Step driver must set Emitter again before the step if physics should emit system events.
	rt.Emitter = nil
	// First step after rebuild: skip contact begin/end (Box2D would otherwise fire for all overlaps).
	rt.SuppressContactsStep = true

	if rt.World == nil {
		def := box2d.DefaultWorldDef()
		def.Gravity = box2d.Vec2{X: gravity.X, Y: gravity.Y}
		// Deterministic across worker counts: byte-identical results for every
		// value, so worlds rebuilt under different Workers settings replay
		// identically (see pkg/box2d/worker_pool.go).
		def.WorkerCount = rt.Workers
		rt.World = box2d.NewWorld(&def)
	} else {
		rt.World.SetGravity(box2d.Vec2{X: gravity.X, Y: gravity.Y})
	}

	newKnown := make(map[cardinal.EntityID]struct{}, len(sorted))
	newShadow := make(map[cardinal.EntityID]ShadowState, len(sorted))

	for _, e := range sorted {
		if err := rt.CreateBodyWithCollider(
			e.EntityID,
			e.Transform,
			e.Velocity,
			e.PhysicsBody,
		); err != nil {
			// On error: destroy all bodies created so far and leave clean state.
			for id := range newKnown {
				rt.DestroyEntityBody(id)
			}
			clear(newKnown)
			clear(newShadow)
			return fmt.Errorf("physics2d: entity %d: %w", e.EntityID, err)
		}
		newKnown[e.EntityID] = struct{}{}
		newShadow[e.EntityID] = NewShadowState(
			e.Transform,
			e.Velocity,
			e.PhysicsBody,
		)
	}

	rt.KnownEntities = newKnown
	rt.Shadow = newShadow
	return nil
}
