package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
)

// storedConfig holds the configuration set by the Register function.
var storedConfig component.ConfigComponent

// SetConfig stores the configuration for the init system to use.
// This is called by the Register function before systems are registered.
func SetConfig(config component.ConfigComponent) {
	storedConfig = config
}

// InitSystemState is the state for the init system.
type InitSystemState struct {
	cardinal.BaseSystemState

	// Index entities
	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]
}

// InitSystem creates singleton index entities.
// This runs once during world initialization (Init hook).
func InitSystem(state *InitSystemState) error {
	// Check if lobby index already exists
	hasLobbyIndex := false
	for range state.LobbyIndexes.Iter() {
		hasLobbyIndex = true
		break
	}

	if !hasLobbyIndex {
		// Create lobby index singleton
		_, lobbyIdx := state.LobbyIndexes.Create()
		idx := component.LobbyIndexComponent{}
		idx.Init()
		lobbyIdx.Index.Set(idx)

		state.Logger().Info().Msg("Created lobby index entity")
	}

	// Check if config already exists
	hasConfig := false
	for range state.Configs.Iter() {
		hasConfig = true
		break
	}

	if !hasConfig {
		// Create config singleton with stored config (or defaults)
		_, cfg := state.Configs.Create()
		config := storedConfig
		if config.HeartbeatTimeoutSeconds <= 0 {
			config.HeartbeatTimeoutSeconds = 60
		}
		cfg.Config.Set(config)

		state.Logger().Info().
			Int64("heartbeat_timeout", config.HeartbeatTimeoutSeconds).
			Msg("Created lobby config entity")
	}

	return nil
}
