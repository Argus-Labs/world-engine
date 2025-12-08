// Package lobby provides an embeddable lobby system for Cardinal worlds.
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	lobby.Register(world, lobby.Config{
//		DefaultMaxPartySize:     4,
//		HeartbeatTimeoutSeconds: 60,
//	})
//	world.StartGame()
//
// The package registers the following systems:
//   - InitSystem (Init hook): Creates singleton index entities
//   - PartySystem (Update hook): Processes party commands
//   - LobbySystem (Update hook): Processes lobby commands
//
// Party Commands:
//   - CreatePartyCommand: Create a new party
//   - JoinPartyCommand: Join an existing party
//   - LeavePartyCommand: Leave current party
//   - SetPartyOpenCommand: Set party open/closed status
//   - PromoteLeaderCommand: Promote another member to leader
//   - KickFromPartyCommand: Kick a member from party
//
// Lobby Commands:
//   - CreateLobbyCommand: Create a new lobby
//   - JoinLobbyCommand: Join a lobby
//   - LeaveLobbyCommand: Leave current lobby
//   - SetReadyCommand: Set ready status
//   - StartGameCommand: Start the game
//   - EndGameCommand: End the game
//   - HeartbeatCommand: Keep lobby alive during game
package lobby

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/argus-labs/world-engine/pkg/lobby/system"
)

// Re-export types for easier user access
type (
	// Party Commands
	CreatePartyCommand   = system.CreatePartyCommand
	JoinPartyCommand     = system.JoinPartyCommand
	LeavePartyCommand    = system.LeavePartyCommand
	SetPartyOpenCommand  = system.SetPartyOpenCommand
	PromoteLeaderCommand = system.PromoteLeaderCommand
	KickFromPartyCommand = system.KickFromPartyCommand

	// Party Events
	PartyCreatedEvent      = system.PartyCreatedEvent
	PlayerJoinedPartyEvent = system.PlayerJoinedPartyEvent
	PlayerLeftPartyEvent   = system.PlayerLeftPartyEvent
	PartyDisbandedEvent    = system.PartyDisbandedEvent
	LeaderChangedEvent     = system.LeaderChangedEvent
	PartyErrorEvent        = system.PartyErrorEvent

	// Lobby Commands
	CreateLobbyCommand = system.CreateLobbyCommand
	JoinLobbyCommand   = system.JoinLobbyCommand
	LeaveLobbyCommand  = system.LeaveLobbyCommand
	SetReadyCommand    = system.SetReadyCommand
	StartGameCommand   = system.StartGameCommand
	EndGameCommand     = system.EndGameCommand
	HeartbeatCommand   = system.HeartbeatCommand

	// Lobby Events
	LobbyCreatedEvent     = system.LobbyCreatedEvent
	PartyJoinedLobbyEvent = system.PartyJoinedLobbyEvent
	PartyLeftLobbyEvent   = system.PartyLeftLobbyEvent
	LobbyReadyEvent       = system.LobbyReadyEvent
	GameStartedEvent      = system.GameStartedEvent
	GameEndedEvent        = system.GameEndedEvent
	LobbyDisbandedEvent   = system.LobbyDisbandedEvent
	LobbyErrorEvent       = system.LobbyErrorEvent

	// Cross-Shard Communication Types
	CreateLobbyFromMatchEvent   = system.CreateLobbyFromMatchEvent   // Received from matchmaking (same-shard)
	CreateLobbyFromMatchCommand = system.CreateLobbyFromMatchCommand // Received from matchmaking (cross-shard)
	NotifyGameStartEvent        = system.NotifyGameStartEvent        // Sent to game (same-shard)
	NotifyGameStartCommand      = system.NotifyGameStartCommand      // Sent to game (cross-shard)
	LobbyTeamInfo               = system.LobbyTeamInfo

	// Components
	PartyComponent = component.PartyComponent
	LobbyComponent = component.LobbyComponent
	LobbyTeam      = component.LobbyTeam
	LobbyState     = component.LobbyState
)

// Lobby states
const (
	LobbyStateWaiting = component.LobbyStateWaiting
	LobbyStateReady   = component.LobbyStateReady
	LobbyStateInGame  = component.LobbyStateInGame
	LobbyStateEnded   = component.LobbyStateEnded
)

// Config holds configuration for the lobby package.
type Config struct {
	// MatchmakingShardID is the shard ID for matchmaking (for receiving matches).
	// If empty, same-shard communication via SystemEvents is used.
	MatchmakingShardID string

	// GameShardID is the shard ID for the game shard (for sending game starts).
	// If empty, same-shard communication via SystemEvents is used.
	GameShardID string

	// DefaultMaxPartySize is the default max party size.
	// If 0, defaults to 4.
	DefaultMaxPartySize int

	// HeartbeatTimeoutSeconds is how long before a lobby is considered stale.
	// If 0, defaults to 60.
	HeartbeatTimeoutSeconds int64
}

// Register registers the lobby systems with the given world.
// This should be called before world.StartGame().
func Register(world *cardinal.World, config Config) {
	// Apply defaults
	if config.DefaultMaxPartySize <= 0 {
		config.DefaultMaxPartySize = 4
	}
	if config.HeartbeatTimeoutSeconds <= 0 {
		config.HeartbeatTimeoutSeconds = 60
	}

	// Store config for init system to use
	system.SetConfig(component.ConfigComponent{
		MatchmakingShardID:      config.MatchmakingShardID,
		GameShardID:             config.GameShardID,
		DefaultMaxPartySize:     config.DefaultMaxPartySize,
		HeartbeatTimeoutSeconds: config.HeartbeatTimeoutSeconds,
	})

	// Register init system (runs once during world initialization)
	cardinal.RegisterSystem(world, system.InitSystem, cardinal.WithHook(cardinal.Init))

	// Register party system (runs every tick)
	cardinal.RegisterSystem(world, system.PartySystem)

	// Register lobby system (runs every tick)
	cardinal.RegisterSystem(world, system.LobbySystem)
}
