package system

import (
	"fmt"
	comp "github.com/argus-labs/starter-game-template/cardinal/component"
	"pkg.world.dev/world-engine/cardinal"
)

// queryTargetPlayer queries for the target player's entity ID and health component.
func queryTargetPlayer(world cardinal.WorldContext, targetNickname string) (cardinal.EntityID, *comp.Health, error) {
	search := world.NewSearch(cardinal.Exact(comp.Player{}, comp.Health{}))

	var playerID cardinal.EntityID
	var playerHealth *comp.Health
	err := search.Each(world, func(id cardinal.EntityID) bool {
		player, err := cardinal.GetComponent[comp.Player](world, id)
		if err != nil {
			return false
		}

		// Terminates the search if the player is found
		if player.Nickname == targetNickname {
			playerID = id
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
		return 0, nil, err
	}

	if playerHealth == nil {
		return 0, nil, fmt.Errorf("player %q does not exist", targetNickname)
	}

	return playerID, playerHealth, err
}
