package query

import (
	"fmt"

	comp "github.com/argus-labs/starter-game-template/cardinal/component"

	"pkg.world.dev/world-engine/cardinal"
)

type PlayerHealthRequest struct {
	Nickname string
}

type PlayerHealthResponse struct {
	HP int
}

func PlayerHealth(world cardinal.WorldContext, req *PlayerHealthRequest) (*PlayerHealthResponse, error) {
	search, err := world.NewSearch(cardinal.Exact(comp.Player{}, comp.Health{}))
	if err != nil {
		return nil, err
	}

	var playerHealth *comp.Health
	err = search.Each(world, func(id cardinal.EntityID) bool {
		player, err := cardinal.GetComponent[comp.Player](world, id)
		if err != nil {
			return false
		}

		// Terminates the search if the player is found
		if player.Nickname == req.Nickname {
			playerHealth, err = cardinal.GetComponent[comp.Health](world, id)
			if err != nil {
				return false
			}
			return false
		}

		// Continue searching if the player is not the target player
		return true
	})
	if err != nil {
		return nil, err
	}

	if playerHealth == nil {
		return nil, fmt.Errorf("player %s does not exist", req.Nickname)
	}

	return &PlayerHealthResponse{HP: playerHealth.HP}, nil
}
