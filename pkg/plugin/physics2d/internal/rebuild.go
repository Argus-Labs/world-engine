package internal

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/ByteArena/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
)

// PhysicsRebuildEntry is one entity’s authoritative physics components as read from ECS.
type PhysicsRebuildEntry struct {
	EntityID  cardinal.EntityID
	Transform component.Transform2D
	Velocity  component.Velocity2D
	Rigidbody component.Rigidbody2D
	Collider  component.Collider2D
}

// FullRebuildFromECS replaces all derived physics state for the package Runtime in one
// deterministic pass: destroys every body in the Box2D world, clears maps and contact buffer,
// optionally applies gravity, recreates bodies/fixtures from entries, then writes shadow
// snapshots. Entries are sorted by EntityID before processing; creation order follows that sort.
//
// Requires ResetRuntime (or prior init) so Runtime() is non-nil. World is created on first
// rebuild using gravity; later rebuilds reuse the world and call SetGravity.
//
// On any creation error, bodies created in this pass are destroyed and runtime maps are left
// empty (same as post-clear); the world remains allocated with no bodies.
//
// After a successful rebuild, Emitter is cleared and SuppressContactsStep is set true for the
// next simulation step. The step driver must call SetStepEmitter before World.Step and
// FlushBufferedContacts after; that flush clears SuppressContactsStep automatically.
func FullRebuildFromECS(gravity box2d.B2Vec2, entries []PhysicsRebuildEntry) error {
	rt := Runtime()
	if rt == nil {
		return errors.New("physics2d: Runtime is nil; call ResetRuntime first")
	}
	return fullRebuild(rt, gravity, entries)
}

func fullRebuild(rt *PhysicsRuntime, gravity box2d.B2Vec2, entries []PhysicsRebuildEntry) error {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b PhysicsRebuildEntry) int {
		return cmp.Compare(a.EntityID, b.EntityID)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].EntityID == sorted[i-1].EntityID {
			return fmt.Errorf("physics2d: duplicate entity_id %d in rebuild entries", sorted[i].EntityID)
		}
	}

	destroyAllBodiesInWorld(rt.World)
	clear(rt.Bodies)
	clear(rt.Shadow)
	rt.BufferedContacts = rt.BufferedContacts[:0]
	// Force reload of active-contact baseline from the ECS singleton on the next step. If we
	// kept the in-memory map, the post-rebuild suppressed diff would compare Box2D against stale
	// runtime state instead of the persisted component.
	rt.ActiveContacts = nil
	rt.ActiveContactsDirty = false
	rt.NoPersistedActiveContactsBaseline = false
	// Step driver must set Emitter again before World.Step if physics should emit system events.
	rt.Emitter = nil
	// First step after rebuild: skip contact begin/end (Box2D would otherwise fire for all overlaps).
	rt.SuppressContactsStep = true

	if rt.World == nil {
		w := box2d.MakeB2World(gravity)
		rt.World = &w
	} else {
		rt.World.SetGravity(gravity)
	}
	RegisterPhysicsContactListener(rt.World)

	newBodies := make(map[cardinal.EntityID]BodyHandle, len(sorted))
	newShadow := make(map[cardinal.EntityID]ShadowState, len(sorted))

	for _, e := range sorted {
		body, err := CreateBodyWithCollider(
			rt.World,
			e.EntityID,
			e.Transform,
			e.Velocity,
			e.Rigidbody,
			e.Collider,
		)
		if err != nil {
			destroyBodyMap(rt.World, newBodies)
			clear(newBodies)
			clear(newShadow)
			return fmt.Errorf("physics2d: entity %d: %w", e.EntityID, err)
		}
		newBodies[e.EntityID] = body
		newShadow[e.EntityID] = NewShadowState(
			e.Transform,
			e.Velocity,
			e.Rigidbody,
			e.Collider,
		)
	}

	rt.Bodies = newBodies
	rt.Shadow = newShadow
	return nil
}

func destroyAllBodiesInWorld(w *box2d.B2World) {
	if w == nil {
		return
	}
	var bodies []*box2d.B2Body
	for b := w.GetBodyList(); b != nil; b = b.GetNext() {
		bodies = append(bodies, b)
	}
	for _, b := range bodies {
		w.DestroyBody(b)
	}
}

func destroyBodyMap(w *box2d.B2World, bodies map[cardinal.EntityID]BodyHandle) {
	if w == nil {
		return
	}
	for _, b := range bodies {
		if b != nil {
			w.DestroyBody(b)
		}
	}
}
