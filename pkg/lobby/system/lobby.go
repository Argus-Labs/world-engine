package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/google/uuid"
)

// -----------------------------------------------------------------------------
// Commands
// -----------------------------------------------------------------------------

// CreateLobbyCommand creates a new lobby with the sender as leader.
type CreateLobbyCommand struct {
	cardinal.BaseCommand
	// Teams is the initial team configuration for the lobby.
	Teams []TeamConfig `json:"teams,omitempty"`
}

// TeamConfig defines initial team configuration.
type TeamConfig struct {
	Name       string `json:"name"`
	MaxPlayers int    `json:"max_players"`
}

// Name returns the command name.
func (CreateLobbyCommand) Name() string { return "lobby_create" }

// JoinLobbyCommand joins an existing lobby via invite code.
type JoinLobbyCommand struct {
	cardinal.BaseCommand
	InviteCode string `json:"invite_code"`
	TeamID     string `json:"team_id,omitempty"` // Optional: join specific team
}

// Name returns the command name.
func (JoinLobbyCommand) Name() string { return "lobby_join" }

// JoinTeamCommand moves a player to a different team.
type JoinTeamCommand struct {
	cardinal.BaseCommand
	TeamID string `json:"team_id"`
}

// Name returns the command name.
func (JoinTeamCommand) Name() string { return "lobby_join_team" }

// LeaveLobbyCommand leaves the current lobby.
type LeaveLobbyCommand struct {
	cardinal.BaseCommand
}

// Name returns the command name.
func (LeaveLobbyCommand) Name() string { return "lobby_leave" }

// SetReadyCommand sets the player's ready status.
type SetReadyCommand struct {
	cardinal.BaseCommand
	IsReady bool `json:"is_ready"`
}

// Name returns the command name.
func (SetReadyCommand) Name() string { return "lobby_set_ready" }

// KickPlayerCommand kicks a player from the lobby (leader only).
type KickPlayerCommand struct {
	cardinal.BaseCommand
	TargetPlayerID string `json:"target_player_id"`
}

// Name returns the command name.
func (KickPlayerCommand) Name() string { return "lobby_kick" }

// TransferLeaderCommand transfers leadership to another player.
type TransferLeaderCommand struct {
	cardinal.BaseCommand
	TargetPlayerID string `json:"target_player_id"`
}

// Name returns the command name.
func (TransferLeaderCommand) Name() string { return "lobby_transfer_leader" }

// StartSessionCommand starts the session (leader only).
type StartSessionCommand struct {
	cardinal.BaseCommand
	PassthroughData map[string]any `json:"passthrough_data,omitempty"`
}

// Name returns the command name.
func (StartSessionCommand) Name() string { return "lobby_start_session" }

// EndSessionCommand ends the current session.
type EndSessionCommand struct {
	cardinal.BaseCommand
	LobbyID string `json:"lobby_id"`
}

// Name returns the command name.
func (EndSessionCommand) Name() string { return "lobby_end_session" }

// GenerateInviteCodeCommand generates a new invite code (leader only).
type GenerateInviteCodeCommand struct {
	cardinal.BaseCommand
}

// Name returns the command name.
func (GenerateInviteCodeCommand) Name() string { return "lobby_generate_invite" }

// -----------------------------------------------------------------------------
// Events
// -----------------------------------------------------------------------------

// LobbyCreatedEvent is emitted when a lobby is created.
type LobbyCreatedEvent struct {
	cardinal.BaseEvent
	LobbyID    string `json:"lobby_id"`
	LeaderID   string `json:"leader_id"`
	InviteCode string `json:"invite_code"`
}

// Name returns the event name.
func (LobbyCreatedEvent) Name() string { return "lobby_created" }

// PlayerJoinedEvent is emitted when a player joins a lobby.
type PlayerJoinedEvent struct {
	cardinal.BaseEvent
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
	TeamID   string `json:"team_id"`
}

// Name returns the event name.
func (PlayerJoinedEvent) Name() string { return "lobby_player_joined" }

// PlayerLeftEvent is emitted when a player leaves a lobby.
type PlayerLeftEvent struct {
	cardinal.BaseEvent
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
}

// Name returns the event name.
func (PlayerLeftEvent) Name() string { return "lobby_player_left" }

// PlayerKickedEvent is emitted when a player is kicked.
type PlayerKickedEvent struct {
	cardinal.BaseEvent
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
	KickerID string `json:"kicker_id"`
}

// Name returns the event name.
func (PlayerKickedEvent) Name() string { return "lobby_player_kicked" }

// PlayerReadyEvent is emitted when a player changes ready status.
type PlayerReadyEvent struct {
	cardinal.BaseEvent
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
	IsReady  bool   `json:"is_ready"`
}

// Name returns the event name.
func (PlayerReadyEvent) Name() string { return "lobby_player_ready" }

// PlayerChangedTeamEvent is emitted when a player changes team.
type PlayerChangedTeamEvent struct {
	cardinal.BaseEvent
	LobbyID   string `json:"lobby_id"`
	PlayerID  string `json:"player_id"`
	OldTeamID string `json:"old_team_id"`
	NewTeamID string `json:"new_team_id"`
}

// Name returns the event name.
func (PlayerChangedTeamEvent) Name() string { return "lobby_player_changed_team" }

// LeaderChangedEvent is emitted when leadership is transferred.
type LeaderChangedEvent struct {
	cardinal.BaseEvent
	LobbyID     string `json:"lobby_id"`
	OldLeaderID string `json:"old_leader_id"`
	NewLeaderID string `json:"new_leader_id"`
}

// Name returns the event name.
func (LeaderChangedEvent) Name() string { return "lobby_leader_changed" }

// SessionStartedEvent is emitted when a session starts.
type SessionStartedEvent struct {
	cardinal.BaseEvent
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (SessionStartedEvent) Name() string { return "lobby_session_started" }

// SessionEndedEvent is emitted when a session ends.
type SessionEndedEvent struct {
	cardinal.BaseEvent
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (SessionEndedEvent) Name() string { return "lobby_session_ended" }

// InviteCodeGeneratedEvent is emitted when a new invite code is generated.
type InviteCodeGeneratedEvent struct {
	cardinal.BaseEvent
	LobbyID    string `json:"lobby_id"`
	InviteCode string `json:"invite_code"`
}

// Name returns the event name.
func (InviteCodeGeneratedEvent) Name() string { return "lobby_invite_generated" }

// LobbyErrorEvent is emitted when an error occurs.
type LobbyErrorEvent struct {
	cardinal.BaseEvent
	LobbyID string `json:"lobby_id,omitempty"`
	Error   string `json:"error"`
}

// Name returns the event name.
func (LobbyErrorEvent) Name() string { return "lobby_error" }

// LobbyDeletedEvent is emitted when a lobby is deleted.
type LobbyDeletedEvent struct {
	cardinal.BaseEvent
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (LobbyDeletedEvent) Name() string { return "lobby_deleted" }

// -----------------------------------------------------------------------------
// Cross-Shard Commands
// -----------------------------------------------------------------------------

// NotifySessionStartCommand is sent to game shard when a session starts.
type NotifySessionStartCommand struct {
	cardinal.BaseCommand
	Lobby             component.LobbyComponent `json:"lobby"`
	LobbyShardAddress string                   `json:"lobby_shard_address"`
}

// Name returns the command name.
func (NotifySessionStartCommand) Name() string { return "lobby_notify_session_start" }

// StartSessionPayload is an alias for NotifySessionStartCommand for documentation clarity.
type StartSessionPayload = NotifySessionStartCommand

// -----------------------------------------------------------------------------
// Provider Interface
// -----------------------------------------------------------------------------

// LobbyProvider defines customizable behavior for the lobby system.
type LobbyProvider interface {
	// GenerateInviteCode generates a custom invite code for the lobby.
	GenerateInviteCode(lobby *component.LobbyComponent) string
}

// DefaultProvider provides default implementations.
type DefaultProvider struct{}

// inviteCodeCharset contains uppercase alphanumeric characters excluding confusing ones (0, O, I, L, 1).
const inviteCodeCharset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenerateInviteCode generates a 6-character invite code using Hash(LobbyID + Now()).
func (DefaultProvider) GenerateInviteCode(lobby *component.LobbyComponent) string {
	// Create hash from lobby ID + current timestamp
	data := fmt.Sprintf("%s:%d", lobby.ID, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	hexStr := hex.EncodeToString(hash[:])

	// Convert to 6-char code using our charset
	code := make([]byte, 6)
	for i := 0; i < 6; i++ {
		// Use each hex byte to index into charset
		idx := int(hexStr[i]) % len(inviteCodeCharset)
		code[i] = inviteCodeCharset[idx]
	}
	return string(code)
}

// storedProvider holds the provider set by the Register function.
var storedProvider LobbyProvider = DefaultProvider{}

// SetProvider stores the provider for the system to use.
func SetProvider(provider LobbyProvider) {
	if provider != nil {
		storedProvider = provider
	}
}

// storedConfig holds the configuration set by the Register function.
var storedConfig component.ConfigComponent

// SetConfig stores the configuration for the init system to use.
func SetConfig(config component.ConfigComponent) {
	storedConfig = config
}

// -----------------------------------------------------------------------------
// Init System
// -----------------------------------------------------------------------------

// InitSystemState is the state for the init system.
type InitSystemState struct {
	cardinal.BaseSystemState

	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]
}

// InitSystem creates singleton index entities.
func InitSystem(state *InitSystemState) error {
	// Check if lobby index already exists
	hasLobbyIndex := false
	for range state.LobbyIndexes.Iter() {
		hasLobbyIndex = true
		break
	}

	if !hasLobbyIndex {
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
		_, cfg := state.Configs.Create()
		cfg.Config.Set(storedConfig)
		state.Logger().Info().Msg("Created lobby config entity")
	}

	return nil
}

// -----------------------------------------------------------------------------
// Lobby System
// -----------------------------------------------------------------------------

// LobbySystemState is the state for the lobby system.
type LobbySystemState struct {
	cardinal.BaseSystemState

	// Commands
	CreateLobbyCmds        cardinal.WithCommand[CreateLobbyCommand]
	JoinLobbyCmds          cardinal.WithCommand[JoinLobbyCommand]
	JoinTeamCmds           cardinal.WithCommand[JoinTeamCommand]
	LeaveLobbyCmds         cardinal.WithCommand[LeaveLobbyCommand]
	SetReadyCmds           cardinal.WithCommand[SetReadyCommand]
	KickPlayerCmds         cardinal.WithCommand[KickPlayerCommand]
	TransferLeaderCmds     cardinal.WithCommand[TransferLeaderCommand]
	StartSessionCmds       cardinal.WithCommand[StartSessionCommand]
	EndSessionCmds         cardinal.WithCommand[EndSessionCommand]
	GenerateInviteCodeCmds cardinal.WithCommand[GenerateInviteCodeCommand]

	// Entities
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}]

	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events
	LobbyCreatedEvents        cardinal.WithEvent[LobbyCreatedEvent]
	PlayerJoinedEvents        cardinal.WithEvent[PlayerJoinedEvent]
	PlayerLeftEvents          cardinal.WithEvent[PlayerLeftEvent]
	PlayerKickedEvents        cardinal.WithEvent[PlayerKickedEvent]
	PlayerReadyEvents         cardinal.WithEvent[PlayerReadyEvent]
	PlayerChangedTeamEvents   cardinal.WithEvent[PlayerChangedTeamEvent]
	LeaderChangedEvents       cardinal.WithEvent[LeaderChangedEvent]
	SessionStartedEvents      cardinal.WithEvent[SessionStartedEvent]
	SessionEndedEvents        cardinal.WithEvent[SessionEndedEvent]
	InviteCodeGeneratedEvents cardinal.WithEvent[InviteCodeGeneratedEvent]
	LobbyErrorEvents          cardinal.WithEvent[LobbyErrorEvent]
	LobbyDeletedEvents        cardinal.WithEvent[LobbyDeletedEvent]
}

// LobbySystem processes lobby commands.
func LobbySystem(state *LobbySystemState) error {
	now := state.Timestamp().Unix()

	// Get lobby index
	var lobbyIndex component.LobbyIndexComponent
	var lobbyIndexEntityID ecs.EntityID
	for eid, idx := range state.LobbyIndexes.Iter() {
		lobbyIndex = idx.Index.Get()
		lobbyIndexEntityID = eid
		break
	}

	// Get config
	var config component.ConfigComponent
	for _, cfg := range state.Configs.Iter() {
		config = cfg.Config.Get()
		break
	}
	_ = config // Used for OnStartShardAddress

	// Process CreateLobby commands
	for cmd := range state.CreateLobbyCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		// Check if player is already in a lobby
		if _, exists := lobbyIndex.GetPlayerLobby(playerID); exists {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				Error: "player already in a lobby",
			})
			continue
		}

		// Generate lobby ID
		lobbyID := generateID()

		// Create lobby with initial data for invite code generation
		lobby := component.LobbyComponent{
			ID:         lobbyID,
			LeaderID:   playerID,
			InviteCode: "", // Will be set after generation
			Session: component.Session{
				State: component.SessionStateIdle,
			},
			CreatedAt: now,
		}

		// Create teams from config or default single team
		if len(payload.Teams) > 0 {
			for i, tc := range payload.Teams {
				lobby.Teams = append(lobby.Teams, component.Team{
					TeamID:     fmt.Sprintf("team_%d", i),
					Name:       tc.Name,
					MaxPlayers: tc.MaxPlayers,
				})
			}
		} else {
			// Default: single team with no limit
			lobby.Teams = []component.Team{{
				TeamID:     "default",
				Name:       "Default",
				MaxPlayers: 0,
			}}
		}

		// Generate invite code with collision check (max 3 retries)
		var inviteCode string
		inviteCodeValid := false
		for i := 0; i < 3; i++ {
			inviteCode = storedProvider.GenerateInviteCode(&lobby)
			if _, exists := lobbyIndex.GetLobbyByInviteCode(inviteCode); !exists {
				inviteCodeValid = true
				break
			}
		}
		if !inviteCodeValid {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				Error: "invite code already exists",
			})
			continue
		}
		lobby.InviteCode = inviteCode

		// Add leader to first team
		lobby.Teams[0].Players = append(lobby.Teams[0].Players, component.PlayerState{
			PlayerID: playerID,
			IsReady:  false,
		})

		// Create entity
		eid, lobbyEntity := state.Lobbies.Create()
		lobbyEntity.Lobby.Set(lobby)

		// Update index
		lobbyIndex.AddLobby(lobbyID, uint32(eid), inviteCode)
		lobbyIndex.AddPlayerToLobby(playerID, lobbyID)

		// Emit event
		state.LobbyCreatedEvents.Emit(LobbyCreatedEvent{
			LobbyID:    lobbyID,
			LeaderID:   playerID,
			InviteCode: inviteCode,
		})

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("leader_id", playerID).
			Msg("Lobby created")
	}

	// Process JoinLobby commands
	for cmd := range state.JoinLobbyCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		// Check if player is already in a lobby
		if _, exists := lobbyIndex.GetPlayerLobby(playerID); exists {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				Error: "player already in a lobby",
			})
			continue
		}

		// Find lobby by invite code
		lobbyID, exists := lobbyIndex.GetLobbyByInviteCode(payload.InviteCode)
		if !exists {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				Error: "invalid invite code",
			})
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Check if lobby is in session
		if lobby.Session.State == component.SessionStateInSession {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "lobby is in session",
			})
			continue
		}

		// Determine which team to join
		targetTeamID := payload.TeamID
		if targetTeamID == "" && len(lobby.Teams) > 0 {
			// Default to first team with space
			for _, team := range lobby.Teams {
				if !team.IsFull() {
					targetTeamID = team.TeamID
					break
				}
			}
		}

		if targetTeamID == "" {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "no team available",
			})
			continue
		}

		// Add player to team
		if !lobby.AddPlayerToTeam(playerID, targetTeamID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "failed to join team",
			})
			continue
		}

		lobbyEntity.Lobby.Set(lobby)
		lobbyIndex.AddPlayerToLobby(playerID, lobbyID)

		// Emit event
		state.PlayerJoinedEvents.Emit(PlayerJoinedEvent{
			LobbyID:  lobbyID,
			PlayerID: playerID,
			TeamID:   targetTeamID,
		})

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("team_id", targetTeamID).
			Msg("Player joined lobby")
	}

	// Process JoinTeam commands
	for cmd := range state.JoinTeamCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Can't change team during session
		if lobby.Session.State == component.SessionStateInSession {
			continue
		}

		// Get current team
		oldTeam := lobby.GetPlayerTeam(playerID)
		if oldTeam == nil {
			continue
		}
		oldTeamID := oldTeam.TeamID

		// Move to new team
		if !lobby.MovePlayerToTeam(playerID, payload.TeamID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "failed to change team",
			})
			continue
		}

		lobbyEntity.Lobby.Set(lobby)

		state.PlayerChangedTeamEvents.Emit(PlayerChangedTeamEvent{
			LobbyID:   lobbyID,
			PlayerID:  playerID,
			OldTeamID: oldTeamID,
			NewTeamID: payload.TeamID,
		})
	}

	// Process LeaveLobby commands
	for cmd := range state.LeaveLobbyCmds.Iter() {
		playerID := cmd.Persona()

		// Find player's lobby
		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Remove player
		lobby.RemovePlayer(playerID)
		lobbyIndex.RemovePlayerFromLobby(playerID)

		// Emit leave event
		state.PlayerLeftEvents.Emit(PlayerLeftEvent{
			LobbyID:  lobbyID,
			PlayerID: playerID,
		})

		// If lobby is empty, delete it
		if lobby.PlayerCount() == 0 {
			lobbyIndex.RemoveLobby(lobbyID, lobby.InviteCode)
			state.Lobbies.Destroy(ecs.EntityID(lobbyEntityID))

			state.LobbyDeletedEvents.Emit(LobbyDeletedEvent{
				LobbyID: lobbyID,
			})

			state.Logger().Info().
				Str("lobby_id", lobbyID).
				Msg("Lobby deleted (empty)")
		} else {
			// Transfer leadership if leader left
			if lobby.LeaderID == playerID {
				oldLeaderID := lobby.LeaderID
				// Find first player in any team
				for _, team := range lobby.Teams {
					if len(team.Players) > 0 {
						lobby.LeaderID = team.Players[0].PlayerID
						break
					}
				}

				state.LeaderChangedEvents.Emit(LeaderChangedEvent{
					LobbyID:     lobbyID,
					OldLeaderID: oldLeaderID,
					NewLeaderID: lobby.LeaderID,
				})
			}

			lobbyEntity.Lobby.Set(lobby)
		}

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Msg("Player left lobby")
	}

	// Process SetReady commands
	for cmd := range state.SetReadyCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Can't change ready during session
		if lobby.Session.State == component.SessionStateInSession {
			continue
		}

		lobby.SetReady(playerID, payload.IsReady)
		lobbyEntity.Lobby.Set(lobby)

		state.PlayerReadyEvents.Emit(PlayerReadyEvent{
			LobbyID:  lobbyID,
			PlayerID: playerID,
			IsReady:  payload.IsReady,
		})
	}

	// Process KickPlayer commands
	for cmd := range state.KickPlayerCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only leader can kick
		if !lobby.IsLeader(playerID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "only leader can kick players",
			})
			continue
		}

		// Can't kick self
		if payload.TargetPlayerID == playerID {
			continue
		}

		// Check if target is in lobby
		if !lobby.HasPlayer(payload.TargetPlayerID) {
			continue
		}

		// Remove player
		lobby.RemovePlayer(payload.TargetPlayerID)
		lobbyEntity.Lobby.Set(lobby)
		lobbyIndex.RemovePlayerFromLobby(payload.TargetPlayerID)

		state.PlayerKickedEvents.Emit(PlayerKickedEvent{
			LobbyID:  lobbyID,
			PlayerID: payload.TargetPlayerID,
			KickerID: playerID,
		})

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", payload.TargetPlayerID).
			Str("kicker_id", playerID).
			Msg("Player kicked from lobby")
	}

	// Process TransferLeader commands
	for cmd := range state.TransferLeaderCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only leader can transfer
		if !lobby.IsLeader(playerID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "only leader can transfer leadership",
			})
			continue
		}

		// Check if target is in lobby
		if !lobby.HasPlayer(payload.TargetPlayerID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "target player not in lobby",
			})
			continue
		}

		oldLeaderID := lobby.LeaderID
		lobby.LeaderID = payload.TargetPlayerID
		lobbyEntity.Lobby.Set(lobby)

		state.LeaderChangedEvents.Emit(LeaderChangedEvent{
			LobbyID:     lobbyID,
			OldLeaderID: oldLeaderID,
			NewLeaderID: payload.TargetPlayerID,
		})

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("old_leader", oldLeaderID).
			Str("new_leader", payload.TargetPlayerID).
			Msg("Leadership transferred")
	}

	// Process StartSession commands
	for cmd := range state.StartSessionCmds.Iter() {
		playerID := cmd.Persona()
		payload := cmd.Payload()

		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only leader can start
		if !lobby.IsLeader(playerID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "only leader can start session",
			})
			continue
		}

		// Already in session
		if lobby.Session.State == component.SessionStateInSession {
			continue
		}

		// Check all ready
		if !lobby.AllReady() {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "not all players are ready",
			})
			continue
		}

		// Update session state
		lobby.Session.State = component.SessionStateInSession
		lobby.Session.PassthroughData = payload.PassthroughData
		lobbyEntity.Lobby.Set(lobby)

		state.SessionStartedEvents.Emit(SessionStartedEvent{
			LobbyID: lobbyID,
		})

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Msg("Session started")

		// Send to game shard if configured
		if config.GameShardID != "" {
			gameWorld := cardinal.OtherWorld{
				Region:       config.GameRegion,
				Organization: config.GameOrganization,
				Project:      config.GameProject,
				ShardID:      config.GameShardID,
			}
			gameWorld.Send(&state.BaseSystemState, NotifySessionStartCommand{
				Lobby:             lobby,
				LobbyShardAddress: config.LobbyShardAddress,
			})
			state.Logger().Info().
				Str("lobby_id", lobbyID).
				Str("game_shard", config.GameShardID).
				Msg("[CROSS-SHARD] Sent NotifySessionStartCommand to game shard")
		}
	}

	// Process EndSession commands
	for cmd := range state.EndSessionCmds.Iter() {
		payload := cmd.Payload()

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.LobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only end if in session
		if lobby.Session.State != component.SessionStateInSession {
			continue
		}

		lobby.Session.State = component.SessionStateIdle
		lobby.Session.PassthroughData = nil

		// Reset ready status for all players
		for i := range lobby.Teams {
			for j := range lobby.Teams[i].Players {
				lobby.Teams[i].Players[j].IsReady = false
			}
		}

		lobbyEntity.Lobby.Set(lobby)

		state.SessionEndedEvents.Emit(SessionEndedEvent{
			LobbyID: payload.LobbyID,
		})

		state.Logger().Info().
			Str("lobby_id", payload.LobbyID).
			Msg("Session ended")
	}

	// Process GenerateInviteCode commands
	for cmd := range state.GenerateInviteCodeCmds.Iter() {
		playerID := cmd.Persona()

		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
		if !exists {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only leader can generate
		if !lobby.IsLeader(playerID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "only leader can generate invite code",
			})
			continue
		}

		oldCode := lobby.InviteCode

		// Generate new invite code with collision check (max 3 retries)
		var newCode string
		newCodeValid := false
		for i := 0; i < 3; i++ {
			newCode = storedProvider.GenerateInviteCode(&lobby)
			// Check collision (but allow same code as current)
			existingLobby, exists := lobbyIndex.GetLobbyByInviteCode(newCode)
			if !exists || existingLobby == lobbyID {
				newCodeValid = true
				break
			}
		}
		if !newCodeValid {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				LobbyID: lobbyID,
				Error:   "invite code already exists",
			})
			continue
		}

		lobby.InviteCode = newCode
		lobbyEntity.Lobby.Set(lobby)

		lobbyIndex.UpdateInviteCode(lobbyID, oldCode, newCode)

		state.InviteCodeGeneratedEvents.Emit(InviteCodeGeneratedEvent{
			LobbyID:    lobbyID,
			InviteCode: newCode,
		})

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("invite_code", newCode).
			Msg("New invite code generated")
	}

	// Save lobby index
	if lobbyIndexEntity, ok := state.LobbyIndexes.GetByID(lobbyIndexEntityID); ok {
		lobbyIndexEntity.Index.Set(lobbyIndex)
	}

	return nil
}

// generateID generates a unique ID using UUID.
func generateID() string {
	return uuid.New().String()
}
