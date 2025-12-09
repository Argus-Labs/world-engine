// Package lobby provides an embeddable lobby system for Cardinal worlds.
//
// This package handles matchmaking-created lobbies only. When matchmaking finds
// a match, it sends a CreateLobbyFromMatch command/event to create a lobby that
// immediately transitions to in_game state.
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	lobby.Register(world, lobby.Config{
//		HeartbeatTimeoutSeconds: 60,
//	})
//	world.StartGame()
//
// The package registers the following systems:
//   - InitSystem (Init hook): Creates singleton index entities
//   - LobbySystem (Update hook): Processes lobby commands
//
// Lobby Commands:
//   - EndGameCommand: End the game
//   - HeartbeatCommand: Keep lobby alive during game
//
// Cross-Shard Commands:
//   - CreateLobbyFromMatchCommand: Received from matchmaking shard
//   - NotifyGameStartCommand: Sent to game shard
//   - NotifyGameEndCommand: Received from game shard
//   - PlayerDisconnectedCommand: Received from game shard
package lobby

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/argus-labs/world-engine/pkg/lobby/system"
)

// Re-export types for easier user access
type (
	// Lobby Commands
	EndGameCommand   = system.EndGameCommand
	HeartbeatCommand = system.HeartbeatCommand

	// Lobby Events
	LobbyCreatedEvent       = system.LobbyCreatedEvent
	GameStartedEvent        = system.GameStartedEvent
	GameEndedEvent          = system.GameEndedEvent
	LobbyErrorEvent         = system.LobbyErrorEvent
	PlayerDisconnectedEvent = system.PlayerDisconnectedEvent

	// Cross-Shard Communication Types
	CreateLobbyFromMatchEvent   = system.CreateLobbyFromMatchEvent   // Received from matchmaking (same-shard)
	CreateLobbyFromMatchCommand = system.CreateLobbyFromMatchCommand // Received from matchmaking (cross-shard)
	NotifyGameStartEvent        = system.NotifyGameStartEvent        // Sent to game (same-shard)
	NotifyGameStartCommand      = system.NotifyGameStartCommand      // Sent to game (cross-shard)
	NotifyGameEndCommand        = system.NotifyGameEndCommand        // Received from game (cross-shard)
	PlayerDisconnectedCommand   = system.PlayerDisconnectedCommand   // Received from game (cross-shard)
	LobbyTeamInfo               = system.LobbyTeamInfo

	// Components
	LobbyComponent = component.LobbyComponent
	LobbyTeam      = component.LobbyTeam
	LobbyState     = component.LobbyState
)

// Lobby states
const (
	LobbyStateInGame = component.LobbyStateInGame
	LobbyStateEnded  = component.LobbyStateEnded
)

// Config holds configuration for the lobby package.
type Config struct {
	// MatchmakingShardID is the shard ID for matchmaking (for receiving matches).
	// If empty, same-shard communication via SystemEvents is used.
	MatchmakingShardID string

	// GameShardID is the shard ID for the game shard (for sending game starts).
	// If empty, same-shard communication via SystemEvents is used.
	GameShardID string

	// GameRegion is the region for the game shard (for cross-shard).
	// Required when GameShardID is set.
	GameRegion string

	// GameOrganization is the organization for the game shard (for cross-shard).
	// Required when GameShardID is set.
	GameOrganization string

	// GameProject is the project for the game shard (for cross-shard).
	// Required when GameShardID is set.
	GameProject string

	// HeartbeatTimeoutSeconds is how long before a lobby is considered stale.
	// If 0, defaults to 60.
	HeartbeatTimeoutSeconds int64
}

// Register registers the lobby systems with the given world.
// This should be called before world.StartGame().
func Register(world *cardinal.World, config Config) {
	// Apply defaults
	if config.HeartbeatTimeoutSeconds <= 0 {
		config.HeartbeatTimeoutSeconds = 60
	}

	// Store config for init system to use
	system.SetConfig(component.ConfigComponent{
		MatchmakingShardID:      config.MatchmakingShardID,
		GameShardID:             config.GameShardID,
		GameRegion:              config.GameRegion,
		GameOrganization:        config.GameOrganization,
		GameProject:             config.GameProject,
		HeartbeatTimeoutSeconds: config.HeartbeatTimeoutSeconds,
	})

	// Register init system (runs once during world initialization)
	cardinal.RegisterSystem(world, system.InitSystem, cardinal.WithHook(cardinal.Init))

	// Register lobby system (runs every tick)
	cardinal.RegisterSystem(world, system.LobbySystem)
}
