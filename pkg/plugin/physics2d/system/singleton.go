package system

import physicscomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/component"

// ensurePhysicsSingleton creates the plugin singleton entity if none exists. Call from Init
// and PreUpdate reconcile so snapshot restore (which may skip Init) still has persisted
// ActiveContacts storage before the first physics step.
//
// singleton must be a pointer: Exact embeds search, whose fields slice points at Refs inside
// the system's result struct. Passing Exact by value makes Create attach those original Refs
// but return a copy of result whose Refs were never attached (nil world → panic on Set).
func ensurePhysicsSingleton(singleton *physicsSingletonArchetype) {
	for range singleton.Iter() {
		return
	}
	_, row := singleton.Create()
	row.Tag.Set(physicscomp.PhysicsSingletonTag{})
	row.ActiveContacts.Set(physicscomp.ActiveContacts{})
}
