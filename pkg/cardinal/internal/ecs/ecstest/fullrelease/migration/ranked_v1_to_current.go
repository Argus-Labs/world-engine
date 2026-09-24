package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// RankedV1ToCurrent carries the score across and leaves the rank at zero.
//
// It is an ordinary per-entity migration and deliberately incomplete: the value it cannot supply
// is filled by [RankAll], which sees every entity. Splitting it this way is what keeps the common
// per-entity path free of whole-world machinery.
type RankedV1ToCurrent struct {
	Old ecs.In[component.RankedV1]
	New ecs.Out[component.Ranked]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *RankedV1ToCurrent) Migrate() {
	m.New.Set(component.Ranked{Score: m.Old.Get().Score})
	m.Calls++
}
