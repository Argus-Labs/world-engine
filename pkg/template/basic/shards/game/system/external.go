package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// ExternalCommand should originate from another game shard.
type ExternalCommand struct {
	Message string
}

func (ExternalCommand) Name() string {
	return "external"
}

type CallExternalCommand struct {
	Message string
}

func (CallExternalCommand) Name() string {
	return "call-external"
}

type CallExternalSystem struct {
	MatchmakingWorld cardinal.OtherWorld
}

func (s *CallExternalSystem) Run(w *cardinal.World) {
	for cmd := range w.Commands[CallExternalCommand]() {
		w.Logger().Info().Msg("Received call-external message")

		w.SendToShard(s.MatchmakingWorld, CreatePlayerCommand{
			Nickname: cmd.Payload.Message,
		})
	}
}
