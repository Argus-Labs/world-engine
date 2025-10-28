package system

import (
	otherworld "github.com/argus-labs/world-engine/pkg/template/basic/pkg/other_worlds"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

// ExternalCommand should originate from another game shard.
type ExternalCommand struct {
	cardinal.BaseCommand
	Message string
}

func (ExternalCommand) Name() string {
	return "external"
}

type CallExternalCommand struct {
	cardinal.BaseCommand
	Message string
}

func (CallExternalCommand) Name() string {
	return "call-external"
}

type CallExternalSystemState struct {
	cardinal.BaseSystemState
	CallExternalCommands cardinal.WithCommand[CallExternalCommand]
}

func CallExternalSystem(state *CallExternalSystemState) error {
	for cmd := range state.CallExternalCommands.Iter() {
		state.Logger().Info().Msg("Received call-external message")

		otherworld.Matchmaking.Send(&state.BaseSystemState, CreatePlayerCommand{
			Nickname: cmd.Payload().Message,
		})
	}
	return nil
}
