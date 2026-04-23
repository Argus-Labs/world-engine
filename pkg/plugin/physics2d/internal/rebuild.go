package internal

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/cbridge"
)

// PhysicsRebuildEntry is one entity's authoritative physics components as read from ECS.
type PhysicsRebuildEntry struct {
	EntityID    cardinal.EntityID
	Transform   component.Transform2D
	Velocity    component.Velocity2D
	PhysicsBody component.PhysicsBody2D
}

// FullRebuildFromECS replaces all derived physics state for the package Runtime in one
// deterministic pass: destroys every body in the C-side world, clears maps and contact buffer,
// optionally applies gravity, recreates bodies/shapes from entries, then writes shadow
// snapshots. Entries are sorted by EntityID before processing; creation order follows that sort.
//
// Requires ResetRuntime (or prior init) so Runtime() is non-nil. World is created on first
// rebuild using gravity; later rebuilds reuse the world and call SetGravity.
//
// On any creation error, bodies created in this pass are destroyed and runtime maps are left
// empty (same as post-clear); the world remains allocated with no bodies.
//
// After a successful rebuild, Emitter is cleared and SuppressContactsStep is set true for the
// next simulation step. The step driver must call SetStepEmitter before the step and
// FlushBufferedContacts after; that flush clears SuppressContactsStep automatically.
func FullRebuildFromECS(gravity component.Vec2, entries []PhysicsRebuildEntry) error {
	rt := Runtime()

	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(a, b PhysicsRebuildEntry) int {
		return cmp.Compare(a.EntityID, b.EntityID)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].EntityID == sorted[i-1].EntityID {
			return fmt.Errorf("physics2d: duplicate entity_id %d in rebuild entries", sorted[i].EntityID)
		}
	}

	// Destroy all existing bodies on the C side.
	cbridge.DestroyAllBodies()
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

	if !cbridge.WorldExists() {
		cbridge.CreateWorld(gravity.X, gravity.Y)
	} else {
		cbridge.SetGravity(gravity.X, gravity.Y)
	}

	newKnown := make(map[cardinal.EntityID]struct{}, len(sorted))
	newShadow := make(map[cardinal.EntityID]ShadowState, len(sorted))

	for _, e := range sorted {
		if err := CreateBodyWithCollider(
			e.EntityID,
			e.Transform,
			e.Velocity,
			e.PhysicsBody,
		); err != nil {
			// On error: destroy all bodies created so far and leave clean state.
			for id := range newKnown {
				cbridge.DestroyBody(uint32(id))
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
