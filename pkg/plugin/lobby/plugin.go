// Package lobby provides a flexible lobby/party system for Cardinal worlds.
//
// This package handles player grouping and session management. Players can
// create lobbies, invite friends via invite codes, form teams, ready up, and start sessions.
//
// Usage:
//
//	w, err := cardinal.NewWorld(cardinal.WorldOptions{...})
//	if err != nil {
//		panic(err)
//	}
//	w.RegisterPlugin(lobby.NewPlugin(lobby.Config{
//		LobbyWorld: myLobbyWorld,
//	}))
//	w.StartGame()
//
// The package registers the following systems:
//   - InitSystem (Init hook): Invalidates the lookup index so the next tick rebuilds it
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
	"github.com/argus-labs/world-engine/pkg/plugin/lobby/component"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby/system"
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
	ShardAddress    = component.ShardAddress

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
	GetLobbyCommand                 = system.GetLobbyCommand

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
	GetLobbyResult                 = system.GetLobbyResult

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

	// LobbyPresets is the server-owned registry of team configurations
	// that clients can reference by label in CreateLobbyCommand.Preset.
	// The map key is the preset label; the value is the ordered list of
	// teams that a lobby created with that preset will contain. Server
	// operators are the source of truth for team caps; clients cannot
	// supply arbitrary team configurations.
	LobbyPresets map[string][]TeamConfig
}

// Plugin owns the configuration and derived lookup index for one Cardinal world.
// Create a separate instance for each world.
type Plugin struct {
	config  Config
	runtime *system.Runtime
}

var _ cardinal.Plugin = (*Plugin)(nil)

// NewPlugin creates a new lobby plugin with the given configuration.
func NewPlugin(config Config) *Plugin {
	return &Plugin{config: config}
}

// Register implements cardinal.Plugin. Each plugin instance belongs to one world.
// Registering the same instance twice panics.
func (p *Plugin) Register(w *cardinal.World) {
	if p.runtime != nil {
		panic("lobby: Plugin.Register called twice on the same instance; create a separate plugin instance per world")
	}
	runtime := system.NewRuntime(system.Config{
		LobbyWorld:           component.ShardAddress(p.config.LobbyWorld),
		HeartbeatTimeout:     p.config.HeartbeatTimeout,
		AssignmentAuthority:  p.config.AssignmentAuthority,
		MaxAllocationTimeout: p.config.MaxAllocationTimeout,
	}, p.config.Provider, p.config.LobbyPresets)

	w.RegisterComponent[component.LobbyComponent]()
	w.RegisterComponent[component.PlayerComponent]()

	registerCommandsAndEvents(w)

	// Register init system (runs once during world initialization)
	w.RegisterSystem(system.NewInitSystem(runtime), cardinal.WithHook(cardinal.Init))

	// Register lobby system (runs every tick)
	w.RegisterSystem(system.NewLobbySystem(runtime))

	// Register heartbeat system (runs every tick)
	w.RegisterSystem(system.NewHeartbeatSystem(runtime))
	p.runtime = runtime
}

// registerCommandsAndEvents registers every command the lobby systems read and every event they
// send. Using an unregistered one panics at runtime.
func registerCommandsAndEvents(w *cardinal.World) {
	// Commands read by LobbySystem and HeartbeatSystem.
	w.RegisterCommand[system.CreateLobbyCommand]()
	w.RegisterCommand[system.JoinLobbyCommand]()
	w.RegisterCommand[system.JoinTeamCommand]()
	w.RegisterCommand[system.LeaveLobbyCommand]()
	w.RegisterCommand[system.SetReadyCommand]()
	w.RegisterCommand[system.KickPlayerCommand]()
	w.RegisterCommand[system.TransferLeaderCommand]()
	w.RegisterCommand[system.StartSessionCommand]()
	w.RegisterCommand[system.NotifySessionEndCommand]()
	w.RegisterCommand[system.AssignShardCommand]()
	w.RegisterCommand[system.GenerateInviteCodeCommand]()
	w.RegisterCommand[system.UpdateSessionPassthroughCommand]()
	w.RegisterCommand[system.UpdatePlayerPassthroughCommand]()
	w.RegisterCommand[system.GetPlayerCommand]()
	w.RegisterCommand[system.GetAllPlayersCommand]()
	w.RegisterCommand[system.GetLobbyCommand]()
	w.RegisterCommand[system.HeartbeatCommand]()

	// Events (Broadcast).
	w.RegisterEvent[system.LobbyCreatedEvent]()
	w.RegisterEvent[system.PlayerJoinedEvent]()
	w.RegisterEvent[system.PlayerLeftEvent]()
	w.RegisterEvent[system.PlayerKickedEvent]()
	w.RegisterEvent[system.PlayerReadyEvent]()
	w.RegisterEvent[system.PlayerChangedTeamEvent]()
	w.RegisterEvent[system.LeaderChangedEvent]()
	w.RegisterEvent[system.SessionStartedEvent]()
	w.RegisterEvent[system.SessionAwaitingAllocationEvent]()
	w.RegisterEvent[system.SessionEndedEvent]()
	w.RegisterEvent[system.InviteCodeGeneratedEvent]()
	w.RegisterEvent[system.LobbyDeletedEvent]()
	w.RegisterEvent[system.PlayerTimedOutEvent]()
	w.RegisterEvent[system.SessionPassthroughUpdatedEvent]()
	w.RegisterEvent[system.PlayerPassthroughUpdatedEvent]()

	// CommandResult (request-prefixed responses).
	w.RegisterEvent[system.CreateLobbyResult]()
	w.RegisterEvent[system.JoinLobbyResult]()
	w.RegisterEvent[system.JoinTeamResult]()
	w.RegisterEvent[system.LeaveLobbyResult]()
	w.RegisterEvent[system.SetReadyResult]()
	w.RegisterEvent[system.KickPlayerResult]()
	w.RegisterEvent[system.TransferLeaderResult]()
	w.RegisterEvent[system.StartSessionResult]()
	w.RegisterEvent[system.GenerateInviteCodeResult]()
	w.RegisterEvent[system.UpdateSessionPassthroughResult]()
	w.RegisterEvent[system.UpdatePlayerPassthroughResult]()
	w.RegisterEvent[system.GetPlayerResult]()
	w.RegisterEvent[system.GetAllPlayersResult]()
	w.RegisterEvent[system.GetLobbyResult]()
}
