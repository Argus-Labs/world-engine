package system

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/rotisserie/eris"
)

// ensurePhysicsSingleton creates the plugin singleton entity if none exists. Call from Init
// and PreUpdate reconcile so snapshot restore (which may skip Init) still has persisted
// ActiveContacts storage before the first physics step.
func ensurePhysicsSingleton(singleton *physicsSingletonSearch) {
	_, err := singleton.Iter().Single()
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
