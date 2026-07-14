package system

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/rotisserie/eris"
)

// ensurePhysicsSingleton creates the plugin singleton entity if none exists. Call from Init
// and PreUpdate reconcile so snapshot restore (which may skip Init) still has persisted
// ActiveContacts storage before the first physics step.
//
// singleton must be a pointer: Exact embeds search, whose fields slice points at Refs inside
// the system's result struct. Passing Exact by value makes Create attach those original Refs
// but return a copy of result whose Refs were never attached (nil world → panic on Set).
func ensurePhysicsSingleton(singleton *physicsSingletonSearch) {
	_, _, err := singleton.Iter().Single()
	if err == nil {
		return
	}
	if errors.Is(err, cardinal.ErrSingleMultipleResult) {
		panic(eris.New("physics2d: more than one physics singleton entity (PhysicsSingletonTag)"))
	}
	if !errors.Is(err, cardinal.ErrSingleNoResult) {
		panic(eris.Wrap(err, "physics2d: singleton.Iter().Single()"))
	}
	singleton.Create()
}
