// Package migration holds the conversions the inventory plugin declares for values its older
// releases wrote. It is a test fixture and not part of the engine.
//
// A plugin owns its components outright, so it owns their history too: these files live beside the
// plugin's components rather than in the shard that installs it. Nothing here differs from a
// shard's own migrations — same fields, same registration, same registry — which is the point.
package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/plugin/inventory/component"
)

// ProvidedV1ToV2 converts provided from the plugin's first release to its second.
//
// Its output is not the current shape. Nothing in the world ever holds a ProvidedV2, because
// ProvidedV2ToCurrent declares it as an input and the engine hands the value straight over. A save
// from the first release therefore reaches the current shape in two steps, without an entity ever
// carrying the shape in between.
type ProvidedV1ToV2 struct {
	Old ecs.In[component.ProvidedV1]
	New ecs.Out[component.ProvidedV2]

	// Calls counts entities migrated, so a test can tell a step that ran from one that was skipped.
	// A real migration holds no state.
	Calls int
}

func (m *ProvidedV1ToV2) Migrate() {
	m.New.Set(component.ProvidedV2{B: m.Old.Get().A + 1})
	m.Calls++
}
