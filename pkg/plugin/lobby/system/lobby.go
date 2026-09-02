package system

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby/component"
	"github.com/google/uuid"
)

// -----------------------------------------------------------------------------
// Commands
// -----------------------------------------------------------------------------

// CreateLobbyCommand creates a new lobby with the sender as leader.
// The server resolves Preset against lobby.Config.LobbyPresets and
// rejects unknown or empty preset labels. Clients cannot supply
// arbitrary team configuration; the server is the source of truth.
//
// The target game shard is not specified here: it is assigned later by
// an external orchestrator via AssignShardCommand when the session starts.
type CreateLobbyCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
	// Preset is the label of a server-registered team configuration.
	Preset string `json:"preset"`
	// PlayerPassthroughData is opaque client JSON for the creating player, relayed to other clients.
	PlayerPassthroughData string `json:"player_passthrough_data,omitempty"`
	// SessionPassthroughData is opaque client JSON for the lobby session, relayed to other clients.
	SessionPassthroughData string `json:"session_passthrough_data,omitempty"`
}

// TeamConfig is an alias for component.TeamConfig to preserve the
// existing external type path (lobby.TeamConfig).
type TeamConfig = component.TeamConfig

// Name returns the command name.
func (CreateLobbyCommand) Name() string { return "lobby_create" }

// JoinLobbyCommand joins an existing lobby via invite code.
type JoinLobbyCommand struct {
	RequestID  string `json:"request_id"`        // For matching request/response
	InviteCode string `json:"invite_code"`       // Required: invite code to join
	TeamID     string `json:"team_id,omitempty"` // Optional: team to join by ID (joins first available if empty)
	// PlayerPassthroughData is opaque client JSON for the joining player, relayed to other clients.
	PlayerPassthroughData string `json:"player_passthrough_data,omitempty"`
}

// Name returns the command name.
func (JoinLobbyCommand) Name() string { return "lobby_join" }

// JoinTeamCommand moves a player to a different team.
type JoinTeamCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
	TeamID    string `json:"team_id"`
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

// StartSessionCommand starts the session (leader only). The
// corresponding StartSessionResult is emitted asynchronously, typically
// several ticks after this command is accepted, once an orchestrator
// responds with AssignShardCommand. Clients must not set short
// timeouts on StartSessionResult.
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
	RequestID       string `json:"request_id"` // For matching request/response
	PassthroughData string `json:"passthrough_data"`
}

// Name returns the command name.
func (UpdateSessionPassthroughCommand) Name() string { return "lobby_update_session_passthrough" }

// UpdatePlayerPassthroughCommand updates the player's own passthrough data.
type UpdatePlayerPassthroughCommand struct {
	RequestID       string `json:"request_id"` // For matching request/response
	PassthroughData string `json:"passthrough_data"`
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

// GetLobbyCommand fetches the caller's current lobby snapshot. Used by
// reconnecting clients to restore their full view (teams, session
// state, assigned GameWorld, etc.) in a single round-trip.
type GetLobbyCommand struct {
	RequestID string `json:"request_id"` // For matching request/response
}

// Name returns the command name.
func (GetLobbyCommand) Name() string { return "lobby_get_lobby" }

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
	LobbyID string                    `json:"lobby_id"`
	TeamID  string                    `json:"team_id"`
	Player  component.PlayerComponent `json:"player"`
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
	LobbyID   string                    `json:"lobby_id"`
	OldTeamID string                    `json:"old_team_id"`
	NewTeamID string                    `json:"new_team_id"`
	Player    component.PlayerComponent `json:"player"`
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

// SessionStartedEvent is emitted when a session starts. Carries the
// assigned game shard address so every player in the lobby (not just
// the session starter) learns which gameplay shard to connect to
// without a follow-up query.
type SessionStartedEvent struct {
	LobbyID   string                 `json:"lobby_id"`
	GameWorld component.ShardAddress `json:"game_world"`
}

// Name returns the event name.
func (SessionStartedEvent) Name() string { return "lobby_session_started" }

// SessionAwaitingAllocationEvent is emitted when a session is awaiting shard
// assignment. External orchestrators listen for this event and respond
// with an AssignShardCommand to complete the session start.
type SessionAwaitingAllocationEvent struct {
	LobbyID string `json:"lobby_id"`
}

// Name returns the event name.
func (SessionAwaitingAllocationEvent) Name() string { return "lobby_session_awaiting_allocation" }

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
	LobbyID         string `json:"lobby_id"`
	PassthroughData string `json:"passthrough_data"`
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
	RequestID        string                                               `json:"request_id"`
	IsSuccess        bool                                                 `json:"is_success"`
	Message          string                                               `json:"message"`
	Lobby            component.LobbyComponent                             `json:"lobby,omitempty"`
	PlayersList      [component.MaxLobbyPlayers]component.PlayerComponent `json:"players_list,omitempty"`
	PlayersListCount int                                                  `json:"players_list_count"`
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
// Emitted asynchronously — may arrive several ticks after the command,
// once the orchestrator assigns a game shard via AssignShardCommand.
// On success, GameWorld holds the assigned shard address so the client
// can connect without an extra query. Zero-valued on failure.
type StartSessionResult struct {
	RequestID string                 `json:"request_id"`
	IsSuccess bool                   `json:"is_success"`
	Message   string                 `json:"message"`
	GameWorld component.ShardAddress `json:"game_world,omitempty"`
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
	RequestID    string                                               `json:"request_id"`
	IsSuccess    bool                                                 `json:"is_success"`
	Message      string                                               `json:"message"`
	Players      [component.MaxLobbyPlayers]component.PlayerComponent `json:"players,omitempty"`
	PlayersCount int                                                  `json:"players_count"`
}

// Name returns the request-prefixed event name.
func (r GetAllPlayersResult) Name() string {
	return r.RequestID + "_get_all_players_result"
}

// GetLobbyResult is sent back to the client after GetLobbyCommand.
type GetLobbyResult struct {
	RequestID string                   `json:"request_id"`
	IsSuccess bool                     `json:"is_success"`
	Message   string                   `json:"message"`
	Lobby     component.LobbyComponent `json:"lobby,omitempty"`
}

// Name returns the request-prefixed event name.
func (r GetLobbyResult) Name() string {
	return r.RequestID + "_get_lobby_result"
}

// -----------------------------------------------------------------------------
// Cross-Shard Commands
// -----------------------------------------------------------------------------

// NotifySessionStartCommand is sent to game shard when a session starts.
// NotifySessionStartCommand tells the game shard a session is starting. It's a wire DTO: it carries
// only what the receiver needs (the lobby id and this lobby shard's address to reply to), not the live
// LobbyComponent/PlayerComponent — those are domain types and never go on the wire.
type NotifySessionStartCommand struct {
	LobbyID    string                 `json:"lobby_id"`
	LobbyWorld component.ShardAddress `json:"lobby_world"`
}

// Name returns the command name.
func (NotifySessionStartCommand) Name() string { return "lobby_notify_session_start" }

// NotifySessionEndCommand is sent from game shard to lobby when session ends.
type NotifySessionEndCommand struct {
	LobbyID string `json:"lobby_id"`
}

// Name returns the command name.
func (NotifySessionEndCommand) Name() string { return "lobby_notify_session_end" }

// AssignShardCommand is sent by an external orchestrator to complete a
// session start that is waiting in SessionStateAwaitingAllocation. The
// lobby writes GameWorld directly onto the lobby component, transitions
// to SessionStateInSession, and dispatches NotifySessionStartCommand.
// The orchestrator is responsible for providing a complete GameWorld
// address (Region, Organization, Project, ShardID).
//
// If GameWorld.ShardID is empty, the assignment is treated as a failure:
// the lobby returns to SessionStateIdle and a failure StartSessionResult
// is emitted with Reason as the failure message.
//
// RequestID must equal the lobby's current Session.PendingRequestID (which
// the orchestrator reads from the LobbyComponent). Mismatched RequestIDs
// are rejected; this rejects late duplicate or stale commands that arrive
// after the original pending cycle has already been completed or cancelled.
type AssignShardCommand struct {
	LobbyID   string                 `json:"lobby_id"`
	RequestID string                 `json:"request_id"`
	GameWorld component.ShardAddress `json:"game_world"`
	Reason    string                 `json:"reason,omitempty"`
}

// Name returns the command name.
func (AssignShardCommand) Name() string { return "lobby_assign_shard" }

// StartSessionPayload is an alias for NotifySessionStartCommand for documentation clarity.
type StartSessionPayload = NotifySessionStartCommand

// -----------------------------------------------------------------------------
// Provider Interface
// -----------------------------------------------------------------------------

// LobbyProvider defines customizable behavior for the lobby system.
type LobbyProvider interface {
	// GenerateInviteCode generates a custom invite code for the lobby.
	//
	// seed is the only source of variation between calls for the same lobby, and
	// the caller must vary it per attempt: generateInviteCodeWithRetry retries on
	// collision and depends on each attempt yielding a different code.
	// Implementations should be pure in (lobby, seed).
	GenerateInviteCode(lobby *component.LobbyComponent, seed int64) string

	// ValidateJoin runs game-specific guards on a JoinLobbyCommand after
	// the generic guards (already-in-lobby, invalid code, in-session)
	// pass and before team selection. Return ok=false with a non-empty
	// reason to reject the join; the plugin emits a JoinLobbyResult
	// failure carrying that reason. Return ok=true to allow.
	ValidateJoin(lobby *component.LobbyComponent, cmd JoinLobbyCommand) (ok bool, reason string)
}

// DefaultProvider provides default implementations.
type DefaultProvider struct{}

// ValidateJoin accepts all joins by default. Games override this to
// enforce per-game requirements (version, region, level, etc.).
func (DefaultProvider) ValidateJoin(*component.LobbyComponent, JoinLobbyCommand) (bool, string) {
	return true, ""
}

// inviteCodeCharset contains uppercase alphanumeric characters excluding confusing ones (0, O, I, L, 1).
const inviteCodeCharset = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// inviteCodeLength is the number of characters in an invite code.
const inviteCodeLength = 6

// inviteCodeMaxRetries bounds how many codes generateInviteCodeWithRetry will try
// before giving up on finding an unused one.
const inviteCodeMaxRetries = 3

// GenerateInviteCode generates a 6-character invite code from Hash(LobbyID + seed).
// Pure in (lobby, seed): the caller owns the clock and must vary seed per attempt.
func (DefaultProvider) GenerateInviteCode(lobby *component.LobbyComponent, seed int64) string {
	data := fmt.Sprintf("%s:%d", lobby.ID, seed)
	hash := sha256.Sum256([]byte(data))

	// Index the hash bytes, not their hex encoding: a hex digit takes only 16
	// distinct values, which would limit codes to 16 of the 31 charset characters.
	code := make([]byte, inviteCodeLength)
	for i, b := range hash[:inviteCodeLength] {
		code[i] = inviteCodeCharset[int(b)%len(inviteCodeCharset)]
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

// Config is the runtime configuration the systems read, supplied by the shard's main.go through
// the lobby plugin at registration.
//
// It lives here rather than in the component package because it is not a component: it has no
// Name(), is wired to no system, never becomes an entity, and is never snapshotted. The component
// package holds only types that are actually stored in the world.
type Config struct {
	// LobbyWorld is this lobby shard's address (for game shard to send NotifySessionEndCommand back).
	LobbyWorld component.ShardAddress `json:"lobby_world"`

	// HeartbeatTimeout is how long (in seconds) before a player is removed for not sending heartbeats.
	// Clients should send heartbeats more frequently than this (e.g., every timeout/3 seconds).
	// Default: 30 seconds.
	HeartbeatTimeout int64 `json:"heartbeat_timeout"`

	// AssignmentAuthority is an accident-prevention filter, NOT an
	// authentication boundary. The plugin compares it against cmd.Persona
	// and drops mismatches. This prevents an unrelated system that
	// happens to send AssignShardCommand from accidentally completing the
	// wrong lobby's session start. It does NOT defend against a client
	// that forges Persona, because cmd.Persona is not signature-verified
	// at this layer. Real authentication must live above the plugin
	// (NATS ACLs, gateway auth, signed commands). Empty = no filter.
	AssignmentAuthority string `json:"assignment_authority,omitempty"`

	// MaxAllocationTimeout bounds how long (in seconds) a lobby may remain
	// in SessionStateAwaitingAllocation before the lobby shard fails the
	// start itself and returns to Idle. Values <= 0 disable timeout
	// enforcement entirely.
	MaxAllocationTimeout int64 `json:"max_allocation_timeout,omitempty"`
}

// storedConfig holds the configuration set by the Register function, and is the only place the
// systems read it from.
//
// Deliberately not an entity. Every field comes from a literal in the shard's main.go, so the binary
// already holds the value before the world exists and there is nothing for a snapshot to contribute.
// Persisting it was actively wrong: restore runs after Init and replaces the world wholesale, so a
// redeployed shard silently kept the snapshot's LobbyWorld and AssignmentAuthority instead of the
// ones it was deployed with.
//
//nolint:gochecknoglobals // set once at initialization, read-only thereafter
var storedConfig Config

// storedPresets is the server-owned team registry. Kept beside storedConfig rather than on
// Config: it is build-time configuration, not world state, and persisting it would mean a
// restored shard silently ignores the presets the running binary declares. Staying out of ECS also
// lets it remain a map — component fields must copy cleanly, plugin state need not.
//
//nolint:gochecknoglobals // set once at initialization, read-only thereafter
var storedPresets map[string][]component.TeamConfig

// SetConfig stores the configuration for the init system to use.
//
// Panics on an unusable preset. A preset comes from the server's own main.go, so an invalid one is a
// deployment bug, not client input: failing at boot puts the reason in front of whoever deployed it,
// where returning an error here would surface as every CreateLobbyCommand being rejected by a shard
// that otherwise looks healthy.
func SetConfig(config Config, presets map[string][]component.TeamConfig) {
	// Every bad preset at once, sorted: map order is random, so reporting the first one found would
	// make an operator with two mistakes fix one, redeploy, and hit the other.
	var unusable []string
	for name, teams := range presets {
		if reason := validatePreset(teams); reason != "" {
			unusable = append(unusable, fmt.Sprintf("%q: %s", name, reason))
		}
	}
	if len(unusable) > 0 {
		sort.Strings(unusable)
		panic("unusable lobby presets:\n  " + strings.Join(unusable, "\n  "))
	}

	// A second Register means a second world in this process. Discard the previous world's index
	// rather than let indexBuilt latch: the next tick would otherwise resolve this world's lobby IDs
	// to the previous world's entity IDs, and destroy entities by them.
	resetIndex()

	storedConfig = config
	storedPresets = presets
}

// -----------------------------------------------------------------------------
// Init System
// -----------------------------------------------------------------------------

// InitSystemState is the state for the init system.
type InitSystemState struct {
	cardinal.BaseSystemState
}

// InitSystem invalidates the lookup index. Runs on boot and again inside World.reset().
//
// It does not build the index, because Init runs before restore and restore replaces the world
// wholesale — an index built here would describe the pre-restore world. The rebuild happens on the
// first tick instead, which is the earliest point the world is final. See rebuildIndex.
func InitSystem(_ *InitSystemState) {
	indexBuilt = false
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
	AssignShardCmds              cardinal.WithCommand[AssignShardCommand]
	GenerateInviteCodeCmds       cardinal.WithCommand[GenerateInviteCodeCommand]
	UpdateSessionPassthroughCmds cardinal.WithCommand[UpdateSessionPassthroughCommand]
	UpdatePlayerPassthroughCmds  cardinal.WithCommand[UpdatePlayerPassthroughCommand]
	GetPlayerCmds                cardinal.WithCommand[GetPlayerCommand]
	GetAllPlayersCmds            cardinal.WithCommand[GetAllPlayersCommand]
	GetLobbyCmds                 cardinal.WithCommand[GetLobbyCommand]

	// Entities
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}]

	Players cardinal.Contains[struct {
		Player cardinal.Ref[component.PlayerComponent]
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
	SessionAwaitingAllocationEvents cardinal.WithEvent[SessionAwaitingAllocationEvent]
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
	GetLobbyResults                 cardinal.WithEvent[GetLobbyResult]
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
	lobbyIndex *lookupIndex,
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

	config := storedConfig

	// Get timeout for deadline
	timeout := config.HeartbeatTimeout
	if timeout <= 0 {
		timeout = 30 // default 30 seconds
	}

	if !indexBuilt {
		var lobbies []lobbyRow
		for eid, l := range state.Lobbies.Iter() {
			lobbies = append(lobbies, lobbyRow{entityID: eid, lobby: l.Lobby.Get()})
		}
		var players []playerRow
		for eid, pl := range state.Players.Iter() {
			players = append(players, playerRow{entityID: eid, player: pl.Player.Get()})
		}
		rebuildIndex(lobbies, players, now, timeout)
	}
	lobbyIndex := &index

	// Process all commands
	processCreateLobbyCommands(state, lobbyIndex, now, timeout)
	processJoinLobbyCommands(state, lobbyIndex, now, timeout)
	processJoinTeamCommands(state, lobbyIndex)
	processLeaveLobbyCommands(state, lobbyIndex)
	processSetReadyCommands(state, lobbyIndex)
	processKickPlayerCommands(state, lobbyIndex)
	processTransferLeaderCommands(state, lobbyIndex)
	processStartSessionCommands(state, lobbyIndex)
	processAssignShardCommands(state, lobbyIndex, &config)
	processAllocationTimeouts(state, &config)
	processNotifySessionEndCommands(state, lobbyIndex)
	processGenerateInviteCodeCommands(state, lobbyIndex)
	processUpdateSessionPassthroughCommands(state, lobbyIndex)
	processUpdatePlayerPassthroughCommands(state, lobbyIndex)
	processGetPlayerCommands(state, lobbyIndex)
	processGetAllPlayersCommands(state, lobbyIndex)
	processGetLobbyCommands(state, lobbyIndex)
}

// timedOutPlayer holds info about a player who missed heartbeat deadline.
type timedOutPlayer struct {
	playerID       string
	lobbyID        string
	teamID         string
	playerEntityID uint32
}

// findTimedOutPlayers returns all players whose deadline has passed.
func findTimedOutPlayers(lobbyIndex *lookupIndex, now int64) []timedOutPlayer {
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
	if lobby.PlayerCount == 0 {
		return ""
	}
	return lobby.PlayerIDs[0]
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

// playerTeamID resolves which team a player is on.
//
// PlayerComponent.TeamID is the authoritative record; the index only caches it. Preferring the
// component means a drifted index cannot leave a team's PlayerCount un-decremented on removal —
// nothing recomputes those counts, so such a team would read full forever.
func playerTeamID(state *LobbySystemState, lobbyIndex *lookupIndex, playerID string) string {
	if entityID, ok := lobbyIndex.GetPlayerEntityID(playerID); ok {
		if entity, err := state.Players.GetByID(cardinal.EntityID(entityID)); err == nil {
			return entity.Player.Get().TeamID
		}
	}
	teamID, _ := lobbyIndex.GetPlayerTeam(playerID)
	return teamID
}

// addPresetTeams installs the preset's teams on a newly built lobby.
//
// Reports false after emitting the failure result, and the caller must then abandon the whole
// command: a lobby that kept only some of its teams would still go on to get an invite code, a
// leader and index entries, leaving players unable to join the teams the preset promised.
func addPresetTeams(
	state *LobbySystemState,
	lobby *component.LobbyComponent,
	presetTeams []TeamConfig,
	playerID, preset, requestID string,
) bool {
	for _, tc := range presetTeams {
		if lobby.AddTeam(component.Team{TeamID: tc.TeamID, MaxPlayers: tc.MaxPlayers}) {
			continue
		}
		state.Logger().Warn().
			Str("player_id", playerID).
			Str("preset", preset).
			Int("max_teams", component.MaxLobbyTeams).
			Msg("create lobby rejected: preset declares more teams than a lobby can hold")
		emitCreateLobbyFailure(state, requestID, "preset misconfigured: too many teams")
		return false
	}
	return true
}

// emitJoinLobbyFailure emits a failure result for JoinLobby command.
func emitJoinLobbyFailure(state *LobbySystemState, requestID, message string) {
	state.JoinLobbyResults.Broadcast(JoinLobbyResult{
		RequestID: requestID,
		IsSuccess: false,
		Message:   message,
	})
}

// emitCreateLobbyFailure emits a failure result for CreateLobby command.
func emitCreateLobbyFailure(state *LobbySystemState, requestID, message string) {
	state.CreateLobbyResults.Broadcast(CreateLobbyResult{
		RequestID: requestID,
		IsSuccess: false,
		Message:   message,
	})
}

// createPlayerEntity creates a player entity and returns the component and entity ID.
func createPlayerEntity(
	state *LobbySystemState,
	playerID, lobbyID, teamID string,
	passthroughData string,
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
	// lobby is the last-read snapshot of the lobby component, used by the
	// caller to emit a failure StartSessionResult if the lobby was in
	// SessionStateAwaitingAllocation at destruction time.
	lobby component.LobbyComponent
}

// processTimedOutLobby handles removing timed out players from a single lobby.
// Returns player entity IDs to destroy and lobby to destroy (if empty).
func processTimedOutLobby(
	state *HeartbeatSystemState,
	lobbyIndex *lookupIndex,
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
			Str("invite_code", lobby.InviteCode).
			Msg("Player timed out due to missed heartbeats")

		state.PlayerTimedOutEvents.Broadcast(PlayerTimedOutEvent{LobbyID: lobbyID, PlayerID: p.playerID})
	}

	// Check if lobby is empty
	if lobbyIndex.GetLobbyPlayerCount(lobbyID) == 0 {
		lobbyIndex.RemoveLobby(lobbyID, lobby.InviteCode)
		// The other way a code dies, and the only one no player asked for: everyone
		// stopped heartbeating. A code that disappears with no "Lobby deleted (empty)"
		// line ended here.
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("invite_code", lobby.InviteCode).
			Msg("Lobby marked for deletion (empty after timeout)")
		state.LobbyDeletedEvents.Broadcast(LobbyDeletedEvent{LobbyID: lobbyID})
		return playerEntities, &lobbyToDestroy{
			entityID: cardinal.EntityID(lobbyEntityID),
			lobbyID:  lobbyID,
			lobby:    lobby,
		}
	}

	// Handle leader timeout
	if isLeaderInList(lobby.LeaderID, players) {
		oldLeaderID := lobby.LeaderID
		lobby.LeaderID = findNewLeader(&lobby)
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("old_leader", oldLeaderID).
			Str("new_leader", lobby.LeaderID).
			Str("invite_code", lobby.InviteCode).
			Msg("Leadership auto-transferred after timeout")
		state.LeaderChangedEvents.Broadcast(LeaderChangedEvent{
			LobbyID: lobbyID, OldLeaderID: oldLeaderID, NewLeaderID: lobby.LeaderID,
		})
	}

	lobbyEntity.Lobby.Set(lobby)
	return playerEntities, nil
}

// processHeartbeatCommands updates deadlines for players who sent heartbeats.
func processHeartbeatCommands(
	state *HeartbeatSystemState,
	lobbyIndex *lookupIndex,
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

// validateUniqueTeamIDs checks for duplicate team IDs in a preset.
// Returns the duplicate ID if found, empty string otherwise.
func validateUniqueTeamIDs(teams []TeamConfig) string {
	teamIDs := make(map[string]bool)
	for _, tc := range teams {
		if teamIDs[tc.TeamID] {
			return tc.TeamID
		}
		teamIDs[tc.TeamID] = true
	}
	return ""
}

// validatePreset reports why a preset cannot be used, or "" when it is fine.
//
// Checked at Register so a misconfigured deployment fails at boot with a message an operator can
// act on, rather than surfacing as a rejected CreateLobbyCommand on every attempt. Checked again on
// the command path so presets installed through SetConfig directly cannot slip past.
//
// The structural limits it enforces are storage bounds, not game rules: a preset may promise fewer
// seats than MaxLobbyPlayers, never more, because the roster physically cannot hold them.
func validatePreset(teams []TeamConfig) string {
	if len(teams) == 0 {
		return "no teams"
	}
	if len(teams) > component.MaxLobbyTeams {
		return fmt.Sprintf("declares %d teams, more than the %d a lobby can hold",
			len(teams), component.MaxLobbyTeams)
	}
	if duplicateID := validateUniqueTeamIDs(teams); duplicateID != "" {
		return "duplicate team id " + duplicateID
	}

	// A team with MaxPlayers <= 0 is unlimited, and the roster cap is what bounds it — so the
	// per-preset total is only meaningful when every team is bounded.
	total := 0
	allBounded := true
	for _, tc := range teams {
		if tc.MaxPlayers <= 0 {
			allBounded = false
			continue
		}
		if tc.MaxPlayers > component.MaxLobbyPlayers {
			return fmt.Sprintf("team %q allows %d players, more than the %d a lobby can hold",
				tc.TeamID, tc.MaxPlayers, component.MaxLobbyPlayers)
		}
		total += tc.MaxPlayers
	}
	if allBounded && total > component.MaxLobbyPlayers {
		return fmt.Sprintf("teams allow %d players in total, more than the %d a lobby can hold",
			total, component.MaxLobbyPlayers)
	}
	return ""
}

// resolvePreset validates and looks up a preset in the server-owned
// registry. Returns the team list on success, or nil and an
// error message describing why the preset was rejected. The server is
// the source of truth for team configuration; the client's preset
// label must match an entry the operator registered.
func resolvePreset(preset string, presets map[string][]TeamConfig) ([]TeamConfig, string) {
	if preset == "" {
		return nil, "preset is required"
	}
	teams, ok := presets[preset]
	if !ok {
		return nil, "unknown preset: " + preset
	}
	if reason := validatePreset(teams); reason != "" {
		return nil, "preset misconfigured: " + reason
	}
	return teams, ""
}

// generateInviteCodeWithRetry generates an invite code with collision check.
// Retries up to maxRetries times if collision detected.
// Returns the code and whether generation succeeded.
//
// A code already owned by this lobby is not a collision, so regeneration can keep
// its current code. On create the lobby is not yet in the index, so that case
// cannot arise and this reduces to a plain "code is unused" check.
//
// seed comes from the tick timestamp rather than the wall clock: a system should
// read the tick's clock, not time.Now(). The attempt index is mixed in so each
// retry is guaranteed a distinct code even though the tick timestamp does not
// advance between attempts. Two lobbies created in the same tick still differ,
// because the lobby ID is part of the hash.
func generateInviteCodeWithRetry(
	lobbyIndex *lookupIndex,
	lobby *component.LobbyComponent,
	maxRetries int,
	seed int64,
) (string, bool) {
	for attempt := range maxRetries {
		code := storedProvider.GenerateInviteCode(lobby, seed+int64(attempt))
		owner, exists := lobbyIndex.GetLobbyByInviteCode(code)
		if !exists || owner == lobby.ID {
			return code, true
		}
	}
	return "", false
}

// areAllPlayersReady checks if all players in a lobby are ready.
// Returns false if lobby has no players or any player is not ready.
func areAllPlayersReady(
	state *LobbySystemState,
	lobbyIndex *lookupIndex,
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
	lobbyIndex *lookupIndex,
	lobby *component.LobbyComponent,
) ([component.MaxLobbyPlayers]component.PlayerComponent, int) {
	var playersList [component.MaxLobbyPlayers]component.PlayerComponent
	count := 0
	for _, pid := range lobby.GetAllPlayerIDs() {
		pEntityID, pExists := lobbyIndex.GetPlayerEntityID(pid)
		if !pExists {
			continue
		}
		pEntity, pErr := state.Players.GetByID(cardinal.EntityID(pEntityID))
		if pErr != nil {
			continue
		}
		playersList[count] = pEntity.Player.Get()
		count++
	}
	return playersList, count
}

// findTargetTeam finds the team for a player to join.
// If teamID is provided, it finds that specific team.
// Otherwise, it finds the first team with available space.
// Returns the team and an error message (empty string if successful).
func findTargetTeam(lobby *component.LobbyComponent, teamID string) (*component.Team, string) {
	if teamID != "" {
		team := lobby.GetTeam(teamID)
		if team == nil {
			return nil, "team not found"
		}
		if team.IsFull() {
			return nil, "team is full"
		}
		return team, ""
	}

	// Find first available team with space. TeamList clamps: ranging TeamCount would index past
	// Teams on a snapshot written by a build with a larger MaxLobbyTeams.
	teams := lobby.TeamList()
	for i := range teams {
		// A restored TeamCount can outrun the teams actually stored, leaving zero entries inside the
		// clamp. Their MaxPlayers of 0 reads as unlimited, so without this they look like the first
		// team with space and admit players to a team with no ID.
		if teams[i].TeamID == "" {
			continue
		}
		if !teams[i].IsFull() {
			return &teams[i], ""
		}
	}
	return nil, "all teams are full"
}

func processCreateLobbyCommands(
	state *LobbySystemState,
	lobbyIndex *lookupIndex,
	now, timeout int64,
) {
	for cmd := range state.CreateLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		// Check if player is already in a lobby
		if _, exists := lobbyIndex.GetPlayerLobby(playerID); exists {
			state.Logger().Warn().Str("player_id", playerID).Msg("player already in a lobby")
			emitCreateLobbyFailure(state, payload.RequestID, "player already in a lobby")
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
				State:           component.SessionStateIdle,
				PassthroughData: payload.SessionPassthroughData,
			},
			CreatedAt: now,
		}

		presetTeams, errMsg := resolvePreset(payload.Preset, storedPresets)
		if errMsg != "" {
			state.Logger().Warn().
				Str("player_id", playerID).
				Str("preset", payload.Preset).
				Msg("create lobby rejected: " + errMsg)
			emitCreateLobbyFailure(state, payload.RequestID, errMsg)
			continue
		}
		if !addPresetTeams(state, &lobby, presetTeams, playerID, payload.Preset, payload.RequestID) {
			continue
		}

		// Generate invite code with collision check
		inviteCode, ok := generateInviteCodeWithRetry(
			lobbyIndex, &lobby, inviteCodeMaxRetries, state.Timestamp().UnixNano(),
		)
		if !ok {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("invite code collision after retries")
			emitCreateLobbyFailure(state, payload.RequestID, "invite code collision")
			continue
		}
		lobby.InviteCode = inviteCode

		// Add leader to first team
		if !lobby.AddPlayerToTeam(playerID, lobby.Teams[0].TeamID) {
			state.Logger().Warn().
				Str("player_id", playerID).
				Str("preset", payload.Preset).
				Msg("create lobby rejected: leader could not join the first team")
			emitCreateLobbyFailure(state, payload.RequestID, "preset misconfigured: leader cannot join")
			continue
		}

		// Create lobby entity
		lobbyEntityID, lobbyEntity := state.Lobbies.Create()
		lobbyEntity.Lobby.Set(lobby)

		// Create player entity and update index
		playerComp, playerEntityID := createPlayerEntity(
			state, playerID, lobbyID, lobby.Teams[0].TeamID, payload.PlayerPassthroughData, now,
		)
		lobbyIndex.AddLobby(lobbyID, uint32(lobbyEntityID), inviteCode)
		lobbyIndex.AddPlayerToLobby(playerID, lobbyID, lobby.Teams[0].TeamID, uint32(playerEntityID), now+timeout)

		// invite_code is logged so an "invalid invite code" report can be traced back to
		// the moment the code was issued. Without it there is no way to tell a code the
		// server never issued from one it issued and then lost.
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("leader_id", playerID).
			Str("invite_code", inviteCode).
			Str("request_id", payload.RequestID).
			Msg("Lobby created")

		// Emit broadcast event
		state.LobbyCreatedEvents.Broadcast(LobbyCreatedEvent{
			LobbyID:    lobbyID,
			LeaderID:   playerID,
			InviteCode: inviteCode,
		})

		// Emit success result
		state.CreateLobbyResults.Broadcast(CreateLobbyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "lobby created",
			Lobby:     lobby,
			Player:    playerComp,
		})
	}
}

// resolveInviteCode turns a join command's invite code into the lobby it names. It logs
// the cause and emits the join failure itself, so callers only branch on ok.
//
// The two Error branches are the only ones that mean the server is at fault: the code
// resolved to a lobby, so the invite index and the entity index disagree, or the entity
// they agree on is gone. A code whose trace ends there was lost by the backend, not by
// the player.
func resolveInviteCode(
	state *LobbySystemState,
	lobbyIndex *lookupIndex,
	playerID string,
	payload JoinLobbyCommand,
) (string, cardinal.Ref[component.LobbyComponent], bool) {
	var none cardinal.Ref[component.LobbyComponent]

	lobbyID, exists := lobbyIndex.GetLobbyByInviteCode(payload.InviteCode)
	if !exists {
		// known_codes distinguishes "this one code is missing" from "the index is
		// empty", which look identical to the player but mean very different things.
		state.Logger().Warn().
			Str("invite_code", payload.InviteCode).
			Str("player_id", playerID).
			Str("request_id", payload.RequestID).
			Int("known_codes", lobbyIndex.InviteCodeCount()).
			Msg("invalid invite code")
		emitJoinLobbyFailure(state, payload.RequestID, "invalid invite code")
		return "", none, false
	}

	lobbyEntityID, exists := lobbyIndex.GetEntityID(lobbyID)
	if !exists {
		state.Logger().Error().
			Str("invite_code", payload.InviteCode).
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("request_id", payload.RequestID).
			Msg("invite code maps to a lobby with no entity")
		emitJoinLobbyFailure(state, payload.RequestID, "lobby not found")
		return "", none, false
	}

	lobbyEntity, err := state.Lobbies.GetByID(cardinal.EntityID(lobbyEntityID))
	if err != nil {
		state.Logger().Error().Err(err).
			Str("invite_code", payload.InviteCode).
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("request_id", payload.RequestID).
			Msg("invite code maps to a lobby entity that no longer exists")
		emitJoinLobbyFailure(state, payload.RequestID, "lobby not found")
		return "", none, false
	}

	return lobbyID, lobbyEntity.Lobby, true
}

// admitToTeam runs the join guards that apply once the invite code has already resolved,
// and places the player on a team. It logs the cause and emits the join failure itself,
// so callers only branch on ok.
//
// Every rejection here carries invite_code even though the code is valid in all of them:
// a grep for the code has to surface attempts that failed for some other reason, or the
// code looks unused when it was actually tried and refused.
//
// lobby is mutated in place on success — the player is appended to the target team.
func admitToTeam(
	state *LobbySystemState,
	lobby *component.LobbyComponent,
	lobbyID, playerID string,
	payload JoinLobbyCommand,
) (*component.Team, bool) {
	if lobby.Session.State == component.SessionStateInSession {
		state.Logger().Warn().
			Str("lobby_id", lobbyID).
			Str("invite_code", payload.InviteCode).
			Str("player_id", playerID).
			Str("request_id", payload.RequestID).
			Msg("lobby is in session")
		emitJoinLobbyFailure(state, payload.RequestID, "lobby is in session")
		return nil, false
	}

	// Game-specific validation (version, region, level, etc.)
	if ok, reason := storedProvider.ValidateJoin(lobby, payload); !ok {
		state.Logger().Warn().
			Str("lobby_id", lobbyID).
			Str("invite_code", payload.InviteCode).
			Str("player_id", playerID).
			Str("reason", reason).
			Msg("join rejected by provider")
		emitJoinLobbyFailure(state, payload.RequestID, reason)
		return nil, false
	}

	targetTeam, errMsg := findTargetTeam(lobby, payload.TeamID)
	if targetTeam == nil {
		state.Logger().Warn().
			Str("lobby_id", lobbyID).
			Str("invite_code", payload.InviteCode).
			Str("player_id", playerID).
			Str("team_id", payload.TeamID).
			Msg(errMsg)
		emitJoinLobbyFailure(state, payload.RequestID, errMsg)
		return nil, false
	}

	if !lobby.AddPlayerToTeam(playerID, targetTeam.TeamID) {
		state.Logger().Warn().
			Str("lobby_id", lobbyID).
			Str("invite_code", payload.InviteCode).
			Str("player_id", playerID).
			Msg("failed to join team")
		emitJoinLobbyFailure(state, payload.RequestID, "failed to join team")
		return nil, false
	}

	return targetTeam, true
}

func processJoinLobbyCommands(
	state *LobbySystemState,
	lobbyIndex *lookupIndex,
	now, timeout int64,
) {
	for cmd := range state.JoinLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		// Check if player is already in a lobby. invite_code is logged even though the
		// code is not the problem here: a grep for the code must surface every attempt to
		// use it, including the ones rejected for an unrelated reason.
		if existingLobbyID, exists := lobbyIndex.GetPlayerLobby(playerID); exists {
			state.Logger().Warn().
				Str("player_id", playerID).
				Str("invite_code", payload.InviteCode).
				Str("lobby_id", existingLobbyID).
				Str("request_id", payload.RequestID).
				Msg("player already in a lobby")
			emitJoinLobbyFailure(state, payload.RequestID, "player already in a lobby")
			continue
		}

		lobbyID, lobbyRef, found := resolveInviteCode(state, lobbyIndex, playerID, payload)
		if !found {
			continue
		}
		lobby := lobbyRef.Get()

		targetTeam, admitted := admitToTeam(state, &lobby, lobbyID, playerID, payload)
		if !admitted {
			continue
		}

		lobbyRef.Set(lobby)

		// Create player entity
		playerComp, playerEntityID := createPlayerEntity(
			state, playerID, lobbyID, targetTeam.TeamID, payload.PlayerPassthroughData, now,
		)
		lobbyIndex.AddPlayerToLobby(playerID, lobbyID, targetTeam.TeamID, uint32(playerEntityID), now+timeout)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("team_id", targetTeam.TeamID).
			Str("invite_code", payload.InviteCode).
			Str("request_id", payload.RequestID).
			Msg("Player joined lobby")

		// Emit broadcast event
		state.PlayerJoinedEvents.Broadcast(PlayerJoinedEvent{
			LobbyID: lobbyID,
			TeamID:  targetTeam.TeamID,
			Player:  playerComp,
		})

		// Gather all players in the lobby for the result
		playersList, playersListCount := gatherLobbyPlayers(state, lobbyIndex, &lobby)

		// Emit success result
		state.JoinLobbyResults.Broadcast(JoinLobbyResult{
			RequestID:        payload.RequestID,
			IsSuccess:        true,
			Message:          "joined lobby",
			Lobby:            lobby,
			PlayersList:      playersList,
			PlayersListCount: playersListCount,
		})
	}
}

func processJoinTeamCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.JoinTeamCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.JoinTeamResults.Broadcast(JoinTeamResult{
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
			state.JoinTeamResults.Broadcast(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "cannot change team during session",
			})
			continue
		}

		// Get current team
		oldTeamID, inTeam := lobbyIndex.GetPlayerTeam(playerID)
		if !inTeam {
			state.JoinTeamResults.Broadcast(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in any team",
			})
			continue
		}

		// Find target team by ID
		newTeam := lobby.GetTeam(payload.TeamID)
		if newTeam == nil {
			state.Logger().Warn().Str("lobby_id", lobbyID).Str("team_id", payload.TeamID).Msg("team not found")
			state.JoinTeamResults.Broadcast(JoinTeamResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "team not found",
			})
			continue
		}

		// Move to new team
		if !lobby.MovePlayerToTeam(playerID, oldTeamID, newTeam.TeamID) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("failed to change team")
			state.JoinTeamResults.Broadcast(JoinTeamResult{
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
			Str("old_team", oldTeamID).
			Str("new_team", newTeam.TeamID).
			Str("request_id", payload.RequestID).
			Str("invite_code", lobby.InviteCode).
			Msg("Player changed team")

		// Emit broadcast event
		state.PlayerChangedTeamEvents.Broadcast(PlayerChangedTeamEvent{
			LobbyID:   lobbyID,
			OldTeamID: oldTeamID,
			NewTeamID: newTeam.TeamID,
			Player:    playerComp,
		})

		state.JoinTeamResults.Broadcast(JoinTeamResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "changed team",
			Player:    playerComp,
		})
	}
}

func processLeaveLobbyCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.LeaveLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.LeaveLobbyResults.Broadcast(LeaveLobbyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}
		lobbyID := result.lobbyID
		lobby := result.lobby

		// Resolved before the entity is destroyed: it reads the player's own component.
		teamID := playerTeamID(state, lobbyIndex, playerID)

		// Delete player entity
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(playerID)
		if exists {
			state.Players.Destroy(cardinal.EntityID(playerEntityID))
		}

		lobby.RemovePlayerFromTeam(playerID, teamID)
		lobbyIndex.RemovePlayerFromLobby(playerID)

		// Emit broadcast event for player leaving
		state.PlayerLeftEvents.Broadcast(PlayerLeftEvent{
			LobbyID:  lobbyID,
			PlayerID: playerID,
		})

		// Logged before the branch below, not after it: when this is the last player, the
		// lobby is deleted as a consequence of the leave, and the deletion line has to be
		// the final entry for this code. Logging it afterwards made "Player left lobby"
		// the last thing a grep sees for a lobby that no longer exists.
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("invite_code", lobby.InviteCode).
			Str("request_id", payload.RequestID).
			Msg("Player left lobby")

		// If lobby is empty, delete it - use index for O(1) check
		if lobbyIndex.GetLobbyPlayerCount(lobbyID) == 0 {
			failPendingAssignment(&state.StartSessionResults, &lobby, "lobby deleted before shard assignment")
			lobbyIndex.RemoveLobby(lobbyID, lobby.InviteCode)
			state.Lobbies.Destroy(result.entityID)

			// The code dies here: RemoveLobby drops it from the invite index. This is the
			// end of the trace for that code, and the reason a later join is rejected.
			state.Logger().Info().
				Str("lobby_id", lobbyID).
				Str("invite_code", lobby.InviteCode).
				Str("triggered_by", playerID).
				Msg("Lobby deleted (empty)")

			// Emit broadcast event for lobby deletion
			state.LobbyDeletedEvents.Broadcast(LobbyDeletedEvent{
				LobbyID: lobbyID,
			})
		} else {
			// Transfer leadership if leader left
			if lobby.LeaderID == playerID {
				oldLeaderID := lobby.LeaderID
				lobby.LeaderID = findNewLeader(&lobby)

				state.Logger().Info().
					Str("lobby_id", lobbyID).
					Str("old_leader", oldLeaderID).
					Str("new_leader", lobby.LeaderID).
					Str("invite_code", lobby.InviteCode).
					Msg("Leadership auto-transferred")

				// Emit broadcast event for leader change
				state.LeaderChangedEvents.Broadcast(LeaderChangedEvent{
					LobbyID:     lobbyID,
					OldLeaderID: oldLeaderID,
					NewLeaderID: lobby.LeaderID,
				})
			}

			result.lobbyRef.Set(lobby)
		}

		state.LeaveLobbyResults.Broadcast(LeaveLobbyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "left lobby",
		})
	}
}

func processSetReadyCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.SetReadyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.SetReadyResults.Broadcast(SetReadyResult{
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
			state.SetReadyResults.Broadcast(SetReadyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "cannot change ready status during session",
			})
			continue
		}

		// Update player entity's IsReady
		playerEntityID, exists := lobbyIndex.GetPlayerEntityID(playerID)
		if !exists {
			state.SetReadyResults.Broadcast(SetReadyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}
		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			state.SetReadyResults.Broadcast(SetReadyResult{
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
			Str("request_id", payload.RequestID).
			Str("invite_code", lobby.InviteCode).
			Msg("Player ready status changed")

		// Emit broadcast event
		state.PlayerReadyEvents.Broadcast(PlayerReadyEvent{
			LobbyID: lobbyID,
			Player:  playerComp,
		})

		state.SetReadyResults.Broadcast(SetReadyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "ready status updated",
			Player:    playerComp,
		})
	}
}

func processKickPlayerCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.KickPlayerCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.KickPlayerResults.Broadcast(KickPlayerResult{
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
			state.KickPlayerResults.Broadcast(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can kick players",
			})
			continue
		}

		// Can't kick self
		if payload.TargetPlayerID == playerID {
			state.KickPlayerResults.Broadcast(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "cannot kick yourself",
			})
			continue
		}

		// Check if target is in lobby
		if !lobby.HasPlayer(payload.TargetPlayerID) {
			state.KickPlayerResults.Broadcast(KickPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "target player not in lobby",
			})
			continue
		}

		// Resolved before the entity is destroyed: it reads the player's own component.
		targetTeamID := playerTeamID(state, lobbyIndex, payload.TargetPlayerID)

		// Delete player entity
		targetPlayerEntityID, exists := lobbyIndex.GetPlayerEntityID(payload.TargetPlayerID)
		if exists {
			state.Players.Destroy(cardinal.EntityID(targetPlayerEntityID))
		}

		lobby.RemovePlayerFromTeam(payload.TargetPlayerID, targetTeamID)
		result.lobbyRef.Set(lobby)
		lobbyIndex.RemovePlayerFromLobby(payload.TargetPlayerID)

		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", payload.TargetPlayerID).
			Str("kicker_id", playerID).
			Str("request_id", payload.RequestID).
			Str("invite_code", lobby.InviteCode).
			Msg("Player kicked from lobby")

		// Emit broadcast event
		state.PlayerKickedEvents.Broadcast(PlayerKickedEvent{
			LobbyID:  lobbyID,
			PlayerID: payload.TargetPlayerID,
			KickerID: playerID,
		})

		state.KickPlayerResults.Broadcast(KickPlayerResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "player kicked",
		})
	}
}

func processTransferLeaderCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.TransferLeaderCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.TransferLeaderResults.Broadcast(TransferLeaderResult{
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
			state.TransferLeaderResults.Broadcast(TransferLeaderResult{
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
			state.TransferLeaderResults.Broadcast(TransferLeaderResult{
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
			Str("request_id", payload.RequestID).
			Str("invite_code", lobby.InviteCode).
			Msg("Leadership transferred")

		// Emit broadcast event
		state.LeaderChangedEvents.Broadcast(LeaderChangedEvent{
			LobbyID:     lobbyID,
			OldLeaderID: oldLeaderID,
			NewLeaderID: payload.TargetPlayerID,
		})

		state.TransferLeaderResults.Broadcast(TransferLeaderResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "leadership transferred",
		})
	}
}

func processStartSessionCommands(
	state *LobbySystemState,
	lobbyIndex *lookupIndex,
) {
	for cmd := range state.StartSessionCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.StartSessionResults.Broadcast(StartSessionResult{
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
			state.StartSessionResults.Broadcast(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can start session",
			})
			continue
		}

		// Already in session or awaiting assignment
		if lobby.Session.State == component.SessionStateInSession {
			state.StartSessionResults.Broadcast(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "session already in progress",
			})
			continue
		}
		if lobby.Session.State == component.SessionStateAwaitingAllocation {
			state.StartSessionResults.Broadcast(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "session already pending shard assignment",
			})
			continue
		}

		// Check all ready
		if !areAllPlayersReady(state, lobbyIndex, &lobby) {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("not all players are ready")
			state.StartSessionResults.Broadcast(StartSessionResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "not all players are ready",
			})
			continue
		}

		// Hand off to an external orchestrator via SessionAwaitingAllocationEvent. The
		// lobby waits in SessionStateAwaitingAllocation until an AssignShardCommand
		// arrives. Games that want immediate dispatch from a pre-configured
		// GameWorld can wire a one-system orchestrator that observes
		// SessionStateAwaitingAllocation lobbies and immediately sends AssignShardCommand
		// with lobby.GameWorld.
		lobby.Session.State = component.SessionStateAwaitingAllocation
		lobby.Session.PendingRequestID = payload.RequestID
		lobby.Session.PendingStartedAt = state.Timestamp().Unix()
		result.lobbyRef.Set(lobby)

		// player_id is the leader who started it — this line had no actor at all, so a
		// session start could not be attributed to anyone.
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("player_id", playerID).
			Str("request_id", payload.RequestID).
			Str("invite_code", lobby.InviteCode).
			Msg("Session pending shard assignment")

		state.SessionAwaitingAllocationEvents.Broadcast(SessionAwaitingAllocationEvent{
			LobbyID: lobbyID,
		})
	}
}

// processAllocationTimeouts fails any lobby that has been waiting in
// SessionStateAwaitingAllocation for more than config.MaxAllocationTimeout
// seconds. Disabled when MaxAllocationTimeout <= 0. Runs once per tick.
func processAllocationTimeouts(
	state *LobbySystemState,
	config *Config,
) {
	if config.MaxAllocationTimeout <= 0 {
		return
	}
	now := state.Timestamp().Unix()

	for _, refs := range state.Lobbies.Iter() {
		lob := refs.Lobby.Get()
		if lob.Session.State != component.SessionStateAwaitingAllocation {
			continue
		}
		if now-lob.Session.PendingStartedAt < config.MaxAllocationTimeout {
			continue
		}

		state.Logger().Warn().
			Str("lobby_id", lob.ID).
			Int64("waited_seconds", now-lob.Session.PendingStartedAt).
			Int64("max_seconds", config.MaxAllocationTimeout).
			Msg("allocation timeout: failing pending session-start")

		abortAwaitingAllocation(&state.StartSessionResults, refs.Lobby, &lob, "shard assignment timed out")
	}
}

// failPendingAssignment emits a failure StartSessionResult for a lobby that
// is being destroyed or reset while in SessionStateAwaitingAllocation. Without this,
// the client that issued the original StartSessionCommand would never
// receive a response and would hang on its RequestID.
func failPendingAssignment(
	results *cardinal.WithEvent[StartSessionResult],
	lobby *component.LobbyComponent,
	reason string,
) {
	if lobby.Session.State != component.SessionStateAwaitingAllocation {
		return
	}
	if lobby.Session.PendingRequestID == "" {
		return
	}
	results.Broadcast(StartSessionResult{
		RequestID: lobby.Session.PendingRequestID,
		IsSuccess: false,
		Message:   reason,
	})
}

// abortAwaitingAllocation fails a lobby currently in
// SessionStateAwaitingAllocation: emits a failure StartSessionResult,
// clears all pending fields, transitions to SessionStateIdle, and
// persists the mutation via ref. Safe to call on lobbies not in
// SessionStateAwaitingAllocation — returns without effect. Centralizes
// the exit protocol so no caller can forget a field.
func abortAwaitingAllocation(
	results *cardinal.WithEvent[StartSessionResult],
	ref cardinal.Ref[component.LobbyComponent],
	lobby *component.LobbyComponent,
	reason string,
) {
	if lobby.Session.State != component.SessionStateAwaitingAllocation {
		return
	}
	if lobby.Session.PendingRequestID != "" {
		results.Broadcast(StartSessionResult{
			RequestID: lobby.Session.PendingRequestID,
			IsSuccess: false,
			Message:   reason,
		})
	}
	lobby.Session.State = component.SessionStateIdle
	lobby.Session.PendingRequestID = ""
	lobby.Session.PendingStartedAt = 0
	ref.Set(*lobby)
}

// dispatchSessionStart sends NotifySessionStartCommand to the game shard
// configured on the lobby. No-op if no game shard is configured.
func dispatchSessionStart(
	state *LobbySystemState,
	config *Config,
	lobby *component.LobbyComponent,
	lobbyID string,
) {
	gameWorld := lobby.GameWorld
	state.SendToShard(cardinal.OtherWorld(gameWorld), NotifySessionStartCommand{
		LobbyID:    lobbyID,
		LobbyWorld: config.LobbyWorld,
	})
	state.Logger().Info().
		Str("lobby_id", lobbyID).
		Str("game_shard", lobby.GameWorld.ShardID).
		Str("invite_code", lobby.InviteCode).
		Msg("[CROSS-SHARD] Sent NotifySessionStartCommand to game shard")
}

// processAssignShardCommands completes the session start for lobbies that
// are waiting in SessionStateAwaitingAllocation. An empty GameWorld.ShardID is treated
// as an assignment failure: the lobby returns to Idle and a failure
// StartSessionResult is emitted carrying the original RequestID.
func processAssignShardCommands(
	state *LobbySystemState,
	lobbyIndex *lookupIndex,
	config *Config,
) {
	for cmd := range state.AssignShardCmds.Iter() {
		payload := cmd.Payload

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.LobbyID)
		if !exists {
			state.Logger().Warn().
				Str("lobby_id", payload.LobbyID).
				Msg("AssignShardCommand for unknown lobby; dropping")
			continue
		}
		lobbyEntity, err := state.Lobbies.GetByID(cardinal.EntityID(lobbyEntityID))
		if err != nil {
			continue
		}
		lobby := lobbyEntity.Lobby.Get()

		if lobby.Session.State != component.SessionStateAwaitingAllocation {
			state.Logger().Warn().
				Str("lobby_id", payload.LobbyID).
				Str("state", string(lobby.Session.State)).
				Msg("AssignShardCommand received for lobby not in pending state; dropping")
			continue
		}

		if authority := config.AssignmentAuthority; authority != "" && cmd.Persona != authority {
			state.Logger().Warn().
				Str("lobby_id", payload.LobbyID).
				Str("sender", cmd.Persona).
				Str("expected", authority).
				Msg("AssignShardCommand rejected: sender is not the configured AssignmentAuthority")
			continue
		}

		if payload.RequestID != lobby.Session.PendingRequestID {
			state.Logger().Warn().
				Str("lobby_id", payload.LobbyID).
				Str("command_request_id", payload.RequestID).
				Str("pending_request_id", lobby.Session.PendingRequestID).
				Msg("AssignShardCommand rejected: RequestID does not match current pending cycle")
			continue
		}

		// Failure path: empty ShardID means the orchestrator could not assign.
		if payload.GameWorld.ShardID == "" {
			reason := payload.Reason
			if reason == "" {
				reason = "no game shard available"
			}
			abortAwaitingAllocation(&state.StartSessionResults, lobbyEntity.Lobby, &lobby, reason)
			continue
		}

		requestID := lobby.Session.PendingRequestID
		lobby.Session.PendingRequestID = ""
		lobby.Session.PendingStartedAt = 0

		lobby.GameWorld = payload.GameWorld
		lobby.Session.State = component.SessionStateInSession
		lobbyEntity.Lobby.Set(lobby)

		state.Logger().Info().
			Str("lobby_id", payload.LobbyID).
			Str("game_shard", lobby.GameWorld.ShardID).
			Str("invite_code", lobby.InviteCode).
			Msg("Session started (async assignment)")

		state.SessionStartedEvents.Broadcast(SessionStartedEvent{
			LobbyID:   payload.LobbyID,
			GameWorld: lobby.GameWorld,
		})

		dispatchSessionStart(state, config, &lobby, payload.LobbyID)

		state.StartSessionResults.Broadcast(StartSessionResult{
			RequestID: requestID,
			IsSuccess: true,
			Message:   "session started",
			GameWorld: lobby.GameWorld,
		})
	}
}

func processNotifySessionEndCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
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

		// If the lobby was awaiting allocation when NotifySessionEnd arrives,
		// the session somehow ended before this shard ever assigned one.
		// Fail the pending request so the client unblocks, transition to
		// Idle, and continue. Without this, the lobby would stay stuck.
		if lobby.Session.State == component.SessionStateAwaitingAllocation {
			state.Logger().Warn().
				Str("lobby_id", payload.LobbyID).
				Msg("NotifySessionEndCommand arrived while lobby was awaiting allocation; failing pending request")
			abortAwaitingAllocation(
				&state.StartSessionResults, lobbyEntity.Lobby, &lobby,
				"session ended before shard assignment completed",
			)
			continue
		}

		// Only end if in session
		if lobby.Session.State != component.SessionStateInSession {
			state.Logger().Warn().
				Str("lobby_id", payload.LobbyID).
				Str("state", string(lobby.Session.State)).
				Msg("NotifySessionEndCommand dropped: lobby not in session")
			continue
		}

		lobby.Session.State = component.SessionStateIdle
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

			state.PlayerReadyEvents.Broadcast(PlayerReadyEvent{
				LobbyID: payload.LobbyID,
				Player:  playerComp,
			})
		}

		state.Logger().Info().
			Str("lobby_id", payload.LobbyID).
			Str("invite_code", lobby.InviteCode).
			Msg("Session ended")

		// Emit broadcast event
		state.SessionEndedEvents.Broadcast(SessionEndedEvent(payload))
	}
}

func processGenerateInviteCodeCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.GenerateInviteCodeCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.GenerateInviteCodeResults.Broadcast(GenerateInviteCodeResult{
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
			state.GenerateInviteCodeResults.Broadcast(GenerateInviteCodeResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "only leader can generate invite code",
			})
			continue
		}

		oldCode := lobby.InviteCode

		// Generate new invite code with collision check (max 3 retries)
		newCode, newCodeValid := generateInviteCodeWithRetry(
			lobbyIndex, &lobby, inviteCodeMaxRetries, state.Timestamp().UnixNano(),
		)
		if !newCodeValid {
			state.Logger().Warn().Str("lobby_id", lobbyID).Msg("invite code collision after retries")
			state.GenerateInviteCodeResults.Broadcast(GenerateInviteCodeResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "invite code collision",
			})
			continue
		}

		lobby.InviteCode = newCode
		result.lobbyRef.Set(lobby)

		lobbyIndex.UpdateInviteCode(lobbyID, oldCode, newCode)

		// old_invite_code is the one a game dev will be holding when they report a code
		// that stopped working. Logging only the new one leaves the retired code with no
		// death record, which is indistinguishable from the server losing it.
		state.Logger().Info().
			Str("lobby_id", lobbyID).
			Str("invite_code", newCode).
			Str("old_invite_code", oldCode).
			Str("rotated_by", playerID).
			Msg("New invite code generated")

		// Emit broadcast event
		state.InviteCodeGeneratedEvents.Broadcast(InviteCodeGeneratedEvent{
			LobbyID:    lobbyID,
			InviteCode: newCode,
		})

		state.GenerateInviteCodeResults.Broadcast(GenerateInviteCodeResult{
			RequestID:  payload.RequestID,
			IsSuccess:  true,
			Message:    "invite code generated",
			InviteCode: newCode,
		})
	}
}

func processUpdateSessionPassthroughCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.UpdateSessionPassthroughCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.UpdateSessionPassthroughResults.Broadcast(UpdateSessionPassthroughResult{
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
			state.UpdateSessionPassthroughResults.Broadcast(UpdateSessionPassthroughResult{
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
			Str("invite_code", lobby.InviteCode).
			Msg("Session passthrough data updated")

		// Emit broadcast event
		state.SessionPassthroughUpdatedEvents.Broadcast(SessionPassthroughUpdatedEvent{
			LobbyID:         lobbyID,
			PassthroughData: lobby.Session.PassthroughData,
		})

		state.UpdateSessionPassthroughResults.Broadcast(UpdateSessionPassthroughResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "session passthrough data updated",
		})
	}
}

func processUpdatePlayerPassthroughCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.UpdatePlayerPassthroughCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.UpdatePlayerPassthroughResults.Broadcast(UpdatePlayerPassthroughResult{
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
			state.UpdatePlayerPassthroughResults.Broadcast(UpdatePlayerPassthroughResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}
		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			state.UpdatePlayerPassthroughResults.Broadcast(UpdatePlayerPassthroughResult{
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
			Str("invite_code", result.lobby.InviteCode).
			Msg("Player passthrough data updated")

		// Emit broadcast event
		state.PlayerPassthroughUpdatedEvents.Broadcast(PlayerPassthroughUpdatedEvent{
			LobbyID: lobbyID,
			Player:  playerComp,
		})

		state.UpdatePlayerPassthroughResults.Broadcast(UpdatePlayerPassthroughResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "player passthrough data updated",
			Player:    playerComp,
		})
	}
}

func processGetPlayerCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
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
			state.GetPlayerResults.Broadcast(GetPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not found",
			})
			continue
		}

		playerEntity, err := state.Players.GetByID(cardinal.EntityID(playerEntityID))
		if err != nil {
			state.GetPlayerResults.Broadcast(GetPlayerResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player entity not found",
			})
			continue
		}

		playerComp := playerEntity.Player.Get()

		state.GetPlayerResults.Broadcast(GetPlayerResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "player found",
			Player:    playerComp,
		})
	}
}

func processGetLobbyCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.GetLobbyCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.GetLobbyResults.Broadcast(GetLobbyResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}

		state.GetLobbyResults.Broadcast(GetLobbyResult{
			RequestID: payload.RequestID,
			IsSuccess: true,
			Message:   "lobby found",
			Lobby:     result.lobby,
		})
	}
}

func processGetAllPlayersCommands(state *LobbySystemState, lobbyIndex *lookupIndex) {
	for cmd := range state.GetAllPlayersCmds.Iter() {
		playerID := cmd.Persona
		payload := cmd.Payload

		// Get caller's lobby
		result := getPlayerLobby(playerID, lobbyIndex, &state.Lobbies)
		if result == nil {
			state.GetAllPlayersResults.Broadcast(GetAllPlayersResult{
				RequestID: payload.RequestID,
				IsSuccess: false,
				Message:   "player not in a lobby",
			})
			continue
		}

		lobby := result.lobby

		// Get all player components
		players, playersCount := gatherLobbyPlayers(state, lobbyIndex, &lobby)

		state.GetAllPlayersResults.Broadcast(GetAllPlayersResult{
			RequestID:    payload.RequestID,
			IsSuccess:    true,
			Message:      "players found",
			Players:      players,
			PlayersCount: playersCount,
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

	// Events
	PlayerTimedOutEvents cardinal.WithEvent[PlayerTimedOutEvent]
	LeaderChangedEvents  cardinal.WithEvent[LeaderChangedEvent]
	LobbyDeletedEvents   cardinal.WithEvent[LobbyDeletedEvent]
	StartSessionResults  cardinal.WithEvent[StartSessionResult]
}

// HeartbeatSystem processes heartbeat commands and removes stale players.
func HeartbeatSystem(state *HeartbeatSystemState) {
	now := state.Timestamp().Unix()

	// LobbySystem rebuilds the index earlier in the same tick. Guarding here rather than relying on
	// that registration order: after World.reset() the flag is false while index still holds the
	// previous world's entity IDs, so running first would evict against stale deadlines and destroy
	// entities by stale ID.
	if !indexBuilt {
		return
	}
	lobbyIndex := &index

	// Debug: print deadline map state
	state.Logger().Debug().
		Interface("deadline_map", lobbyIndex.PlayerDeadline).
		Int64("now", now).
		Msg("HeartbeatSystem tick")

	config := storedConfig

	// Get timeout for deadline
	timeout := config.HeartbeatTimeout
	if timeout <= 0 {
		timeout = 30 // default 30 seconds
	}

	// Process heartbeat commands - update deadline for senders
	processHeartbeatCommands(state, lobbyIndex, now, timeout)

	// Find timed out players - O(allPlayers)
	timedOutPlayers := findTimedOutPlayers(lobbyIndex, now)

	// Early exit if no players timed out
	if len(timedOutPlayers) == 0 {
		return
	}

	// Group timed out players by lobby for efficient processing
	timedOutByLobby := groupPlayersByLobby(timedOutPlayers)

	// Process each affected lobby
	var lobbiesToDestroy []lobbyToDestroy
	var playerEntitiesToDestroy []cardinal.EntityID
	for lobbyID, players := range timedOutByLobby {
		playerEntities, toDestroy := processTimedOutLobby(state, lobbyIndex, lobbyID, players)
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
		failPendingAssignment(&state.StartSessionResults, &toDestroy.lobby, "lobby deleted (timeout) before shard assignment")
		state.Lobbies.Destroy(toDestroy.entityID)
		state.Logger().Info().
			Str("lobby_id", toDestroy.lobbyID).
			Str("invite_code", toDestroy.lobby.InviteCode).
			Msg("Lobby deleted (empty after timeout)")
	}
}
