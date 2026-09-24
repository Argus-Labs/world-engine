package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// ChainedCurrentToV1 is a mistake on purpose, and the only thing in this package that is.
//
// It converts the current shape back to the oldest one. Registered beside the two real steps it
// closes a loop: v1 becomes v2, v2 becomes current, current becomes v1 again, and a route that was
// meant to end at a shape the build declares never ends at all.
//
// It is here so the engine's refusal has something real to refuse. Nothing else registers it.
type ChainedCurrentToV1 struct {
	Old ecs.In[component.Chained]
	New ecs.Out[component.ChainedV1]
}

func (m *ChainedCurrentToV1) Migrate() {
	m.New.Set(component.ChainedV1{A: m.Old.Get().C})
}
