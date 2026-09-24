// Package migration holds the conversions the full-release build declares for values older builds
// wrote. It is a test fixture and not part of the engine.
//
// One file per migration, named for the version step it performs: <Component>V<n>To<next>, or
// ToCurrent for the last step. A migration is a version step, and naming it that way says which
// releases it serves, so an author dropping support for old saves knows which files to delete.
// What the migration does is in the two lines below its declaration anyway.
package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// RenamedV1ToCurrent converts renamed from the early-access shape to the current one.
//
// The In and Out fields are the whole declaration: In names the stored shape this consumes, Out the
// component it produces. The engine reads them once at registration and calls Migrate once per
// entity holding a matching value.
//
// Nothing about a rename moves bytes, so this migration exists to state intent, not to transform.
// The engine cannot tell a renamed field from a field that happens to share a number, so it refuses
// to guess and requires the developer to say which it is.
type RenamedV1ToCurrent struct {
	Old ecs.In[component.RenamedV1]
	New ecs.Out[component.Renamed]

	// Calls counts entities migrated. A real migration holds no state — it runs against one entity
	// at a time and the same struct is reused for all of them. The fixture keeps a counter because
	// the converted values alone cannot show whether the migration ran: a rename leaves them
	// identical either way.
	Calls int
}

func (m *RenamedV1ToCurrent) Migrate() {
	m.New.Set(component.Renamed{After: m.Old.Get().Before})
	m.Calls++
}
