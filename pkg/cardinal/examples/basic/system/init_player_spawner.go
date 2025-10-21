package system

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/basic/component"
)

type PlayerSpawnerSystemState struct {
	cardinal.BaseSystemState
	Players PlayerSearch
}

func PlayerSpawnerSystem(state *PlayerSpawnerSystemState) error {
	for i := range 10 {
		name := fmt.Sprintf("default-%d", i)

		id, err := state.Players.Create(
			component.PlayerTag{Nickname: name},
			component.Health{HP: 100},
		)
		if err != nil {
			return err
		}

		state.Logger().Info().Uint32("entity", uint32(id)).Msgf("Created player %s", name)
	}
	return nil
}
