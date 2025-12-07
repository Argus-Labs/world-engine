package lobby

// Party Management Commands (client)

// CreatePartyCommand creates a new party with the sender as leader.
type CreatePartyCommand struct {
	PlayerID string `json:"player_id"`
	IsOpen   bool   `json:"is_open"`
	MaxSize  int    `json:"max_size"`
}

func (c CreatePartyCommand) Name() string { return "create-party" }

// JoinPartyCommand adds a player to an existing party.
type JoinPartyCommand struct {
	PartyID  string `json:"party_id"`
	PlayerID string `json:"player_id"`
}

func (c JoinPartyCommand) Name() string { return "join-party" }

// LeavePartyCommand removes a player from their party.
type LeavePartyCommand struct {
	PartyID  string `json:"party_id"`
	PlayerID string `json:"player_id"`
}

func (c LeavePartyCommand) Name() string { return "leave-party" }

// KickFromPartyCommand removes a player from the party (leader only).
type KickFromPartyCommand struct {
	PartyID        string `json:"party_id"`
	LeaderID       string `json:"leader_id"`
	TargetPlayerID string `json:"target_player_id"`
}

func (c KickFromPartyCommand) Name() string { return "kick-from-party" }

// DisbandPartyCommand disbands the entire party (leader only).
type DisbandPartyCommand struct {
	PartyID  string `json:"party_id"`
	LeaderID string `json:"leader_id"`
}

func (c DisbandPartyCommand) Name() string { return "disband-party" }

// SetPartyLeaderCommand changes the party leader.
type SetPartyLeaderCommand struct {
	PartyID         string `json:"party_id"`
	CurrentLeaderID string `json:"current_leader_id"`
	NewLeaderID     string `json:"new_leader_id"`
}

func (c SetPartyLeaderCommand) Name() string { return "set-party-leader" }

// SetPartyOpenCommand sets whether the party is open for joining.
type SetPartyOpenCommand struct {
	PartyID  string `json:"party_id"`
	LeaderID string `json:"leader_id"`
	IsOpen   bool   `json:"is_open"`
}

func (c SetPartyOpenCommand) Name() string { return "set-party-open" }

// Lobby Management Commands (client)

// CreateLobbyCommand creates a new manual lobby.
type CreateLobbyCommand struct {
	PartyID    string         `json:"party_id"`
	MinPlayers int            `json:"min_players"`
	MaxPlayers int            `json:"max_players"`
	Config     map[string]any `json:"config,omitempty"`
}

func (c CreateLobbyCommand) Name() string { return "create-lobby" }

// JoinLobbyCommand adds a party to a lobby.
type JoinLobbyCommand struct {
	LobbyID string `json:"lobby_id"`
	PartyID string `json:"party_id"`
}

func (c JoinLobbyCommand) Name() string { return "join-lobby" }

// LeaveLobbyCommand removes a party from a lobby.
type LeaveLobbyCommand struct {
	LobbyID string `json:"lobby_id"`
	PartyID string `json:"party_id"`
}

func (c LeaveLobbyCommand) Name() string { return "leave-lobby" }

// KickFromLobbyCommand removes a party from lobby (host only).
type KickFromLobbyCommand struct {
	LobbyID       string `json:"lobby_id"`
	HostPartyID   string `json:"host_party_id"`
	TargetPartyID string `json:"target_party_id"`
}

func (c KickFromLobbyCommand) Name() string { return "kick-from-lobby" }

// CloseLobbyCommand closes the lobby (host only).
type CloseLobbyCommand struct {
	LobbyID     string `json:"lobby_id"`
	HostPartyID string `json:"host_party_id"`
}

func (c CloseLobbyCommand) Name() string { return "close-lobby" }

// Ready/Match Lifecycle Commands (client)

// SetReadyCommand marks a party as ready.
type SetReadyCommand struct {
	LobbyID string `json:"lobby_id"`
	PartyID string `json:"party_id"`
}

func (c SetReadyCommand) Name() string { return "set-ready" }

// UnsetReadyCommand marks a party as not ready.
type UnsetReadyCommand struct {
	LobbyID string `json:"lobby_id"`
	PartyID string `json:"party_id"`
}

func (c UnsetReadyCommand) Name() string { return "unset-ready" }

// StartMatchCommand starts the match (host only, when all ready).
type StartMatchCommand struct {
	LobbyID     string `json:"lobby_id"`
	HostPartyID string `json:"host_party_id"`
}

func (c StartMatchCommand) Name() string { return "start-match" }

// EndMatchCommand ends the match.
type EndMatchCommand struct {
	LobbyID string `json:"lobby_id"`
}

func (c EndMatchCommand) Name() string { return "end-match" }

// Internal Commands (from Game Shard)

// HeartbeatCommand is sent by the game shard to indicate the match is still active.
type HeartbeatCommand struct {
	LobbyID string `json:"lobby_id"`
}

func (c HeartbeatCommand) Name() string { return "heartbeat" }

// SetPlayerStatusCommand is sent by Game Shard to update a player's connection status during in_game.
type SetPlayerStatusCommand struct {
	LobbyID   string `json:"lobby_id"`
	PartyID   string `json:"party_id"`
	Connected bool   `json:"connected"`
}

func (c SetPlayerStatusCommand) Name() string { return "set-player-status" }

// Note: RequestBackfill and CancelBackfill are handled via service endpoints (not commands)
// See service.go handleRequestBackfill and handleCancelBackfill

// =============================================================================
// Internal Commands (for deterministic state changes from service handlers)
// =============================================================================
// These commands are queued by service handlers and processed in Tick() to ensure
// deterministic state changes. They are not exposed via NATS endpoints.

// InternalCommand is the interface for internal commands that bypass NATS routing.
type InternalCommand interface {
	InternalName() string
}

// ReceiveMatchInternalCommand handles a match received from Matchmaking Shard.
type ReceiveMatchInternalCommand struct {
	MatchID            string              `json:"match_id"`
	MatchProfileName   string              `json:"match_profile_name"`
	Teams              []MatchTeam         `json:"teams"`
	Config             map[string]any      `json:"config,omitempty"`
	MatchmakingAddress *ServiceAddressJSON `json:"matchmaking_address,omitempty"`
	TargetAddress      *ServiceAddressJSON `json:"target_address,omitempty"`
}

func (c ReceiveMatchInternalCommand) InternalName() string { return "receive-match" }

// MatchTeam represents a team in a match for internal commands.
type MatchTeam struct {
	Name    string        `json:"name"`
	Tickets []MatchTicket `json:"tickets"`
}

// MatchTicket represents a ticket in a match for internal commands.
type MatchTicket struct {
	ID        string   `json:"id"`
	PlayerIDs []string `json:"player_ids"`
}

// ServiceAddressJSON is a JSON-serializable version of ServiceAddress for internal commands.
type ServiceAddressJSON struct {
	Region       string `json:"region"`
	Realm        string `json:"realm"`
	Organization string `json:"organization"`
	Project      string `json:"project"`
	ServiceID    string `json:"service_id"`
}

// ReceiveBackfillMatchInternalCommand handles a backfill match received from Matchmaking Shard.
type ReceiveBackfillMatchInternalCommand struct {
	BackfillRequestID string        `json:"backfill_request_id"`
	MatchID           string        `json:"match_id"`
	TeamName          string        `json:"team_name"`
	Tickets           []MatchTicket `json:"tickets"`
}

func (c ReceiveBackfillMatchInternalCommand) InternalName() string { return "receive-backfill-match" }

// GameHeartbeatInternalCommand handles heartbeat from Game Shard.
type GameHeartbeatInternalCommand struct {
	MatchID string `json:"match_id"`
}

func (c GameHeartbeatInternalCommand) InternalName() string { return "game-heartbeat" }

// GamePlayerStatusInternalCommand handles player status from Game Shard.
type GamePlayerStatusInternalCommand struct {
	MatchID   string `json:"match_id"`
	PlayerID  string `json:"player_id"`
	Connected bool   `json:"connected"`
}

func (c GamePlayerStatusInternalCommand) InternalName() string { return "game-player-status" }

// GameEndMatchInternalCommand handles end-match from Game Shard.
type GameEndMatchInternalCommand struct {
	MatchID string `json:"match_id"`
	Result  string `json:"result"`
}

func (c GameEndMatchInternalCommand) InternalName() string { return "game-end-match" }
