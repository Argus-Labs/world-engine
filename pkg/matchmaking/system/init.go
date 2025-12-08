package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/matchmaking/component"
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
	TicketIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.TicketIndexComponent]
	}]

	ProfileIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.ProfileIndexComponent]
	}]

	BackfillIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.BackfillIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]
}

// InitSystem creates singleton index entities.
// This runs once during world initialization (Init hook).
func InitSystem(state *InitSystemState) error {
	// Check if indexes already exist
	hasTicketIndex := false
	for range state.TicketIndexes.Iter() {
		hasTicketIndex = true
		break
	}

	if !hasTicketIndex {
		// Create ticket index singleton
		_, ticketIdx := state.TicketIndexes.Create()
		idx := component.TicketIndexComponent{}
		idx.Init()
		ticketIdx.Index.Set(idx)

		state.Logger().Info().Msg("Created ticket index entity")
	}

	hasProfileIndex := false
	for range state.ProfileIndexes.Iter() {
		hasProfileIndex = true
		break
	}

	if !hasProfileIndex {
		// Create profile index singleton
		_, profileIdx := state.ProfileIndexes.Create()
		idx := component.ProfileIndexComponent{}
		idx.Init()
		profileIdx.Index.Set(idx)

		state.Logger().Info().Msg("Created profile index entity")
	}

	hasBackfillIndex := false
	for range state.BackfillIndexes.Iter() {
		hasBackfillIndex = true
		break
	}

	if !hasBackfillIndex {
		// Create backfill index singleton
		_, backfillIdx := state.BackfillIndexes.Create()
		idx := component.BackfillIndexComponent{}
		idx.Init()
		backfillIdx.Index.Set(idx)

		state.Logger().Info().Msg("Created backfill index entity")
	}

	hasConfig := false
	for range state.Configs.Iter() {
		hasConfig = true
		break
	}

	if !hasConfig {
		// Create config singleton with stored config (or defaults)
		_, cfg := state.Configs.Create()
		config := storedConfig
		if config.DefaultTTLSeconds <= 0 {
			config.DefaultTTLSeconds = 300 // 5 minutes
		}
		if config.BackfillTTLSeconds <= 0 {
			config.BackfillTTLSeconds = 60 // 1 minute
		}
		cfg.Config.Set(config)

		state.Logger().Info().
			Int64("default_ttl", config.DefaultTTLSeconds).
			Int64("backfill_ttl", config.BackfillTTLSeconds).
			Str("lobby_shard", config.LobbyShardID).
			Msg("Created config entity")
	}

	return nil
}
