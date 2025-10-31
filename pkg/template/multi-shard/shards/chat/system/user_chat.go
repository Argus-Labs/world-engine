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
	ChatSearch       ChatSearch
}

func UserChatSystem(state *UserChatSystemState) error {
	for cmd := range state.UserChatCommands.Iter() {
		command := cmd.Payload()

		timestamp := time.Now()

		id, chat := state.ChatSearch.Create()
		chat.UserTag.Set(component.UserTag{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
		})
		chat.Chat.Set(component.Chat{
			Message:   command.Message,
			Timestamp: timestamp,
		})

		state.Logger().Info().
			Uint32("entity", uint32(id)).
			Msgf("Created chat message %s (id: %s)", command.Message, command.ArgusAuthID)

		state.UserChatEvent.Emit(event.UserChat{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			Message:       command.Message,
			Timestamp:     timestamp,
		})
	}
	return nil
}
