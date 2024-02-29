package system

import (
	"fmt"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"

	comp "github.com/argus-labs/starter-game-template/cardinal/component"
	"github.com/argus-labs/starter-game-template/cardinal/msg"
)

const AttackDamage = 10

// AttackSystem inflict damage to player's HP based on `AttackPlayer` transactions.
// This provides an example of a system that modifies the component of an entity.
func AttackSystem(world cardinal.WorldContext) error {
	return cardinal.EachMessage[msg.AttackPlayerMsg, msg.AttackPlayerMsgReply](
		world,
		func(attack message.TxData[msg.AttackPlayerMsg]) (msg.AttackPlayerMsgReply, error) {
			playerID, playerHealth, err := queryTargetPlayer(world, attack.Msg.TargetNickname)
			if err != nil {
				return msg.AttackPlayerMsgReply{}, fmt.Errorf("failed to inflict damage: %w", err)
			}

			playerHealth.HP -= AttackDamage
			if err := cardinal.SetComponent[comp.Health](world, playerID, playerHealth); err != nil {
				return msg.AttackPlayerMsgReply{}, fmt.Errorf("failed to inflict damage: %w", err)
			}

			return msg.AttackPlayerMsgReply{Damage: AttackDamage}, nil

		})
}
