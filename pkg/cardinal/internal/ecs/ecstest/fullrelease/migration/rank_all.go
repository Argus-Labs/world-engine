package migration

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// RankAll is case 3a: it fills in each entity's rank, which is a position among all the others.
//
// All gives every entity holding the component, ordered by entity ID, and Put writes back to any
// of them. Reading and writing the same component is not a dependency on itself, so this does not
// deadlock the order; it simply runs after whatever produced the component.
//
// It runs once, after every entity has been restored, and only when something in this restore
// actually produced a ranked component. An ordinary restart converts nothing, so nothing here runs.
type RankAll struct {
	Everyone ecs.All[component.Ranked]
	Write    ecs.Put[component.Ranked]

	// Calls counts how many times the whole-world pass ran this, which must be once per restore
	// that needs it and never on a restart.
	Calls int
}

func (m *RankAll) Migrate() {
	m.Calls++

	type scored struct {
		id    ecs.EntityID
		value component.Ranked
	}

	ordered := make([]scored, 0, m.Everyone.Len())
	for id, value := range m.Everyone.Iter() {
		ordered = append(ordered, scored{id: id, value: value})
	}

	// Highest score first. Ties fall back to entity ID, which All already ordered by, so two
	// shards restoring the same save produce the same ranks.
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].value.Score != ordered[j].value.Score {
			return ordered[i].value.Score > ordered[j].value.Score
		}
		return ordered[i].id < ordered[j].id
	})

	for position, entry := range ordered {
		entry.value.Rank = uint32(position + 1)
		m.Write.Set(entry.id, entry.value)
	}
}
