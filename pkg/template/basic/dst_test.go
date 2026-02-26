package basic_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system"
)

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		cardinal.RegisterSystem(w, system.PlayerSpawnerSystem, cardinal.WithHook(cardinal.Init))
		cardinal.RegisterSystem(w, system.CreatePlayerSystem)
		cardinal.RegisterSystem(w, system.RegenSystem)
		cardinal.RegisterSystem(w, system.AttackPlayerSystem)
		cardinal.RegisterSystem(w, system.GraveyardSystem)
		cardinal.RegisterSystem(w, system.CallExternalSystem)
	})
}
