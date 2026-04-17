// Package lobby provides a flexible lobby/party system for Cardinal worlds.
//
// This package handles player grouping and session management. Players can
// create lobbies, invite friends via invite codes, form teams, ready up, and start sessions.
//
// Usage:
//
//	world := cardinal.NewWorld(cardinal.WorldOptions{...})
//	cardinal.RegisterPlugin(world, lobby.NewPlugin(lobby.Config{
//		LobbyWorld: myLobbyWorld,
//	}))
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
package lobby

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/argus-labs/world-engine/pkg/lobby/system"
)

// Re-export types for easier user access.
type (
	// Data structures.
	Component       = component.LobbyComponent
	PlayerComponent = component.PlayerComponent
	Team            = component.Team
	Session         = component.Session
	SessionState    = component.SessionState
	GameWorld       = cardinal.OtherWorld
	IndexComponent  = component.LobbyIndexComponent
	ConfigComponent = component.ConfigComponent

	// Commands.
	CreateLobbyCommand              = system.CreateLobbyCommand
	TeamConfig                      = system.TeamConfig
	JoinLobbyCommand                = system.JoinLobbyCommand
	JoinTeamCommand                 = system.JoinTeamCommand
	LeaveLobbyCommand               = system.LeaveLobbyCommand
	SetReadyCommand                 = system.SetReadyCommand
	KickPlayerCommand               = system.KickPlayerCommand
	TransferLeaderCommand           = system.TransferLeaderCommand
	StartSessionCommand             = system.StartSessionCommand
	AssignShardCommand              = system.AssignShardCommand
	GenerateInviteCodeCommand       = system.GenerateInviteCodeCommand
	HeartbeatCommand                = system.HeartbeatCommand
	UpdateSessionPassthroughCommand = system.UpdateSessionPassthroughCommand
	UpdatePlayerPassthroughCommand  = system.UpdatePlayerPassthroughCommand
	GetPlayerCommand                = system.GetPlayerCommand
	GetAllPlayersCommand            = system.GetAllPlayersCommand

	// Events (Broadcast).
	CreatedEvent                   = system.LobbyCreatedEvent
	PlayerJoinedEvent              = system.PlayerJoinedEvent
	PlayerLeftEvent                = system.PlayerLeftEvent
	PlayerKickedEvent              = system.PlayerKickedEvent
	PlayerReadyEvent               = system.PlayerReadyEvent
	PlayerChangedTeamEvent         = system.PlayerChangedTeamEvent
	LeaderChangedEvent             = system.LeaderChangedEvent
	SessionStartedEvent            = system.SessionStartedEvent
	SessionAwaitingAllocationEvent = system.SessionAwaitingAllocationEvent
	SessionEndedEvent              = system.SessionEndedEvent
	InviteCodeGeneratedEvent       = system.InviteCodeGeneratedEvent
	DeletedEvent                   = system.LobbyDeletedEvent
	PlayerTimedOutEvent            = system.PlayerTimedOutEvent
	SessionPassthroughUpdatedEvent = system.SessionPassthroughUpdatedEvent
	PlayerPassthroughUpdatedEvent  = system.PlayerPassthroughUpdatedEvent

	// CommandResult (persona-prefixed responses).
	CreateLobbyResult              = system.CreateLobbyResult
	JoinLobbyResult                = system.JoinLobbyResult
	JoinTeamResult                 = system.JoinTeamResult
	LeaveLobbyResult               = system.LeaveLobbyResult
	SetReadyResult                 = system.SetReadyResult
	KickPlayerResult               = system.KickPlayerResult
	TransferLeaderResult           = system.TransferLeaderResult
	StartSessionResult             = system.StartSessionResult
	GenerateInviteCodeResult       = system.GenerateInviteCodeResult
	UpdateSessionPassthroughResult = system.UpdateSessionPassthroughResult
	UpdatePlayerPassthroughResult  = system.UpdatePlayerPassthroughResult
	GetPlayerResult                = system.GetPlayerResult
	GetAllPlayersResult            = system.GetAllPlayersResult

	// Cross-Shard Commands.
	NotifySessionStartCommand = system.NotifySessionStartCommand
	NotifySessionEndCommand   = system.NotifySessionEndCommand
	StartSessionPayload       = system.StartSessionPayload // Alias for NotifySessionStartCommand

	// Provider.
	Provider        = system.LobbyProvider
	DefaultProvider = system.DefaultProvider
)

// Session states.
const (
	SessionStateIdle               = component.SessionStateIdle
	SessionStateAwaitingAllocation = component.SessionStateAwaitingAllocation
	SessionStateInSession          = component.SessionStateInSession
)

// Config holds configuration for the lobby package.
type Config struct {
	// LobbyWorld is this lobby shard's address.
	// Included in NotifySessionStartCommand so game shard can send NotifySessionEndCommand back.
	LobbyWorld cardinal.OtherWorld

	// Provider is the customizable provider for the lobby system.
	// If nil, DefaultProvider is used.
	Provider Provider

	// AssignmentAuthority is an accident-prevention filter — not
	// authentication. Dropped commands whose cmd.Persona differs from
	// this value. cmd.Persona is not signature-verified at this layer, so
	// this does NOT protect against a malicious client; real auth belongs
	// above the plugin (NATS ACLs, gateway auth). Empty = no filter.
	AssignmentAuthority string

	// MaxAllocationTimeout bounds how long (in seconds) a lobby may sit in
	// SessionStateAwaitingAllocation before the lobby fails the start
	// itself. Values <= 0 disable timeout enforcement.
	MaxAllocationTimeout int64

	// HeartbeatTimeout is how long (in seconds) before a player is removed for not sending heartbeats.
	// Clients should send heartbeats more frequently than this (e.g., every timeout/3 seconds).
	// Default: 30 seconds.
	HeartbeatTimeout int64
}

// Plugin implements cardinal.Plugin for the lobby system.
type Plugin struct {
	config Config
}

var _ cardinal.Plugin = (*Plugin)(nil)

// NewPlugin creates a new lobby plugin with the given configuration.
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

// Register implements cardinal.Plugin.
func (p *Plugin) Register(world *cardinal.World) {
	system.SetConfig(component.ConfigComponent{
		LobbyWorld:           p.config.LobbyWorld,
		HeartbeatTimeout:     p.config.HeartbeatTimeout,
		AssignmentAuthority:  p.config.AssignmentAuthority,
		MaxAllocationTimeout: p.config.MaxAllocationTimeout,
	})

	// Store provider
	system.SetProvider(p.config.Provider)

	// Register init system (runs once during world initialization)
	cardinal.RegisterSystem(world, system.InitSystem, cardinal.WithHook(cardinal.Init))

	// Register lobby system (runs every tick)
	cardinal.RegisterSystem(world, system.LobbySystem)

	// Register heartbeat system (runs every tick)
	cardinal.RegisterSystem(world, system.HeartbeatSystem)
}
