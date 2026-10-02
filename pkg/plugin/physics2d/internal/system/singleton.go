package system

import (
	"errors"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/rotisserie/eris"
)

// ensurePhysicsSingleton creates the plugin singleton entity if none exists and reports
// whether it created it. Call from Init and PreUpdate reconcile so snapshot restore (which
// may skip Init) still has persisted ActiveContacts storage before the first physics step.
//
// A true return means the singleton was absent this call (e.g. a cross-plugin/migration
// restore whose snapshot did not carry the physics singleton). A caller that hits that on
// a suppressed step arms NoPersistedActiveContactsBaseline so the next contact flush adopts
// live contacts silently instead of firing a Begin for every overlap.
func ensurePhysicsSingleton(singleton cardinal.Search) bool {
	_, err := singleton.Iter().Single()
	if err == nil {
		return false
	}
	if errors.Is(err, cardinal.ErrSingleMultipleResult) {
		panic(eris.New("physics2d: more than one physics singleton entity (PhysicsSingletonTag)"))
	}
	if !errors.Is(err, cardinal.ErrSingleNoResult) {
		panic(eris.Wrap(err, "physics2d: singleton.Iter().Single()"))
	}
	singleton.Create()
	return true
}
