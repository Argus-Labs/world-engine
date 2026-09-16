package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type UserChatSystemState struct {
	cardinal.BaseSystemState
	UserChatCommands cardinal.WithCommand[command.UserChat]
	UserChatEvent    cardinal.WithEvent[event.UserChat]
}

func UserChatSystem(state *UserChatSystemState) {
	for cmd := range state.UserChatCommands.Iter() {
		command := cmd.Payload

		timestamp := time.Now()

		chat := state.Create[ChatRow]()

		id := chat.ID()
		chat.Set(component.UserTag{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
		})
		chat.Set(component.Chat{
			Message:   command.Message,
			Timestamp: timestamp,
		})

		state.Logger().Info().
			Uint32("entity", uint32(id)).
			Msgf("Created chat message %s (id: %s)", command.Message, command.ArgusAuthID)

		state.UserChatEvent.Broadcast(event.UserChat{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			Message:       command.Message,
			Timestamp:     timestamp,
		})
	}
}
