package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/plugin/inventory/component"
)

// ProvidedV2ToCurrent converts provided from the plugin's second release to its third.
//
// Its input is a shape no save in the fixture holds. It runs because ProvidedV1ToV2 produced one,
// which is the link the engine derives from the declarations alone — neither migration names the
// other, and the plugin registers them in the wrong order on purpose.
type ProvidedV2ToCurrent struct {
	Old ecs.In[component.ProvidedV2]
	New ecs.Out[component.Provided]

	// Calls counts entities migrated. See ProvidedV1ToV2.
	Calls int
}

func (m *ProvidedV2ToCurrent) Migrate() {
	m.New.Set(component.Provided{C: m.Old.Get().B * 3})
	m.Calls++
}
