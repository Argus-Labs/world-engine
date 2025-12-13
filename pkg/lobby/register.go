// Package lobby provides a flexible lobby/party system for Cardinal worlds.
//
// This package handles player grouping and session management. Players can
// create lobbies, invite friends via invite codes, form teams, ready up, and start sessions.
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	lobby.Register(world, lobby.Config{
//		GameShardID: "game-shard-1",
//	})
//	world.StartGame()
//
// The package registers the following systems:
//   - InitSystem (Init hook): Creates singleton index and config entities
//   - LobbySystem (Update hook): Processes lobby commands
//
// Commands:
//   - CreateLobby: Player creates a new lobby, becomes leader
//   - JoinLobby: Player joins via invite code
//   - JoinTeam: Player moves to a different team
//   - LeaveLobby: Player leaves current lobby
//   - SetReady: Player marks ready/unready
//   - KickPlayer: Leader removes a player
//   - TransferLeader: Leader gives leadership to another
//   - StartSession: Leader starts the session
//   - EndSession: End the current session
//   - GenerateInviteCode: Leader generates new invite code
//
// Queries:
//   - GetLobby: Get lobby by ID
//   - GetMyLobby: Get player's current lobby
//   - GetLobbyByInviteCode: Preview lobby before joining
package lobby

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/argus-labs/world-engine/pkg/lobby/system"
)

// Re-export types for easier user access
type (
	// Data structures
	LobbyComponent      = component.LobbyComponent
	Team                = component.Team
	PlayerState         = component.PlayerState
	Session             = component.Session
	SessionState        = component.SessionState
	LobbyIndexComponent = component.LobbyIndexComponent
	ConfigComponent     = component.ConfigComponent

	// Commands
	CreateLobbyCommand        = system.CreateLobbyCommand
	TeamConfig                = system.TeamConfig
	JoinLobbyCommand          = system.JoinLobbyCommand
	JoinTeamCommand           = system.JoinTeamCommand
	LeaveLobbyCommand         = system.LeaveLobbyCommand
	SetReadyCommand           = system.SetReadyCommand
	KickPlayerCommand         = system.KickPlayerCommand
	TransferLeaderCommand     = system.TransferLeaderCommand
	StartSessionCommand       = system.StartSessionCommand
	EndSessionCommand         = system.EndSessionCommand
	GenerateInviteCodeCommand = system.GenerateInviteCodeCommand

	// Events
	LobbyCreatedEvent        = system.LobbyCreatedEvent
	PlayerJoinedEvent        = system.PlayerJoinedEvent
	PlayerLeftEvent          = system.PlayerLeftEvent
	PlayerKickedEvent        = system.PlayerKickedEvent
	PlayerReadyEvent         = system.PlayerReadyEvent
	PlayerChangedTeamEvent   = system.PlayerChangedTeamEvent
	LeaderChangedEvent       = system.LeaderChangedEvent
	SessionStartedEvent      = system.SessionStartedEvent
	SessionEndedEvent        = system.SessionEndedEvent
	InviteCodeGeneratedEvent = system.InviteCodeGeneratedEvent
	LobbyErrorEvent          = system.LobbyErrorEvent
	LobbyDeletedEvent        = system.LobbyDeletedEvent

	// Cross-Shard Commands
	NotifySessionStartCommand = system.NotifySessionStartCommand
	StartSessionPayload       = system.StartSessionPayload // Alias for NotifySessionStartCommand

	// Provider
	LobbyProvider   = system.LobbyProvider
	DefaultProvider = system.DefaultProvider
)

// Session states
const (
	SessionStateIdle      = component.SessionStateIdle
	SessionStateInSession = component.SessionStateInSession
)

// Config holds configuration for the lobby package.
type Config struct {
	// GameShardID is the shard ID for the game shard (for sending session starts).
	// If empty, no cross-shard notification is sent.
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

	// LobbyShardAddress is this lobby shard's full address string.
	// Included in NotifySessionStartCommand so game shard can send EndSession back.
	// Format: "<region>.<realm>.<organization>.<project>.<service_id>"
	LobbyShardAddress string

	// Provider is the customizable provider for the lobby system.
	// If nil, DefaultProvider is used.
	Provider LobbyProvider
}

// Register registers the lobby systems with the given world.
// This should be called before world.StartGame().
func Register(world *cardinal.World, config Config) {
	// Store config for init system to use
	system.SetConfig(component.ConfigComponent{
		GameShardID:       config.GameShardID,
		GameRegion:        config.GameRegion,
		GameOrganization:  config.GameOrganization,
		GameProject:       config.GameProject,
		LobbyShardAddress: config.LobbyShardAddress,
	})

	// Store provider
	system.SetProvider(config.Provider)

	// Register init system (runs once during world initialization)
	cardinal.RegisterSystem(world, system.InitSystem, cardinal.WithHook(cardinal.Init))

	// Register lobby system (runs every tick)
	cardinal.RegisterSystem(world, system.LobbySystem)
}
