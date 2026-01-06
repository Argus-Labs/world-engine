package component

import "github.com/argus-labs/world-engine/pkg/cardinal"

// SessionState represents the current state of a lobby session.
type SessionState string

const (
	SessionStateIdle      SessionState = "idle"       // Lobby is waiting, not in a session
	SessionStateInSession SessionState = "in_session" // Lobby is currently in a game session
)

// PlayerComponent represents a player entity in a lobby.
// Players are created when joining a lobby and deleted when leaving.
type PlayerComponent struct {
	PlayerID        string         `json:"player_id"`
	LobbyID         string         `json:"lobby_id"`
	TeamID          string         `json:"team_id"`
	IsReady         bool           `json:"is_ready"`
	PassthroughData map[string]any `json:"passthrough_data,omitempty"`
	JoinedAt        int64          `json:"joined_at"` // Unix timestamp when player joined
}

// Name returns the component name for ECS registration.
func (PlayerComponent) Name() string { return "player" }

// Team represents a team within a lobby.
type Team struct {
	TeamID     string   `json:"team_id"`
	Name       string   `json:"name"`
	PlayerIDs  []string `json:"player_ids"` // References to player IDs (source of truth)
	MaxPlayers int      `json:"max_players"`
}

// Session represents the current session state of a lobby.
type Session struct {
	State           SessionState   `json:"state"`
	PassthroughData map[string]any `json:"passthrough_data,omitempty"`
}

// LobbyComponent represents a lobby where players gather.
// Following rampage-backend pattern: Name() uses value receiver for ecs.Component interface,
// helper methods use pointer receivers. This may change based on best practices.
//
//nolint:recvcheck // Name must be value receiver for ecs.Component; helpers use pointer receivers.
type LobbyComponent struct {
	// ID is the unique identifier for the lobby.
	ID string `json:"id"`

	// LeaderID is the player who controls the lobby.
	LeaderID string `json:"leader_id"`

	// Teams is the list of teams in the lobby.
	Teams []Team `json:"teams"`

	// InviteCode is the code for others to join.
	InviteCode string `json:"invite_code"`

	// GameWorld is the target game shard address.
	GameWorld cardinal.OtherWorld `json:"game_world"`

	// Session is the current session state.
	Session Session `json:"session"`

	// CreatedAt is the Unix timestamp when the lobby was created.
	CreatedAt int64 `json:"created_at"`
}

// Name returns the component name for ECS registration.
func (LobbyComponent) Name() string { return "lobby" }

// PlayerCount returns the total number of players across all teams.
func (l *LobbyComponent) PlayerCount() int {
	count := 0
	for _, team := range l.Teams {
		count += len(team.PlayerIDs)
	}
	return count
}

// GetTeam returns a team by ID.
func (l *LobbyComponent) GetTeam(teamID string) *Team {
	for i := range l.Teams {
		if l.Teams[i].TeamID == teamID {
			return &l.Teams[i]
		}
	}
	return nil
}

// GetTeamByName returns a team by its name.
func (l *LobbyComponent) GetTeamByName(name string) *Team {
	for i := range l.Teams {
		if l.Teams[i].Name == name {
			return &l.Teams[i]
		}
	}
	return nil
}

// HasPlayer returns true if the player is in any team.
func (l *LobbyComponent) HasPlayer(playerID string) bool {
	for _, team := range l.Teams {
		for _, pid := range team.PlayerIDs {
			if pid == playerID {
				return true
			}
		}
	}
	return false
}

// GetPlayerTeam returns the team that contains the player.
func (l *LobbyComponent) GetPlayerTeam(playerID string) *Team {
	for i := range l.Teams {
		for _, pid := range l.Teams[i].PlayerIDs {
			if pid == playerID {
				return &l.Teams[i]
			}
		}
	}
	return nil
}

// IsLeader returns true if the player is the lobby leader.
func (l *LobbyComponent) IsLeader(playerID string) bool {
	return l.LeaderID == playerID
}

// AddPlayerToTeam adds a player ID to a specific team.
// Returns false if player already in lobby, team doesn't exist, or team is full.
// Note: This only updates the lobby's team membership. PlayerComponent entity must be created separately.
func (l *LobbyComponent) AddPlayerToTeam(playerID, teamID string) bool {
	if l.HasPlayer(playerID) {
		return false
	}
	team := l.GetTeam(teamID)
	if team == nil {
		return false
	}
	if team.MaxPlayers > 0 && len(team.PlayerIDs) >= team.MaxPlayers {
		return false
	}
	team.PlayerIDs = append(team.PlayerIDs, playerID)
	return true
}

// RemovePlayer removes a player ID from their team.
// Note: This only updates the lobby's team membership. PlayerComponent entity must be deleted separately.
func (l *LobbyComponent) RemovePlayer(playerID string) {
	for i := range l.Teams {
		for j, pid := range l.Teams[i].PlayerIDs {
			if pid == playerID {
				l.Teams[i].PlayerIDs = append(l.Teams[i].PlayerIDs[:j], l.Teams[i].PlayerIDs[j+1:]...)
				return
			}
		}
	}
}

// RemovePlayerFromTeam removes a player ID from a specific team. O(players in team).
// Note: This only updates the lobby's team membership. PlayerComponent entity must be deleted separately.
func (l *LobbyComponent) RemovePlayerFromTeam(playerID, teamID string) bool {
	team := l.GetTeam(teamID)
	if team == nil {
		return false
	}
	for j, pid := range team.PlayerIDs {
		if pid == playerID {
			team.PlayerIDs = append(team.PlayerIDs[:j], team.PlayerIDs[j+1:]...)
			return true
		}
	}
	return false
}

// MovePlayerToTeam moves a player from their current team to a new team.
// Returns false if player not in lobby, new team doesn't exist, or new team is full.
// Note: PlayerComponent.TeamID must be updated separately.
func (l *LobbyComponent) MovePlayerToTeam(playerID, newTeamID string) bool {
	// Check player exists
	if !l.HasPlayer(playerID) {
		return false
	}

	// Check new team exists
	newTeam := l.GetTeam(newTeamID)
	if newTeam == nil {
		return false
	}

	// Already in target team - no-op
	for _, pid := range newTeam.PlayerIDs {
		if pid == playerID {
			return true
		}
	}

	// Check capacity
	if newTeam.MaxPlayers > 0 && len(newTeam.PlayerIDs) >= newTeam.MaxPlayers {
		return false
	}

	// Remove from current team
	l.RemovePlayer(playerID)

	// Add to new team
	newTeam.PlayerIDs = append(newTeam.PlayerIDs, playerID)
	return true
}

// AddTeam adds a new team to the lobby.
func (l *LobbyComponent) AddTeam(team Team) {
	l.Teams = append(l.Teams, team)
}

// GetAllPlayerIDs returns all player IDs across all teams.
func (l *LobbyComponent) GetAllPlayerIDs() []string {
	var playerIDs []string
	for _, team := range l.Teams {
		playerIDs = append(playerIDs, team.PlayerIDs...)
	}
	return playerIDs
}

// IsTeamFull returns true if the team is at max capacity.
func (t *Team) IsFull() bool {
	return t.MaxPlayers > 0 && len(t.PlayerIDs) >= t.MaxPlayers
}

// LobbyIndexComponent provides O(1) lookups for lobbies and players.
// This is a singleton component - only one entity should have it.
// Following rampage-backend pattern: Name() uses value receiver for ecs.Component interface,
// helper methods use pointer receivers. This may change based on best practices.
//
//nolint:recvcheck // Name must be value receiver for ecs.Component; helpers use pointer receivers.
type LobbyIndexComponent struct {
	// LobbyIDToEntity maps LobbyID -> EntityID for O(1) lookup
	LobbyIDToEntity map[string]uint32 `json:"lobby_id_to_entity"`

	// InviteCodeToLobby maps InviteCode -> LobbyID for join lookups
	InviteCodeToLobby map[string]string `json:"invite_code_to_lobby"`

	// PlayerToLobby maps PlayerID -> LobbyID for "my lobby" lookups
	PlayerToLobby map[string]string `json:"player_to_lobby"`

	// PlayerToTeam maps PlayerID -> TeamID for O(1) team lookup
	PlayerToTeam map[string]string `json:"player_to_team"`

	// PlayerToEntity maps PlayerID -> EntityID for O(1) player entity lookup
	PlayerToEntity map[string]uint32 `json:"player_to_entity"`

	// PlayerDeadline maps PlayerID -> Unix timestamp when player will be kicked if no heartbeat
	// This enables O(1) heartbeat updates instead of O(teams × players) lookups
	PlayerDeadline map[string]int64 `json:"player_deadline"`

	// LobbyPlayerCount maps LobbyID -> player count for O(1) count lookup
	LobbyPlayerCount map[string]int `json:"lobby_player_count"`
}

// Name returns the component name for ECS registration.
func (LobbyIndexComponent) Name() string { return "lobby_index" }

// Init initializes the maps if nil.
func (idx *LobbyIndexComponent) Init() {
	if idx.LobbyIDToEntity == nil {
		idx.LobbyIDToEntity = make(map[string]uint32)
	}
	if idx.InviteCodeToLobby == nil {
		idx.InviteCodeToLobby = make(map[string]string)
	}
	if idx.PlayerToLobby == nil {
		idx.PlayerToLobby = make(map[string]string)
	}
	if idx.PlayerToTeam == nil {
		idx.PlayerToTeam = make(map[string]string)
	}
	if idx.PlayerToEntity == nil {
		idx.PlayerToEntity = make(map[string]uint32)
	}
	if idx.PlayerDeadline == nil {
		idx.PlayerDeadline = make(map[string]int64)
	}
	if idx.LobbyPlayerCount == nil {
		idx.LobbyPlayerCount = make(map[string]int)
	}
}

// GetEntityID returns the entity ID for a lobby.
func (idx *LobbyIndexComponent) GetEntityID(lobbyID string) (uint32, bool) {
	eid, exists := idx.LobbyIDToEntity[lobbyID]
	return eid, exists
}

// GetLobbyByInviteCode returns the lobby ID for an invite code.
func (idx *LobbyIndexComponent) GetLobbyByInviteCode(inviteCode string) (string, bool) {
	lobbyID, exists := idx.InviteCodeToLobby[inviteCode]
	return lobbyID, exists
}

// GetPlayerLobby returns the lobby ID for a player.
func (idx *LobbyIndexComponent) GetPlayerLobby(playerID string) (string, bool) {
	lobbyID, exists := idx.PlayerToLobby[playerID]
	return lobbyID, exists
}

// AddLobby adds a lobby to the index.
func (idx *LobbyIndexComponent) AddLobby(lobbyID string, entityID uint32, inviteCode string) {
	idx.Init()
	idx.LobbyIDToEntity[lobbyID] = entityID
	if inviteCode != "" {
		idx.InviteCodeToLobby[inviteCode] = lobbyID
	}
}

// RemoveLobby removes a lobby from the index.
func (idx *LobbyIndexComponent) RemoveLobby(lobbyID string, inviteCode string) {
	delete(idx.LobbyIDToEntity, lobbyID)
	if inviteCode != "" {
		delete(idx.InviteCodeToLobby, inviteCode)
	}
}

// AddPlayerToLobby maps a player to a lobby, team, entity ID, and sets their deadline.
func (idx *LobbyIndexComponent) AddPlayerToLobby(playerID, lobbyID, teamID string, entityID uint32, deadline int64) {
	idx.Init()
	idx.PlayerToLobby[playerID] = lobbyID
	idx.PlayerToTeam[playerID] = teamID
	idx.PlayerToEntity[playerID] = entityID
	idx.PlayerDeadline[playerID] = deadline
	idx.LobbyPlayerCount[lobbyID]++
}

// RemovePlayerFromLobby removes a player's lobby mapping, team mapping, entity mapping, and deadline.
func (idx *LobbyIndexComponent) RemovePlayerFromLobby(playerID string) {
	if lobbyID, exists := idx.PlayerToLobby[playerID]; exists {
		idx.LobbyPlayerCount[lobbyID]--
		if idx.LobbyPlayerCount[lobbyID] <= 0 {
			delete(idx.LobbyPlayerCount, lobbyID)
		}
	}
	delete(idx.PlayerToLobby, playerID)
	delete(idx.PlayerToTeam, playerID)
	delete(idx.PlayerToEntity, playerID)
	delete(idx.PlayerDeadline, playerID)
}

// GetPlayerEntityID returns the entity ID for a player.
func (idx *LobbyIndexComponent) GetPlayerEntityID(playerID string) (uint32, bool) {
	eid, exists := idx.PlayerToEntity[playerID]
	return eid, exists
}

// UpdatePlayerDeadline updates the deadline for a player.
func (idx *LobbyIndexComponent) UpdatePlayerDeadline(playerID string, deadline int64) {
	idx.PlayerDeadline[playerID] = deadline
}

// GetPlayerDeadline returns the deadline for a player.
func (idx *LobbyIndexComponent) GetPlayerDeadline(playerID string) (int64, bool) {
	deadline, exists := idx.PlayerDeadline[playerID]
	return deadline, exists
}

// GetPlayerTeam returns the team ID for a player.
func (idx *LobbyIndexComponent) GetPlayerTeam(playerID string) (string, bool) {
	teamID, exists := idx.PlayerToTeam[playerID]
	return teamID, exists
}

// UpdatePlayerTeam updates the team ID for a player.
func (idx *LobbyIndexComponent) UpdatePlayerTeam(playerID, teamID string) {
	idx.PlayerToTeam[playerID] = teamID
}

// GetLobbyPlayerCount returns the player count for a lobby.
func (idx *LobbyIndexComponent) GetLobbyPlayerCount(lobbyID string) int {
	return idx.LobbyPlayerCount[lobbyID]
}

// HasPlayer returns true if player exists in the index (O(1)).
func (idx *LobbyIndexComponent) HasPlayer(playerID string) bool {
	_, exists := idx.PlayerToLobby[playerID]
	return exists
}

// UpdateInviteCode updates the invite code for a lobby.
func (idx *LobbyIndexComponent) UpdateInviteCode(lobbyID, oldCode, newCode string) {
	if oldCode != "" {
		delete(idx.InviteCodeToLobby, oldCode)
	}
	if newCode != "" {
		idx.InviteCodeToLobby[newCode] = lobbyID
	}
}

// ConfigComponent stores lobby configuration.
type ConfigComponent struct {
	// LobbyWorld is this lobby shard's address (for game shard to send NotifySessionEndCommand back).
	LobbyWorld cardinal.OtherWorld `json:"lobby_world"`

	// HeartbeatTimeout is how long (in seconds) before a player is removed for not sending heartbeats.
	// Clients should send heartbeats more frequently than this (e.g., every timeout/3 seconds).
	// Default: 30 seconds.
	HeartbeatTimeout int64 `json:"heartbeat_timeout"`
}

// Name returns the component name for ECS registration.
func (ConfigComponent) Name() string { return "lobby_config" }
