package system

import (
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type RegenSystem struct{}

func (s *RegenSystem) Run(w *cardinal.World) {
	// Contains matches every entity with Health, whatever else it carries.
	for health := range w.Contains[struct{ component.Health }]().Iter() {
		health.Set(component.Health{HP: health.Get[component.Health]().HP + 10})
	}
}
