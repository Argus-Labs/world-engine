package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// SpawnerV1ToCurrent carries the count across. How many entities were actually made is filled in
// by [SpawnMinions], because making them is not something an entity can do to itself.
type SpawnerV1ToCurrent struct {
	Old ecs.In[component.SpawnerV1]
	New ecs.Out[component.Spawner]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *SpawnerV1ToCurrent) Migrate() {
	m.New.Set(component.Spawner{Count: m.Old.Get().Count})
	m.Calls++
}
