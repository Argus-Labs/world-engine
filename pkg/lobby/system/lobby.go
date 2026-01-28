package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
	"github.com/google/uuid"
)

// -----------------------------------------------------------------------------
// Commands
// -----------------------------------------------------------------------------

// CreateLobbyCommand creates a new lobby with the sender as leader.
type CreateLobbyCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
	// Teams is the initial team configuration for the lobby.
	Teams []TeamConfig `json:"teams,omitempty"`
	// GameWorld is the target game shard address for this lobby.
	GameWorld cardinal.OtherWorld `json:"game_world"`
	// PlayerPassthroughData is custom data for the creating player, forwarded to game shard.
	PlayerPassthroughData map[string]any `json:"player_passthrough_data,omitempty"`
	// SessionPassthroughData is custom data for the lobby session, forwarded to game shard.
	SessionPassthroughData map[string]any `json:"session_passthrough_data,omitempty"`
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
	RequestID  string `json:"request_id"`          // For matching request/response
	InviteCode string `json:"invite_code"`         // Required: invite code to join
	TeamName   string `json:"team_name,omitempty"` // Optional: team to join by name (joins first available if empty)
	// PlayerPassthroughData is custom data for the joining player, forwarded to game shard.
	PlayerPassthroughData map[string]any `json:"player_passthrough_data,omitempty"`
}

// Name returns the command name.
func (JoinLobbyCommand) Name() string { return "lobby_join" }

// JoinTeamCommand moves a player to a different team.
type JoinTeamCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
	TeamName  string `json:"team_name"`
}

// Name returns the command name.
func (JoinTeamCommand) Name() string { return "lobby_join_team" }

// LeaveLobbyCommand leaves the current lobby.
type LeaveLobbyCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
}

// Name returns the command name.
func (LeaveLobbyCommand) Name() string { return "lobby_leave" }

// SetReadyCommand sets the player's ready status.
type SetReadyCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
	IsReady   bool   `json:"is_ready"`
}

// Name returns the command name.
func (SetReadyCommand) Name() string { return "lobby_set_ready" }

// KickPlayerCommand kicks a player from the lobby (leader only).
type KickPlayerCommand struct {
	RequestID      string `json:"request_id"` // For matching request/response
	TargetPlayerID string `json:"target_player_id"`
}

// Name returns the command name.
func (KickPlayerCommand) Name() string { return "lobby_kick" }

// TransferLeaderCommand transfers leadership to another player.
type TransferLeaderCommand struct {
	RequestID      string `json:"request_id"` // For matching request/response
	TargetPlayerID string `json:"target_player_id"`
}

// Name returns the command name.
func (TransferLeaderCommand) Name() string { return "lobby_transfer_leader" }

// StartSessionCommand starts the session (leader only).
type StartSessionCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
}

// Name returns the command name.
func (StartSessionCommand) Name() string { return "lobby_start_session" }

// GenerateInviteCodeCommand generates a new invite code (leader only).
type GenerateInviteCodeCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
}

// Name returns the command name.
func (GenerateInviteCodeCommand) Name() string { return "lobby_generate_invite" }

// HeartbeatCommand is sent periodically by clients to indicate they're still connected.
// Players who don't send heartbeats within the timeout period are automatically removed.
type HeartbeatCommand struct {
}

// Name returns the command name.
func (HeartbeatCommand) Name() string { return "lobby_heartbeat" }

// UpdateSessionPassthroughCommand updates the session passthrough data (leader only).
type UpdateSessionPassthroughCommand struct {
	RequestID       string         `json:"request_id"` // For matching request/response
	PassthroughData map[string]any `json:"passthrough_data"`
}

// Name returns the command name.
func (UpdateSessionPassthroughCommand) Name() string { return "lobby_update_session_passthrough" }

// UpdatePlayerPassthroughCommand updates the player's own passthrough data.
type UpdatePlayerPassthroughCommand struct {
	RequestID       string         `json:"request_id"` // For matching request/response
	PassthroughData map[string]any `json:"passthrough_data"`
}

// Name returns the command name.
func (UpdatePlayerPassthroughCommand) Name() string { return "lobby_update_player_passthrough" }

// GetPlayerCommand fetches a specific player's component data.
type GetPlayerCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
	PlayerID  string `json:"player_id"`  // Target player ID (empty = self)
}

// Name returns the command name.
func (GetPlayerCommand) Name() string { return "lobby_get_player" }

// GetAllPlayersCommand fetches all players in the caller's lobby.
type GetAllPlayersCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
}

// Name returns the command name.
func (GetAllPlayersCommand) Name() string { return "lobby_get_all_players" }

// -----------------------------------------------------------------------------
// Events (Broadcast)
// -----------------------------------------------------------------------------

// LobbyCreatedEvent is emitted when a lobby is created.
type LobbyCreatedEvent struct {
	LobbyID    string `json:"lobby_id"`
	LeaderID   string `json:"leader_id"`
	InviteCode string `json:"invite_code"`
}

// Name returns the event name.
func (LobbyCreatedEvent) Name() string { return "lobby_created" }

// PlayerJoinedEvent is emitted when a player joins a lobby.
type PlayerJoinedEvent struct {
	LobbyID  string                    `json:"lobby_id"`
	TeamName string                    `json:"team_name"`
	Player   component.PlayerComponent `json:"player"`
}

// Name returns the event name.
func (PlayerJoinedEvent) Name() string { return "lobby_player_joined" }

// PlayerLeftEvent is emitted when a player leaves a lobby.
type PlayerLeftEvent struct {
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
}

// Name returns the event name.
func (PlayerLeftEvent) Name() string { return "lobby_player_left" }

// PlayerKickedEvent is emitted when a player is kicked.
type PlayerKickedEvent struct {
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
	KickerID string `json:"kicker_id"`
}

// Name returns the event name.
func (PlayerKickedEvent) Name() string { return "lobby_player_kicked" }

// PlayerReadyEvent is emitted when a player changes ready status.
type PlayerReadyEvent struct {
	LobbyID string                    `json:"lobby_id"`
	Player  component.PlayerComponent `json:"player"`
}

// Name returns the event name.
func (PlayerReadyEvent) Name() string { return "lobby_player_ready" }

// PlayerChangedTeamEvent is emitted when a player changes team.
type PlayerChangedTeamEvent struct {
	LobbyID     string                    `json:"lobby_id"`
	OldTeamName string                    `json:"old_team_name"`
	NewTeamName string                    `json:"new_team_name"`
	Player      component.PlayerComponent `json:"player"`
}

// Name returns the event name.
func (PlayerChangedTeamEvent) Name() string { return "lobby_player_changed_team" }

// LeaderChangedEvent is emitted when leadership is transferred.
type LeaderChangedEvent struct {
	LobbyID     string `json:"lobby_id"`
	OldLeaderID string `json:"old_leader_id"`
	NewLeaderID string `json:"new_leader_id"`
}

// Name returns the event name.
func (LeaderChangedEvent) Name() string { return "lobby_leader_changed" }

// SessionStartedEvent is emitted when a session starts.
type SessionStartedEvent struct {
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (SessionStartedEvent) Name() string { return "lobby_session_started" }

// SessionEndedEvent is emitted when a session ends.
type SessionEndedEvent struct {
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (SessionEndedEvent) Name() string { return "lobby_session_ended" }

// InviteCodeGeneratedEvent is emitted when a new invite code is generated.
type InviteCodeGeneratedEvent struct {
	LobbyID    string `json:"lobby_id"`
	InviteCode string `json:"invite_code"`
}

// Name returns the event name.
func (InviteCodeGeneratedEvent) Name() string { return "lobby_invite_generated" }

// LobbyDeletedEvent is emitted when a lobby is deleted.
type LobbyDeletedEvent struct {
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (LobbyDeletedEvent) Name() string { return "lobby_deleted" }

// PlayerTimedOutEvent is emitted when a player is removed due to missed heartbeats.
type PlayerTimedOutEvent struct {
	LobbyID  string `json:"lobby_id"`
	PlayerID string `json:"player_id"`
}

// Name returns the event name.
func (PlayerTimedOutEvent) Name() string { return "lobby_player_timed_out" }

// SessionPassthroughUpdatedEvent is emitted when session passthrough data is updated.
type SessionPassthroughUpdatedEvent struct {
	LobbyID         string         `json:"lobby_id"`
	PassthroughData map[string]any `json:"passthrough_data"`
}

// Name returns the event name.
func (SessionPassthroughUpdatedEvent) Name() string { return "lobby_session_passthrough_updated" }

// PlayerPassthroughUpdatedEvent is emitted when a player's passthrough data is updated.
type PlayerPassthroughUpdatedEvent struct {
	LobbyID string                    `json:"lobby_id"`
	Player  component.PlayerComponent `json:"player"`
}

// Name returns the event name.
func (PlayerPassthroughUpdatedEvent) Name() string { return "lobby_player_passthrough_updated" }

// -----------------------------------------------------------------------------
// CommandResult (Shard → Client, persona-prefixed)
// -----------------------------------------------------------------------------

// CreateLobbyResult is sent back to the client after CreateLobbyCommand.
type CreateLobbyResult struct {
	RequestID string                    `json:"request_id"`
	IsSuccess bool                      `json:"is_success"`
	Message   string                    `json:"message"`
	Lobby     component.LobbyComponent  `json:"lobby,omitempty"`
	Player    component.PlayerComponent `json:"player,omitempty"`
}

// Name returns the request-prefixed event name.
func (r CreateLobbyResult) Name() string { return r.RequestID + "_create_lobby_result" }

// JoinLobbyResult is sent back to the client after JoinLobbyCommand.
type JoinLobbyResult struct {
	RequestID   string                      `json:"request_id"`
	IsSuccess   bool                        `json:"is_success"`
	Message     string                      `json:"message"`
	Lobby       component.LobbyComponent    `json:"lobby,omitempty"`
	PlayersList []component.PlayerComponent `json:"players_list,omitempty"`
}

// Name returns the request-prefixed event name.
func (r JoinLobbyResult) Name() string { return r.RequestID + "_join_lobby_result" }

// JoinTeamResult is sent back to the client after JoinTeamCommand.
type JoinTeamResult struct {
	RequestID string                    `json:"request_id"`
	IsSuccess bool                      `json:"is_success"`
	Message   string                    `json:"message"`
	Player    component.PlayerComponent `json:"player,omitempty"`
}

// Name returns the request-prefixed event name.
func (r JoinTeamResult) Name() string { return r.RequestID + "_join_team_result" }

// LeaveLobbyResult is sent back to the client after LeaveLobbyCommand.
type LeaveLobbyResult struct {
	RequestID string `json:"request_id"`
	IsSuccess bool   `json:"is_success"`
	Message   string `json:"message"`
}

// Name returns the request-prefixed event name.
func (r LeaveLobbyResult) Name() string { return r.RequestID + "_leave_lobby_result" }

// SetReadyResult is sent back to the client after SetReadyCommand.
type SetReadyResult struct {
	RequestID string                    `json:"request_id"`
	IsSuccess bool                      `json:"is_success"`
	Message   string                    `json:"message"`
	Player    component.PlayerComponent `json:"player,omitempty"`
}

// Name returns the request-prefixed event name.
func (r SetReadyResult) Name() string { return r.RequestID + "_set_ready_result" }

// KickPlayerResult is sent back to the client after KickPlayerCommand.
type KickPlayerResult struct {
	RequestID string `json:"request_id"`
	IsSuccess bool   `json:"is_success"`
	Message   string `json:"message"`
}

// Name returns the request-prefixed event name.
func (r KickPlayerResult) Name() string { return r.RequestID + "_kick_player_result" }

// TransferLeaderResult is sent back to the client after TransferLeaderCommand.
type TransferLeaderResult struct {
	RequestID string `json:"request_id"`
	IsSuccess bool   `json:"is_success"`
	Message   string `json:"message"`
}

// Name returns the request-prefixed event name.
func (r TransferLeaderResult) Name() string { return r.RequestID + "_transfer_leader_result" }

// StartSessionResult is sent back to the client after StartSessionCommand.
type StartSessionResult struct {
	RequestID string `json:"request_id"`
	IsSuccess bool   `json:"is_success"`
	Message   string `json:"message"`
}

// Name returns the request-prefixed event name.
func (r StartSessionResult) Name() string { return r.RequestID + "_start_session_result" }

// GenerateInviteCodeResult is sent back to the client after GenerateInviteCodeCommand.
type GenerateInviteCodeResult struct {
	RequestID  string `json:"request_id"`
	IsSuccess  bool   `json:"is_success"`
	Message    string `json:"message"`
	InviteCode string `json:"invite_code,omitempty"`
}

// Name returns the request-prefixed event name.
func (r GenerateInviteCodeResult) Name() string { return r.RequestID + "_generate_invite_code_result" }

// UpdateSessionPassthroughResult is sent back to the client after UpdateSessionPassthroughCommand.
type UpdateSessionPassthroughResult struct {
	RequestID string `json:"request_id"`
	IsSuccess bool   `json:"is_success"`
	Message   string `json:"message"`
}

// Name returns the request-prefixed event name.
func (r UpdateSessionPassthroughResult) Name() string {
	return r.RequestID + "_update_session_passthrough_result"
}

// UpdatePlayerPassthroughResult is sent back to the client after UpdatePlayerPassthroughCommand.
type UpdatePlayerPassthroughResult struct {
	RequestID string                    `json:"request_id"`
	IsSuccess bool                      `json:"is_success"`
	Message   string                    `json:"message"`
	Player    component.PlayerComponent `json:"player,omitempty"`
}

// Name returns the request-prefixed event name.
func (r UpdatePlayerPassthroughResult) Name() string {
	return r.RequestID + "_update_player_passthrough_result"
}

// GetPlayerResult is sent back to the client after GetPlayerCommand.
type GetPlayerResult struct {
	RequestID string                    `json:"request_id"`
	IsSuccess bool                      `json:"is_success"`
	Message   string                    `json:"message"`
	Player    component.PlayerComponent `json:"player,omitempty"`
}

// Name returns the request-prefixed event name.
func (r GetPlayerResult) Name() string {
	return r.RequestID + "_get_player_result"
}

// GetAllPlayersResult is sent back to the client after GetAllPlayersCommand.
type GetAllPlayersResult struct {
	RequestID string                      `json:"request_id"`
	IsSuccess bool                        `json:"is_success"`
	Message   string                      `json:"message"`
	Players   []component.PlayerComponent `json:"players,omitempty"`
}

// Name returns the request-prefixed event name.
func (r GetAllPlayersResult) Name() string {
	return r.RequestID + "_get_all_players_result"
}

// -----------------------------------------------------------------------------
// Cross-Shard Commands
// -----------------------------------------------------------------------------

// NotifySessionStartCommand is sent to game shard when a session starts.
type NotifySessionStartCommand struct {
	Lobby      component.LobbyComponent `json:"lobby"`
	LobbyWorld cardinal.OtherWorld      `json:"lobby_world"`
}

// Name returns the command name.
func (NotifySessionStartCommand) Name() string { return "lobby_notify_session_start" }

// NotifySessionEndCommand is sent from game shard to lobby when session ends.
type NotifySessionEndCommand struct {
	LobbyID string `json:"lobby_id"`
}

// Name returns the command name.
func (NotifySessionEndCommand) Name() string { return "lobby_notify_session_end" }

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
	for i := range 6 {
		// Use each hex byte to index into charset
		idx := int(hexStr[i]) % len(inviteCodeCharset)
		code[i] = inviteCodeCharset[idx]
	}
	return string(code)
}

// storedProvider holds the provider set by the Register function.
//
//nolint:gochecknoglobals // set once at initialization, read-only thereafter
var storedProvider LobbyProvider = DefaultProvider{}

// SetProvider stores the provider for the system to use.
func SetProvider(provider LobbyProvider) {
	if provider != nil {
		storedProvider = provider
	}
}

// storedConfig holds the configuration set by the Register function.
//
//nolint:gochecknoglobals // set once at initialization, read-only thereafter
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

// InitSystem creates singleton index entities. Runs once at tick 0.
func InitSystem(state *InitSystemState) {
	// Create lobby index entity
	_, lobbyIdx := state.LobbyIndexes.Create()
	idx := component.LobbyIndexComponent{}
	idx.Init()
	lobbyIdx.Index.Set(idx)
	state.Logger().Info().Msg("Created lobby index entity")

	// Create config entity
	_, cfg := state.Configs.Create()
	cfg.Config.Set(storedConfig)
	state.Logger().Info().Msg("Created lobby config entity")
}

// -----------------------------------------------------------------------------
// Lobby System
// -----------------------------------------------------------------------------

// LobbySystemState is the state for the lobby system.
type LobbySystemState struct {
	cardinal.BaseSystemState

	// Commands
	CreateLobbyCmds              cardinal.WithCommand[CreateLobbyCommand]
	JoinLobbyCmds                cardinal.WithCommand[JoinLobbyCommand]
	JoinTeamCmds                 cardinal.WithCommand[JoinTeamCommand]
	LeaveLobbyCmds               cardinal.WithCommand[LeaveLobbyCommand]
	SetReadyCmds                 cardinal.WithCommand[SetReadyCommand]
	KickPlayerCmds               cardinal.WithCommand[KickPlayerCommand]
	TransferLeaderCmds           cardinal.WithCommand[TransferLeaderCommand]
	StartSessionCmds             cardinal.WithCommand[StartSessionCommand]
	NotifySessionEndCmds         cardinal.WithCommand[NotifySessionEndCommand]
	GenerateInviteCodeCmds       cardinal.WithCommand[GenerateInviteCodeCommand]
	UpdateSessionPassthroughCmds cardinal.WithCommand[UpdateSessionPassthroughCommand]
	UpdatePlayerPassthroughCmds  cardinal.WithCommand[UpdatePlayerPassthroughCommand]
	GetPlayerCmds                cardinal.WithCommand[GetPlayerCommand]
	GetAllPlayersCmds            cardinal.WithCommand[GetAllPlayersCommand]

	// Entities
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}]

	Players cardinal.Contains[struct {
		Player cardinal.Ref[component.PlayerComponent]
	}]

	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events (Broadcast)
	LobbyCreatedEvents              cardinal.WithEvent[LobbyCreatedEvent]
	PlayerJoinedEvents              cardinal.WithEvent[PlayerJoinedEvent]
	PlayerLeftEvents                cardinal.WithEvent[PlayerLeftEvent]
	PlayerKickedEvents              cardinal.WithEvent[PlayerKickedEvent]
	PlayerReadyEvents               cardinal.WithEvent[PlayerReadyEvent]
	PlayerChangedTeamEvents         cardinal.WithEvent[PlayerChangedTeamEvent]
	LeaderChangedEvents             cardinal.WithEvent[LeaderChangedEvent]
	SessionStartedEvents            cardinal.WithEvent[SessionStartedEvent]
	SessionEndedEvents              cardinal.WithEvent[SessionEndedEvent]
	InviteCodeGeneratedEvents       cardinal.WithEvent[InviteCodeGeneratedEvent]
	LobbyDeletedEvents              cardinal.WithEvent[LobbyDeletedEvent]
	SessionPassthroughUpdatedEvents cardinal.WithEvent[SessionPassthroughUpdatedEvent]
	PlayerPassthroughUpdatedEvents  cardinal.WithEvent[PlayerPassthroughUpdatedEvent]

	// CommandResult (request-prefixed responses)
	CreateLobbyResults              cardinal.WithEvent[CreateLobbyResult]
	JoinLobbyResults                cardinal.WithEvent[JoinLobbyResult]
	JoinTeamResults                 cardinal.WithEvent[JoinTeamResult]
	LeaveLobbyResults               cardinal.WithEvent[LeaveLobbyResult]
	SetReadyResults                 cardinal.WithEvent[SetReadyResult]
	KickPlayerResults               cardinal.WithEvent[KickPlayerResult]
	TransferLeaderResults           cardinal.WithEvent[TransferLeaderResult]
	StartSessionResults             cardinal.WithEvent[StartSessionResult]
	GenerateInviteCodeResults       cardinal.WithEvent[GenerateInviteCodeResult]
	UpdateSessionPassthroughResults cardinal.WithEvent[UpdateSessionPassthroughResult]
	UpdatePlayerPassthroughResults  cardinal.WithEvent[UpdatePlayerPassthroughResult]
	GetPlayerResults                cardinal.WithEvent[GetPlayerResult]
	GetAllPlayersResults            cardinal.WithEvent[GetAllPlayersResult]
}

// lobbyLookupResult holds the result of looking up a player's lobby.
type lobbyLookupResult struct {
	lobbyID  string
	entityID cardinal.EntityID
	lobby    component.LobbyComponent
	lobbyRef cardinal.Ref[component.LobbyComponent]
}

// getPlayerLobby looks up the lobby for a player and returns all relevant data.
// Returns nil if the player is not in a lobby or the lobby doesn't exist.
func getPlayerLobby(
	playerID string,
	lobbyIndex *component.LobbyIndexComponent,
	lobbies *cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}],
) *lobbyLookupResult {
	lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)
	if !exists {
		return nil
	}

	lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
	if !exists {
		return nil
	}

	lobbyEntity, err := lobbies.GetByID(cardinal.EntityID(lobbyEntityID))
	if err != nil {
		return nil
	}

	return &lobbyLookupResult{
		lobbyID:  lobbyID,
		entityID: cardinal.EntityID(lobbyEntityID),
		lobby:    lobbyEntity.Lobby.Get(),
		lobbyRef: lobbyEntity.Lobby,
	}
}

// LobbySystem processes lobby commands.
func LobbySystem(state *LobbySystemState) {
	now := state.Timestamp().Unix()

	// Get lobby index
	var lobbyIndex component.LobbyIndexComponent
	var lobbyIndexEntityID cardinal.EntityID
	for entityID, idx := range state.LobbyIndexes.Iter() {
		lobbyIndex = idx.Index.Get()
		lobbyIndexEntityID = entityID
		break
	}

	// Get config
	var config component.ConfigComponent
	for _, cfg := range state.Configs.Iter() {
		config = cfg.Config.Get()
		break
	}

	// Get timeout for deadline
	timeout := config.HeartbeatTimeout
	if timeout <= 0 {
		timeout = 30 // default 30 seconds
	}

	// Process all commands
	processCreateLobbyCommands(state, &lobbyIndex, now, timeout)
	processJoinLobbyCommands(state, &lobbyIndex, now, timeout)
	processJoinTeamCommands(state, &lobbyIndex)
	processLeaveLobbyCommands(state, &lobbyIndex)
	processSetReadyCommands(state, &lobbyIndex)
	processKickPlayerCommands(state, &lobbyIndex)
	processTransferLeaderCommands(state, &lobbyIndex)
	processStartSessionCommands(state, &lobbyIndex, &config)
	processNotifySessionEndCommands(state, &lobbyIndex)
	processGenerateInviteCodeCommands(state, &lobbyIndex)
	processUpdateSessionPassthroughCommands(state, &lobbyIndex)
	processUpdatePlayerPassthroughCommands(state, &lobbyIndex)
	processGetPlayerCommands(state, &lobbyIndex)
	processGetAllPlayersCommands(state, &lobbyIndex)

	// Save lobby index
	if lobbyIndexEntity, err := state.LobbyIndexes.GetByID(lobbyIndexEntityID); err == nil {
		lobbyIndexEntity.Index.Set(lobbyIndex)
	}
}

// timedOutPlayer holds info about a player who missed heartbeat deadline.
type timedOutPlayer struct {
	playerID       string
	lobbyID        string
	teamID         string
	playerEntityID uint32
}

// findTimedOutPlayers returns all players whose deadline has passed.
func findTimedOutPlayers(lobbyIndex *component.LobbyIndexComponent, now int64) []timedOutPlayer {
	var result []timedOutPlayer
	for playerID, deadline := range lobbyIndex.PlayerDeadline {
		if now >= deadline {
			result = append(result, timedOutPlayer{
				playerID:       playerID,
				lobbyID:        lobbyIndex.PlayerToLobby[playerID],
				teamID:         lobbyIndex.PlayerToTeam[playerID],
				playerEntityID: lobbyIndex.PlayerToEntity[playerID],
			})
		}
	}
	return result
}

// groupPlayersByLobby groups timed out players by their lobby ID.
func groupPlayersByLobby(players []timedOutPlayer) map[string][]timedOutPlayer {
	result := make(map[string][]timedOutPlayer)
	for _, p := range players {
		result[p.lobbyID] = append(result[p.lobbyID], p)
	}
	return result
}

// findNewLeader finds the first remaining player in a lobby to be leader.
// Returns empty string if no players remain.
func findNewLeader(lobby *component.LobbyComponent) string {
	for _, team := range lobby.Teams {
		if len(team.PlayerIDs) > 0 {
			return team.PlayerIDs[0]
		}
	}
	return ""
}

// isLeaderInList checks if the lobby leader is in the timed out players list.
func isLeaderInList(leaderID string, players []timedOutPlayer) bool {
	for _, p := range players {
		if p.playerID == leaderID {
			return true
		}
	}
	return false
}

// emitJoinLobbyFailure emits a failure result for JoinLobby command.
func emitJoinLobbyFailure(state *LobbySystemState, requestID, message string) {
	state.JoinLobbyResults.Emit(JoinLobbyResult{
		RequestID: requestID,
		IsSuccess: false,
		Message:   message,
	})
}

// createPlayerEntity creates a player entity and returns the component and entity ID.
func createPlayerEntity(
	state *LobbySystemState,
	playerID, lobbyID, teamID string,
	passthroughData map[string]any,
	now int64,
) (component.PlayerComponent, cardinal.EntityID) {
	playerComp := component.PlayerComponent{
		PlayerID:        playerID,
		LobbyID:         lobbyID,
		TeamID:          teamID,
		IsReady:         false,
		PassthroughData: passthroughData,
		JoinedAt:        now,
	}
	playerEntityID, playerEntity := state.Players.Create()
	playerEntity.Player.Set(playerComp)
	return playerComp, playerEntityID
}

// lobbyToDestroy holds info about a lobby to be destroyed.
type lobbyToDestroy struct {
	entityID cardinal.EntityID
	lobbyID  string
}

// processTimedOutLobby handles removing timed out players from a single lobby.
// Returns player entity IDs to destroy and lobby to destroy (if empty).
func processTimedOutLobby(
	state *HeartbeatSystemState,
	lobbyIndex *component.LobbyIndexComponent,
	lobbyID string,
	players []timedOutPlayer,
) ([]cardinal.EntityID, *lobbyToDestroy) {
	var playerEntities []cardinal.EntityID
	lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
	if !exists {
		return nil, nil
	}

	lobbyEntity, err := state.Lobbies.GetByID(cardinal.EntityID(lobbyEntityID))
	if err != nil {
		return nil, nil
	}

	lobby := lobbyEntity.Lobby.Get()

	// Remove each timed out player
	for _, p := range players {
		lobby.RemovePlayerFromTeam(p.playerID, p.teamID)
		lobbyIndex.RemovePlayerFromLobby(p.playerID)
		playerEntities = append(playerEntities, cardinal.EntityID(p.playerEntityID))

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", p.playerID).
			Msg("Player timed out due to missed heartbeats")

		state.PlayerTimedOutEvents.Emit(PlayerTimedOutEvent{LobbyID: lobbyID, PlayerID: p.playerID})
		state.PlayerLeftEvents.Emit(PlayerLeftEvent{LobbyID: lobbyID, PlayerID: p.playerID})
	}

	// Check if lobby is empty
	if lobbyIndex.GetLobbyPlayerCount(lobbyID) == 0 {
		lobbyIndex.RemoveLobby(lobbyID, lobby.InviteCode)
		state.Logger().Info().Str("lobby_id", lobbyID).Msg("Lobby marked for deletion (empty after timeout)")
		state.LobbyDeletedEvents.Emit(LobbyDeletedEvent{LobbyID: lobbyID})
		return playerEntities, &lobbyToDestroy{entityID: cardinal.EntityID(lobbyEntityID), lobbyID: lobbyID}
	}

	// Handle leader timeout
	if isLeaderInList(lobby.LeaderID, players) {
		oldLeaderID := lobby.LeaderID
		lobby.LeaderID = findNewLeader(&lobby)
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("old_leader", oldLeaderID).
			Str("new_leader", lobby.LeaderID).
			Msg("Leadership auto-transferred after timeout")
		state.LeaderChangedEvents.Emit(LeaderChangedEvent{
			LobbyID: lobbyID, OldLeaderID: oldLeaderID, NewLeaderID: lobby.LeaderID,
		})
	}

	lobbyEntity.Lobby.Set(lobby)
	return playerEntities, nil
}

// processHeartbeatCommands updates deadlines for players who sent heartbeats.
func processHeartbeatCommands(
	state *HeartbeatSystemState,
	lobbyIndex *component.LobbyIndexComponent,
	now, timeout int64,
) {
	for cmd := range state.HeartbeatCmds.Iter() {
		playerID := cmd.Persona
		lobbyID, exists := lobbyIndex.GetPlayerLobby(playerID)

		state.Logger().Debug().
			Str("player_id", playerID).
			Str("lobby_id", lobbyID).
			Bool("in_lobby", exists).
			Msg("Heartbeat command received")

		if exists {
			lobbyIndex.UpdatePlayerDeadline(playerID, now+timeout)
		}
	}
}

// validateUniqueTeamNames checks for duplicate team names in config.
// Returns the duplicate name if found, empty string otherwise.
func validateUniqueTeamNames(teams []TeamConfig) string {
	teamNames := make(map[string]bool)
	for _, tc := range teams {
		if teamNames[tc.Name] {
			return tc.Name
		}
		teamNames[tc.Name] = true
	}
	return ""
}

// generateInviteCodeWithRetry generates an invite code with collision check.
// Retries up to maxRetries times if collision detected.
// Returns the code and whether generation succeeded.
func generateInviteCodeWithRetry(
	lobbyIndex *component.LobbyIndexComponent,
	lobby *component.LobbyComponent,
	maxRetries int,
) (string, bool) {
	for range maxRetries {
		code := storedProvider.GenerateInviteCode(lobby)
		if _, exists := lobbyIndex.GetLobbyByInviteCode(code); !exists {
			return code, true
		}
	}
	return "", false
}

// areAllPlayersReady checks if all players in a lobby are ready.
// Returns false if lobby has no players or any player is not ready.
func areAllPlayersReady(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	lobby *component.LobbyComponent,
) bool {
	playerIDs := lobby.GetAllPlayerIDs()
	if len(playerIDs) == 0 {
		return false
	}
	for _, pid := range playerIDs {
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(pid)
		if !exists {
			return false
		}
		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			return false
		}
		if !playerEntity.Player.Get().IsReady {
			return false
		}
	}
	return true
}

// gatherLobbyPlayers collects all PlayerComponent data for players in a lobby.
// Used to include player list in command results.
func gatherLobbyPlayers(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	lobby *component.LobbyComponent,
) []component.PlayerComponent {
	var playersList []component.PlayerComponent
	for _, pid := range lobby.GetAllPlayerIDs() {
		pEntityID, pExists := lobbyIndex.GetPlayerEntityID(pid)
		if !pExists {
			continue
		}
		pEntity, pErr := state.Players.GetByID(cardinal.EntityID(pEntityID))
		if pErr != nil {
			continue
		}
		playersList = append(playersList, pEntity.Player.Get())
	}
	return playersList
}

// findTargetTeam finds the team for a player to join.
// If teamName is provided, it finds that specific team.
// Otherwise, it finds the first team with available space.
// Returns the team and an error message (empty string if successful).
func findTargetTeam(lobby *component.LobbyComponent, teamName string) (*component.Team, string) {
	if teamName != "" {
		team := lobby.GetTeamByName(teamName)
		if team == nil {
			return nil, "team not found"
		}
		if team.IsFull() {
			return nil, "team is full"
		}
		return team, ""
	}

	// Find first available team with space
	for i := range lobby.Teams {
		if !lobby.Teams[i].IsFull() {
			return &lobby.Teams[i], ""
		}
	}
	return nil, "all teams are full"
}

func processCreateLobbyCommands(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	now, timeout int64,
) {
	for cmd := range state.CreateLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		// Check if player is already in a lobby
		if _, exists := lobbyIndex.GetPlayerLobby(playerID); exists {
			state.Logger().Warn().Str("player_id", playerID).Msg("player already in a lobby")
			state.CreateLobbyResults.Emit(CreateLobbyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player already in a lobby",
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
			GameWorld:  payload.GameWorld,
			Session: component.Session{
				State:           component.SessionStateIdle,
				PassthroughData: payload.SessionPassthroughData,
			},
			CreatedAt: now,
		}

		// Create teams from config or default single team
		if len(payload.Teams) > 0 {
			// Validate unique team names
			if duplicateName := validateUniqueTeamNames(payload.Teams); duplicateName != "" {
				state.Logger().Warn().Str("team_name", duplicateName).Msg("duplicate team name")
				state.CreateLobbyResults.Emit(CreateLobbyResult{
					RequestID: payload.RequestID,
					IsSuccess: false,
					Message:   "duplicate team name: " + duplicateName,
				})
				continue
			}

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

		// Generate invite code with collision check
		inviteCode, ok := generateInviteCodeWithRetry(lobbyIndex, &lobby, 3)
		if !ok {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("invite code collision after retries")
			state.CreateLobbyResults.Emit(CreateLobbyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "invite code collision",
			})
			continue
		}
		lobby.InviteCode = inviteCode

		// Add leader to first team (just the ID)
		lobby.Teams[0].PlayerIDs = append(lobby.Teams[0].PlayerIDs, playerID)

		// Create lobby entity
		lobbyEntityID, lobbyEntity := state.Lobbies.Create()
		lobbyEntity.Lobby.Set(lobby)

		// Create player entity and update index
		playerComp, playerEntityID := createPlayerEntity(
			state, playerID, lobbyID, lobby.Teams[0].TeamID, payload.PlayerPassthroughData, now,
		)
		lobbyIndex.AddLobby(lobbyID, uint32(lobbyEntityID), inviteCode)
		lobbyIndex.AddPlayerToLobby(playerID, lobbyID, lobby.Teams[0].TeamID, uint32(playerEntityID), now+timeout)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("leader_id", playerID).
			Msg("Lobby created")

		// Emit broadcast event
		state.LobbyCreatedEvents.Emit(LobbyCreatedEvent{
			LobbyID:    lobbyID,
			LeaderID:   playerID,
			InviteCode: inviteCode,
		})

		// Emit success result
		state.CreateLobbyResults.Emit(CreateLobbyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "lobby created",
			Lobby:     lobby,
			Player:    playerComp,
		})
	}
}

func processJoinLobbyCommands(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	now, timeout int64,
) {
	for cmd := range state.JoinLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		// Check if player is already in a lobby
		if _, exists := lobbyIndex.GetPlayerLobby(playerID); exists {
			state.Logger().Warn().Str("player_id", playerID).Msg("player already in a lobby")
			emitJoinLobbyFailure(state, payload.RequestID, "player already in a lobby")
			continue
		}

		// Find lobby by invite code
		lobbyID, exists := lobbyIndex.GetLobbyByInviteCode(payload.InviteCode)
		if !exists {
			state.Logger().Warn().Str("invite_code", payload.InviteCode).Msg("invalid invite code")
			emitJoinLobbyFailure(state, payload.RequestID, "invalid invite code")
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
		if !exists {
			emitJoinLobbyFailure(state, payload.RequestID, "lobby not found")
			continue
		}

		lobbyEntity, err := state.Lobbies.GetByID(cardinal.EntityID(lobbyEntityID))
		if err != nil {
			emitJoinLobbyFailure(state, payload.RequestID, "lobby not found")
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Check if lobby is in session
		if lobby.Session.State == component.SessionStateInSession {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("lobby is in session")
			emitJoinLobbyFailure(state, payload.RequestID, "lobby is in session")
			continue
		}

		// Find target team
		targetTeam, errMsg := findTargetTeam(&lobby, payload.TeamName)
		if targetTeam == nil {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("team_name", payload.TeamName).Msg(errMsg)
			emitJoinLobbyFailure(state, payload.RequestID, errMsg)
			continue
		}

		// Add player ID to team
		if !lobby.AddPlayerToTeam(playerID, targetTeam.TeamID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("failed to join team")
			emitJoinLobbyFailure(state, payload.RequestID, "failed to join team")
			continue
		}

		lobbyEntity.Lobby.Set(lobby)

		// Create player entity
		playerComp, playerEntityID := createPlayerEntity(
			state, playerID, lobbyID, targetTeam.TeamID, payload.PlayerPassthroughData, now,
		)
		lobbyIndex.AddPlayerToLobby(playerID, lobbyID, targetTeam.TeamID, uint32(playerEntityID), now+timeout)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("team_name", targetTeam.Name).
			Msg("Player joined lobby")

		// Emit broadcast event
		state.PlayerJoinedEvents.Emit(PlayerJoinedEvent{
			LobbyID:  lobbyID,
			TeamName: targetTeam.Name,
			Player:   playerComp,
		})

		// Gather all players in the lobby for the result
		playersList := gatherLobbyPlayers(state, lobbyIndex, &lobby)

		// Emit success result
		state.JoinLobbyResults.Emit(JoinLobbyResult{
			RequestID:   payload.RequestID,
			IsSuccess:   true,
			Message:     "joined lobby",
			Lobby:       lobby,
			PlayersList: playersList,
		})
	}
}

func processJoinTeamCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.JoinTeamCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.JoinTeamResults.Emit(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Can't change team during session
		if lobby.Session.State == component.SessionStateInSession {
			state.JoinTeamResults.Emit(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "cannot change team during session",
			})
			continue
		}

		// Get current team
		oldTeam := lobby.GetPlayerTeam(playerID)
		if oldTeam == nil {
			state.JoinTeamResults.Emit(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in any team",
			})
			continue
		}
		oldTeamName := oldTeam.Name

		// Find target team by name
		newTeam := lobby.GetTeamByName(payload.TeamName)
		if newTeam == nil {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("team_name", payload.TeamName).Msg("team not found")
			state.JoinTeamResults.Emit(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "team not found",
			})
			continue
		}

		// Move to new team
		if !lobby.MovePlayerToTeam(playerID, newTeam.TeamID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("failed to change team")
			state.JoinTeamResults.Emit(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "failed to change team (team may be full)",
			})
			continue
		}

		result.lobbyRef.Set(lobby)

		// Update player entity's TeamID and index
		lobbyIndex.UpdatePlayerTeam(playerID, newTeam.TeamID)
		var playerComp component.PlayerComponent
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(playerID)
		if exists {
			if playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID)); err == nil {
				playerComp = playerEntity.Player.Get()
				playerComp.TeamID = newTeam.TeamID
				playerEntity.Player.Set(playerComp)
			}
		}

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("old_team", oldTeamName).
			Str("new_team", newTeam.Name).
			Msg("Player changed team")

		// Emit broadcast event
		state.PlayerChangedTeamEvents.Emit(PlayerChangedTeamEvent{
			LobbyID:     lobbyID,
			OldTeamName: oldTeamName,
			NewTeamName: newTeam.Name,
			Player:      playerComp,
		})

		state.JoinTeamResults.Emit(JoinTeamResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "changed team",
			Player:    playerComp,
		})
	}
}

func processLeaveLobbyCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.LeaveLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.LeaveLobbyResults.Emit(LeaveLobbyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Delete player entity
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(playerID)
		if exists {
			state.Players.Destroy(cardinal.EntityID(playerEntityID))
		}

		// Remove player from lobby - use index for O(1) team lookup, then O(players in team) removal
		teamID, _ := lobbyIndex.GetPlayerTeam(playerID)
		lobby.RemovePlayerFromTeam(playerID, teamID)
		lobbyIndex.RemovePlayerFromLobby(playerID)

		// Emit broadcast event for player leaving
		state.PlayerLeftEvents.Emit(PlayerLeftEvent{
			LobbyID:  lobbyID,
			PlayerID: playerID,
		})

		// If lobby is empty, delete it - use index for O(1) check
		if lobbyIndex.GetLobbyPlayerCount(lobbyID) == 0 {
			lobbyIndex.RemoveLobby(lobbyID, lobby.InviteCode)
			state.Lobbies.Destroy(result.entityID)

			state.Logger().Info().
				Str("lobby_id", lobbyID).
				Msg("Lobby deleted (empty)")

			// Emit broadcast event for lobby deletion
			state.LobbyDeletedEvents.Emit(LobbyDeletedEvent{
				LobbyID: lobbyID,
			})
		} else {
			// Transfer leadership if leader left
			if lobby.LeaderID == playerID {
				oldLeaderID := lobby.LeaderID
				// Find first player ID in any team
				for _, team := range lobby.Teams {
					if len(team.PlayerIDs) > 0 {
						lobby.LeaderID = team.PlayerIDs[0]
						break
					}
				}

				state.Logger().Info().
					Str("lobby_id", lobbyID).
					Str("old_leader", oldLeaderID).
					Str("new_leader", lobby.LeaderID).
					Msg("Leadership auto-transferred")

				// Emit broadcast event for leader change
				state.LeaderChangedEvents.Emit(LeaderChangedEvent{
					LobbyID:     lobbyID,
					OldLeaderID: oldLeaderID,
					NewLeaderID: lobby.LeaderID,
				})
			}

			result.lobbyRef.Set(lobby)
		}

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Msg("Player left lobby")

		state.LeaveLobbyResults.Emit(LeaveLobbyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "left lobby",
		})
	}
}

func processSetReadyCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.SetReadyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.SetReadyResults.Emit(SetReadyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Can't change ready during session
		if lobby.Session.State == component.SessionStateInSession {
			state.SetReadyResults.Emit(SetReadyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "cannot change ready status during session",
			})
			continue
		}

		// Update player entity's IsReady
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(playerID)
		if !exists {
			state.SetReadyResults.Emit(SetReadyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}
		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			state.SetReadyResults.Emit(SetReadyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}
		playerComp := playerEntity.Player.Get()
		playerComp.IsReady = payload.IsReady
		playerEntity.Player.Set(playerComp)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Bool("is_ready", payload.IsReady).
			Msg("Player ready status changed")

		// Emit broadcast event
		state.PlayerReadyEvents.Emit(PlayerReadyEvent{
			LobbyID: lobbyID,
			Player:  playerComp,
		})

		state.SetReadyResults.Emit(SetReadyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "ready status updated",
			Player:    playerComp,
		})
	}
}

func processKickPlayerCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.KickPlayerCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.KickPlayerResults.Emit(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Only leader can kick
		if !lobby.IsLeader(playerID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("player_id", playerID).Msg("only leader can kick players")
			state.KickPlayerResults.Emit(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can kick players",
			})
			continue
		}

		// Can't kick self
		if payload.TargetPlayerID == playerID {
			state.KickPlayerResults.Emit(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "cannot kick yourself",
			})
			continue
		}

		// Check if target is in lobby
		if !lobby.HasPlayer(payload.TargetPlayerID) {
			state.KickPlayerResults.Emit(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "target player not in lobby",
			})
			continue
		}

		// Delete player entity
		targetPlayerEntityID, exists := lobbyIndex.GetPlayerEntityID(payload.TargetPlayerID)
		if exists {
			state.Players.Destroy(cardinal.EntityID(targetPlayerEntityID))
		}

		// Remove player from lobby - use index for O(1) team lookup, then O(players in team) removal
		targetTeamID, _ := lobbyIndex.GetPlayerTeam(payload.TargetPlayerID)
		lobby.RemovePlayerFromTeam(payload.TargetPlayerID, targetTeamID)
		result.lobbyRef.Set(lobby)
		lobbyIndex.RemovePlayerFromLobby(payload.TargetPlayerID)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", payload.TargetPlayerID).
			Str("kicker_id", playerID).
			Msg("Player kicked from lobby")

		// Emit broadcast event
		state.PlayerKickedEvents.Emit(PlayerKickedEvent{
			LobbyID:  lobbyID,
			PlayerID: payload.TargetPlayerID,
			KickerID: playerID,
		})

		state.KickPlayerResults.Emit(KickPlayerResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "player kicked",
		})
	}
}

func processTransferLeaderCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.TransferLeaderCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.TransferLeaderResults.Emit(TransferLeaderResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Only leader can transfer
		if !lobby.IsLeader(playerID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("player_id", playerID).Msg("only leader can transfer leadership")
			state.TransferLeaderResults.Emit(TransferLeaderResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can transfer leadership",
			})
			continue
		}

		// Check if target is in lobby
		if !lobby.HasPlayer(payload.TargetPlayerID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("target", payload.TargetPlayerID).
				Msg("target player not in lobby")
			state.TransferLeaderResults.Emit(TransferLeaderResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "target player not in lobby",
			})
			continue
		}

		oldLeaderID := lobby.LeaderID
		lobby.LeaderID = payload.TargetPlayerID
		result.lobbyRef.Set(lobby)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("old_leader", oldLeaderID).
			Str("new_leader", payload.TargetPlayerID).
			Msg("Leadership transferred")

		// Emit broadcast event
		state.LeaderChangedEvents.Emit(LeaderChangedEvent{
			LobbyID:     lobbyID,
			OldLeaderID: oldLeaderID,
			NewLeaderID: payload.TargetPlayerID,
		})

		state.TransferLeaderResults.Emit(TransferLeaderResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "leadership transferred",
		})
	}
}

func processStartSessionCommands(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	config *component.ConfigComponent,
) {
	for cmd := range state.StartSessionCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.StartSessionResults.Emit(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Only leader can start
		if !lobby.IsLeader(playerID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("player_id", playerID).Msg("only leader can start session")
			state.StartSessionResults.Emit(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can start session",
			})
			continue
		}

		// Already in session
		if lobby.Session.State == component.SessionStateInSession {
			state.StartSessionResults.Emit(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "session already in progress",
			})
			continue
		}

		// Check all ready
		if !areAllPlayersReady(state, lobbyIndex, &lobby) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("not all players are ready")
			state.StartSessionResults.Emit(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "not all players are ready",
			})
			continue
		}

		// Update session state
		lobby.Session.State = component.SessionStateInSession
		result.lobbyRef.Set(lobby)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Msg("Session started")

		// Emit broadcast event
		state.SessionStartedEvents.Emit(SessionStartedEvent{
			LobbyID: lobbyID,
		})

		// Send to game shard if GameWorld is configured on the lobby
		if lobby.GameWorld.ShardID != "" {
			gameWorld := cardinal.OtherWorld{
				Region:       lobby.GameWorld.Region,
				Organization: lobby.GameWorld.Organization,
				Project:      lobby.GameWorld.Project,
				ShardID:      lobby.GameWorld.ShardID,
			}
			lobbyWorld := cardinal.OtherWorld{
				Region:       config.LobbyWorld.Region,
				Organization: config.LobbyWorld.Organization,
				Project:      config.LobbyWorld.Project,
				ShardID:      config.LobbyWorld.ShardID,
			}
			gameWorld.SendCommand(&state.BaseSystemState, NotifySessionStartCommand{
				Lobby:      lobby,
				LobbyWorld: lobbyWorld,
			})
			state.Logger().Info().
				Str("lobby_id", lobbyID).
				Str("game_shard", lobby.GameWorld.ShardID).
				Msg("[CROSS-SHARD] Sent NotifySessionStartCommand to game shard")
		}

		state.StartSessionResults.Emit(StartSessionResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "session started",
		})
	}
}

func processNotifySessionEndCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.NotifySessionEndCmds.Iter() {
		payload := cmd.Payload

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.LobbyID)
		if !exists {
			continue
		}

		lobbyEntity, err := state.Lobbies.GetByID(cardinal.EntityID(lobbyEntityID))
		if err != nil {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only end if in session
		if lobby.Session.State != component.SessionStateInSession {
			continue
		}

		lobby.Session.State = component.SessionStateIdle
		lobby.Session.PassthroughData = nil
		lobbyEntity.Lobby.Set(lobby)

		// Reset ready status for all player entities
		for _, pid := range lobby.GetAllPlayerIDs() {
			playerEntityID, pExists := lobbyIndex.GetPlayerEntityID(pid)
			if !pExists {
				continue
			}
			playerEntity, pErr := state.Players.GetByID(cardinal.EntityID(playerEntityID))
			if pErr != nil {
				continue
			}
			playerComp := playerEntity.Player.Get()
			playerComp.IsReady = false
			playerEntity.Player.Set(playerComp)
		}

		state.Logger().Info().
			Str("lobby_id", payload.LobbyID).
			Msg("Session ended")

		// Emit broadcast event
		state.SessionEndedEvents.Emit(SessionEndedEvent{
			LobbyID: payload.LobbyID,
		})
	}
}

func processGenerateInviteCodeCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.GenerateInviteCodeCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.GenerateInviteCodeResults.Emit(GenerateInviteCodeResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Only leader can generate
		if !lobby.IsLeader(playerID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("player_id", playerID).Msg("only leader can generate invite code")
			state.GenerateInviteCodeResults.Emit(GenerateInviteCodeResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can generate invite code",
			})
			continue
		}

		oldCode := lobby.InviteCode

		// Generate new invite code with collision check (max 3 retries)
		var newCode string
		newCodeValid := false
		for range 3 {
			newCode = storedProvider.GenerateInviteCode(&lobby)
			// Check collision (but allow same code as current)
			existingLobby, exists := lobbyIndex.GetLobbyByInviteCode(newCode)
			if !exists || existingLobby == lobbyID {
				newCodeValid = true
				break
			}
		}
		if !newCodeValid {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("invite code collision after retries")
			state.GenerateInviteCodeResults.Emit(GenerateInviteCodeResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "invite code collision",
			})
			continue
		}

		lobby.InviteCode = newCode
		result.lobbyRef.Set(lobby)

		lobbyIndex.UpdateInviteCode(lobbyID, oldCode, newCode)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("invite_code", newCode).
			Msg("New invite code generated")

		// Emit broadcast event
		state.InviteCodeGeneratedEvents.Emit(InviteCodeGeneratedEvent{
			LobbyID:    lobbyID,
			InviteCode: newCode,
		})

		state.GenerateInviteCodeResults.Emit(GenerateInviteCodeResult{
			RequestID:  payload.RequestID,
			IsSuccess:  true,
			Message:    "invite code generated",
			InviteCode: newCode,
		})
	}
}

func processUpdateSessionPassthroughCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.UpdateSessionPassthroughCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.UpdateSessionPassthroughResults.Emit(UpdateSessionPassthroughResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Only leader can update session passthrough data
		if !lobby.IsLeader(playerID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("player_id", playerID).
				Msg("only leader can update session passthrough data")
			state.UpdateSessionPassthroughResults.Emit(UpdateSessionPassthroughResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can update session passthrough data",
			})
			continue
		}

		lobby.Session.PassthroughData = payload.PassthroughData
		result.lobbyRef.Set(lobby)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Msg("Session passthrough data updated")

		// Emit broadcast event
		state.SessionPassthroughUpdatedEvents.Emit(SessionPassthroughUpdatedEvent{
			LobbyID:         lobbyID,
			PassthroughData: lobby.Session.PassthroughData,
		})

		state.UpdateSessionPassthroughResults.Emit(UpdateSessionPassthroughResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "session passthrough data updated",
		})
	}
}

func processUpdatePlayerPassthroughCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.UpdatePlayerPassthroughCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.UpdatePlayerPassthroughResults.Emit(UpdatePlayerPassthroughResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID

		// Update player entity's passthrough data
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(playerID)
		if !exists {
			state.UpdatePlayerPassthroughResults.Emit(UpdatePlayerPassthroughResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}
		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			state.UpdatePlayerPassthroughResults.Emit(UpdatePlayerPassthroughResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}

		playerComp := playerEntity.Player.Get()
		playerComp.PassthroughData = payload.PassthroughData
		playerEntity.Player.Set(playerComp)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Msg("Player passthrough data updated")

		// Emit broadcast event
		state.PlayerPassthroughUpdatedEvents.Emit(PlayerPassthroughUpdatedEvent{
			LobbyID: lobbyID,
			Player:  playerComp,
		})

		state.UpdatePlayerPassthroughResults.Emit(UpdatePlayerPassthroughResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "player passthrough data updated",
			Player:    playerComp,
		})
	}
}

func processGetPlayerCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.GetPlayerCmds.Iter() {
		callerID := cmd.Persona
		payload := cmd.Payload

		// Determine target player ID (self if empty)
		targetPlayerID := payload.PlayerID
		if targetPlayerID == "" {
			targetPlayerID = callerID
		}

		// Check if target player exists
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(targetPlayerID)
		if !exists {
			state.GetPlayerResults.Emit(GetPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not found",
			})
			continue
		}

		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			state.GetPlayerResults.Emit(GetPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}

		playerComp := playerEntity.Player.Get()

		state.GetPlayerResults.Emit(GetPlayerResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "player found",
			Player:    playerComp,
		})
	}
}

func processGetAllPlayersCommands(state *LobbySystemState, lobbyIndex *component.LobbyIndexComponent) {
	for cmd := range state.GetAllPlayersCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		// Get caller's lobby
		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.GetAllPlayersResults.Emit(GetAllPlayersResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}

		lobby := result.lobby

		// Get all player components
		var players []component.PlayerComponent
		for _, pid := range lobby.GetAllPlayerIDs() {
			playerEntityID, exists := lobbyIndex.GetPlayerEntityID(pid)
			if !exists {
				continue
			}
			playerEntity, pErr := state.Players.GetByID(cardinal.EntityID(playerEntityID))
			if pErr != nil {
				continue
			}
			players = append(players, playerEntity.Player.Get())
		}

		state.GetAllPlayersResults.Emit(GetAllPlayersResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "players found",
			Players:   players,
		})
	}
}

// generateID generates a unique ID using UUID.
func generateID() string {
	return uuid.New().String()
}

// -----------------------------------------------------------------------------
// Heartbeat System
// -----------------------------------------------------------------------------

// HeartbeatSystemState is the state for the heartbeat system.
type HeartbeatSystemState struct {
	cardinal.BaseSystemState

	// Commands
	HeartbeatCmds cardinal.WithCommand[HeartbeatCommand]

	// Entities
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}]

	Players cardinal.Contains[struct {
		Player cardinal.Ref[component.PlayerComponent]
	}]

	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events
	PlayerTimedOutEvents cardinal.WithEvent[PlayerTimedOutEvent]
	PlayerLeftEvents     cardinal.WithEvent[PlayerLeftEvent]
	LeaderChangedEvents  cardinal.WithEvent[LeaderChangedEvent]
	LobbyDeletedEvents   cardinal.WithEvent[LobbyDeletedEvent]
}

// HeartbeatSystem processes heartbeat commands and removes stale players.
func HeartbeatSystem(state *HeartbeatSystemState) {
	now := state.Timestamp().Unix()

	// Get lobby index
	var lobbyIndex component.LobbyIndexComponent
	var lobbyIndexEntityID cardinal.EntityID
	for entityID, idx := range state.LobbyIndexes.Iter() {
		lobbyIndex = idx.Index.Get()
		lobbyIndexEntityID = entityID
		break
	}

	// Debug: print deadline map state
	state.Logger().Debug().
		Interface("deadline_map", lobbyIndex.PlayerDeadline).
		Int64("now", now).
		Msg("HeartbeatSystem tick")

	// Get config
	var config component.ConfigComponent
	for _, cfg := range state.Configs.Iter() {
		config = cfg.Config.Get()
		break
	}

	// Get timeout for deadline
	timeout := config.HeartbeatTimeout
	if timeout <= 0 {
		timeout = 30 // default 30 seconds
	}

	// Process heartbeat commands - update deadline for senders
	processHeartbeatCommands(state, &lobbyIndex, now, timeout)

	// Find timed out players - O(allPlayers)
	timedOutPlayers := findTimedOutPlayers(&lobbyIndex, now)

	// Early exit if no players timed out
	if len(timedOutPlayers) == 0 {
		// Save lobby index (heartbeat commands may have updated deadlines)
		if lobbyIndexEntity, err := state.LobbyIndexes.GetByID(lobbyIndexEntityID); err == nil {
			lobbyIndexEntity.Index.Set(lobbyIndex)
		}
		return
	}

	// Group timed out players by lobby for efficient processing
	timedOutByLobby := groupPlayersByLobby(timedOutPlayers)

	// Process each affected lobby
	var lobbiesToDestroy []lobbyToDestroy
	var playerEntitiesToDestroy []cardinal.EntityID
	for lobbyID, players := range timedOutByLobby {
		playerEntities, toDestroy := processTimedOutLobby(state, &lobbyIndex, lobbyID, players)
		playerEntitiesToDestroy = append(playerEntitiesToDestroy, playerEntities...)
		if toDestroy != nil {
			lobbiesToDestroy = append(lobbiesToDestroy, *toDestroy)
		}
	}

	// Destroy player entities
	for _, entityID := range playerEntitiesToDestroy {
		state.Players.Destroy(entityID)
	}

	// Destroy empty lobbies
	for _, toDestroy := range lobbiesToDestroy {
		state.Lobbies.Destroy(toDestroy.entityID)
		state.Logger().Info().
			Str("lobby_id", toDestroy.lobbyID).
			Msg("Lobby deleted (empty after timeout)")
	}

	// Save lobby index
	if lobbyIndexEntity, err := state.LobbyIndexes.GetByID(lobbyIndexEntityID); err == nil {
		lobbyIndexEntity.Index.Set(lobbyIndex)
	}
}
