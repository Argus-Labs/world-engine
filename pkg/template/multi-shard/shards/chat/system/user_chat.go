package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type UserChatSystem struct{}

func (s *UserChatSystem) Run(w *cardinal.World) {
	for cmd := range w.Commands[command.UserChat]() {
		command := cmd.Payload

		timestamp := time.Now()

		chat := w.Create[ChatRow]()

		id := chat.ID()
		chat.Set(component.UserTag{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
		})
		chat.Set(component.Chat{
			Message:   command.Message,
			Timestamp: timestamp,
		})

		w.Logger().Info().
			Uint32("entity", uint32(id)).
			Msgf("Created chat message %s (id: %s)", command.Message, command.ArgusAuthID)

		w.Broadcast(event.UserChat{
			ArgusAuthID:   command.ArgusAuthID,
			ArgusAuthName: command.ArgusAuthName,
			Message:       command.Message,
			Timestamp:     timestamp,
		})
	}
}
