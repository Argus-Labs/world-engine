package component

// SessionState represents the current state of a lobby session.
type SessionState string

const (
	SessionStateIdle      SessionState = "idle"       // Lobby is waiting, not in a session
	SessionStateInSession SessionState = "in_session" // Lobby is currently in a game session
)

// PlayerState represents a player's state in a team.
type PlayerState struct {
	PlayerID        string         `json:"player_id"`
	IsReady         bool           `json:"is_ready"`
	PassthroughData map[string]any `json:"passthrough_data,omitempty"`
}

// Team represents a team within a lobby.
type Team struct {
	TeamID     string        `json:"team_id"`
	Name       string        `json:"name"`
	Players    []PlayerState `json:"players"`
	MaxPlayers int           `json:"max_players"`
}

// Session represents the current session state of a lobby.
type Session struct {
	State           SessionState   `json:"state"`
	PassthroughData map[string]any `json:"passthrough_data,omitempty"`
}

// LobbyComponent represents a lobby where players gather.
type LobbyComponent struct {
	// ID is the unique identifier for the lobby.
	ID string `json:"id"`

	// LeaderID is the player who controls the lobby.
	LeaderID string `json:"leader_id"`

	// Teams is the list of teams in the lobby.
	Teams []Team `json:"teams"`

	// InviteCode is the code for others to join.
	InviteCode string `json:"invite_code"`

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
		count += len(team.Players)
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

// HasPlayer returns true if the player is in any team.
func (l *LobbyComponent) HasPlayer(playerID string) bool {
	for _, team := range l.Teams {
		for _, p := range team.Players {
			if p.PlayerID == playerID {
				return true
			}
		}
	}
	return false
}

// GetPlayer returns the player state for a given player ID.
func (l *LobbyComponent) GetPlayer(playerID string) *PlayerState {
	for i := range l.Teams {
		for j := range l.Teams[i].Players {
			if l.Teams[i].Players[j].PlayerID == playerID {
				return &l.Teams[i].Players[j]
			}
		}
	}
	return nil
}

// GetPlayerTeam returns the team that contains the player.
func (l *LobbyComponent) GetPlayerTeam(playerID string) *Team {
	for i := range l.Teams {
		for _, p := range l.Teams[i].Players {
			if p.PlayerID == playerID {
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

// AllReady returns true if all players in all teams are ready.
func (l *LobbyComponent) AllReady() bool {
	totalPlayers := 0
	for _, team := range l.Teams {
		for _, p := range team.Players {
			totalPlayers++
			if !p.IsReady {
				return false
			}
		}
	}
	return totalPlayers > 0
}

// AddPlayerToTeam adds a player to a specific team.
func (l *LobbyComponent) AddPlayerToTeam(playerID, teamID string) bool {
	if l.HasPlayer(playerID) {
		return false
	}
	team := l.GetTeam(teamID)
	if team == nil {
		return false
	}
	if team.MaxPlayers > 0 && len(team.Players) >= team.MaxPlayers {
		return false
	}
	team.Players = append(team.Players, PlayerState{
		PlayerID: playerID,
		IsReady:  false,
	})
	return true
}

// RemovePlayer removes a player from their team.
func (l *LobbyComponent) RemovePlayer(playerID string) {
	for i := range l.Teams {
		for j, p := range l.Teams[i].Players {
			if p.PlayerID == playerID {
				l.Teams[i].Players = append(l.Teams[i].Players[:j], l.Teams[i].Players[j+1:]...)
				return
			}
		}
	}
}

// SetReady sets the ready status for a player.
func (l *LobbyComponent) SetReady(playerID string, isReady bool) {
	for i := range l.Teams {
		for j := range l.Teams[i].Players {
			if l.Teams[i].Players[j].PlayerID == playerID {
				l.Teams[i].Players[j].IsReady = isReady
				return
			}
		}
	}
}

// MovePlayerToTeam moves a player from their current team to a new team.
func (l *LobbyComponent) MovePlayerToTeam(playerID, newTeamID string) bool {
	// Find current player state
	var playerState *PlayerState
	for i := range l.Teams {
		for j := range l.Teams[i].Players {
			if l.Teams[i].Players[j].PlayerID == playerID {
				playerState = &l.Teams[i].Players[j]
				break
			}
		}
		if playerState != nil {
			break
		}
	}
	if playerState == nil {
		return false
	}

	// Check new team exists and has space
	newTeam := l.GetTeam(newTeamID)
	if newTeam == nil {
		return false
	}
	if newTeam.MaxPlayers > 0 && len(newTeam.Players) >= newTeam.MaxPlayers {
		return false
	}

	// Copy player state
	copiedState := *playerState

	// Remove from current team
	l.RemovePlayer(playerID)

	// Add to new team
	newTeam.Players = append(newTeam.Players, copiedState)
	return true
}

// AddTeam adds a new team to the lobby.
func (l *LobbyComponent) AddTeam(team Team) {
	l.Teams = append(l.Teams, team)
}

// IsTeamFull returns true if the team is at max capacity.
func (t *Team) IsFull() bool {
	return t.MaxPlayers > 0 && len(t.Players) >= t.MaxPlayers
}

// LobbyIndexComponent provides O(1) lookups for lobbies.
// This is a singleton component - only one entity should have it.
type LobbyIndexComponent struct {
	// LobbyIDToEntity maps LobbyID -> EntityID for O(1) lookup
	LobbyIDToEntity map[string]uint32 `json:"lobby_id_to_entity"`

	// InviteCodeToLobby maps InviteCode -> LobbyID for join lookups
	InviteCodeToLobby map[string]string `json:"invite_code_to_lobby"`

	// PlayerToLobby maps PlayerID -> LobbyID for "my lobby" lookups
	PlayerToLobby map[string]string `json:"player_to_lobby"`
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

// AddPlayerToLobby maps a player to a lobby.
func (idx *LobbyIndexComponent) AddPlayerToLobby(playerID, lobbyID string) {
	idx.Init()
	idx.PlayerToLobby[playerID] = lobbyID
}

// RemovePlayerFromLobby removes a player's lobby mapping.
func (idx *LobbyIndexComponent) RemovePlayerFromLobby(playerID string) {
	delete(idx.PlayerToLobby, playerID)
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
	// GameShardID is the shard ID for the game shard.
	GameShardID string `json:"game_shard_id,omitempty"`

	// GameRegion is the region for the game shard.
	GameRegion string `json:"game_region,omitempty"`

	// GameOrganization is the organization for the game shard.
	GameOrganization string `json:"game_organization,omitempty"`

	// GameProject is the project for the game shard.
	GameProject string `json:"game_project,omitempty"`

	// LobbyShardAddress is this lobby shard's full address string.
	LobbyShardAddress string `json:"lobby_shard_address,omitempty"`
}

// Name returns the component name for ECS registration.
func (ConfigComponent) Name() string { return "lobby_config" }
