package system

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerSpawnerSystemState struct {
	cardinal.BaseSystemState
}

func PlayerSpawnerSystem(state *PlayerSpawnerSystemState) {
	for i := range 10 {
		name := fmt.Sprintf("default-%d", i)

		entity := state.Create[Player]()
		entity.Set(component.PlayerTag{Nickname: name})
		entity.Set(component.Health{HP: 100})

		state.Logger().Info().Uint32("entity", uint32(entity.ID())).Msgf("Created player %s", name)
	}
}
