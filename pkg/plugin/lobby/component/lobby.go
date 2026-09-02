package component

// ShardAddress mirrors cardinal.OtherWorld for wire types (commands, events, components). A wire type
// can't carry cardinal.OtherWorld directly — it's cross-module to the generator, which would block. The
// fields are identical, so the two convert with a plain Go struct cast: cardinal.OtherWorld(a) and back.
//
// TODO(cross-module): this mirror duplicates cardinal.OtherWorld. Since the wire is proto, the cleaner
// long-term fix is to import world-engine's committed other_world.proto instead of mirroring — deferred,
// the import path (per-module type→proto manifest + buf staging) is high effort. See sdkgen.go.
type ShardAddress struct {
	Region       string
	Organization string
	Project      string
	ShardID      string
}

// Structural ceilings for the lobby roster. These are storage bounds, not game rules: the game rule is
// Team.MaxPlayers, which comes from the preset and stays configurable per deployment. A lobby cannot
// exceed these no matter what a preset asks for, because component data has to be fixed-size — a slice
// field would make a component copy alias the live world.
//
// Raising either is safe for existing snapshots: the generated decoder writes what arrives and zeroes
// the rest. Lowering one is survivable but lossy — the decoder caps the array writes yet copies
// PlayerCount and TeamCount verbatim, so a snapshot from a larger build restores a count the array
// cannot satisfy. Reads go through livePlayers/liveTeams, which clamp, and the first mutation writes
// the clamped value back. Treat these as a floor that only moves up.
const (
	// MaxLobbyPlayers bounds the whole-lobby roster, across all teams.
	MaxLobbyPlayers = 16
	// MaxLobbyTeams bounds how many teams a preset may declare for one lobby.
	MaxLobbyTeams = 4
)

// SessionState represents the current state of a lobby session.
type SessionState string

const (
	SessionStateIdle               SessionState = "idle"                // Lobby is waiting, not in a session
	SessionStateAwaitingAllocation SessionState = "awaiting_allocation" // Lobby is awaiting external shard assignment
	SessionStateInSession          SessionState = "in_session"          // Lobby is currently in a game session
)

// PlayerComponent represents a player entity in a lobby.
// Players are created when joining a lobby and deleted when leaving.
type PlayerComponent struct {
	PlayerID string `json:"player_id"`
	LobbyID  string `json:"lobby_id"`
	// TeamID is the authoritative record of which team this player is on. The lobby's roster tracks
	// membership of the lobby; which team a player belongs to lives here and nowhere else.
	TeamID  string `json:"team_id"`
	IsReady bool   `json:"is_ready"`
	// PassthroughData is client-authored JSON the plugin never reads. It relays the blob between
	// clients unchanged, so it is carried as text rather than decoded — see Session.PassthroughData.
	PassthroughData string `json:"passthrough_data,omitempty"`
	JoinedAt        int64  `json:"joined_at"` // Unix timestamp when player joined
}

// Name returns the component name for ECS registration.
func (PlayerComponent) Name() string { return "player" }

// Team is a team within a lobby: its identity, its capacity rule, and how many players are currently
// charged to it. It holds no roster — the lobby owns one roster for all teams, and a player's team is
// PlayerComponent.TeamID. PlayerCount exists so the capacity check stays O(1) without walking either.
type Team struct {
	// TeamID is the stable identifier used to reference this team in all
	// commands and events. Server-assigned at lobby creation from the
	// preset registry and immutable afterward.
	TeamID string `json:"team_id"`
	// MaxPlayers is the game rule from the preset. MaxPlayers <= 0 means unlimited, which still
	// cannot exceed MaxLobbyPlayers.
	MaxPlayers  int `json:"max_players"`
	PlayerCount int `json:"player_count"`
}

// IsFull reports whether the team is at its configured capacity.
func (t Team) IsFull() bool {
	return t.MaxPlayers > 0 && t.PlayerCount >= t.MaxPlayers
}

// TeamConfig is a creation-time team specification used inside a lobby preset.
// Server operators declare presets in lobby.Config.LobbyPresets; clients choose
// a preset by name on CreateLobbyCommand. MaxPlayers <= 0 means unlimited.
type TeamConfig struct {
	TeamID     string `json:"team_id"`
	MaxPlayers int    `json:"max_players"`
}

// Session represents the current session state of a lobby.
type Session struct {
	State SessionState `json:"state"`
	// PassthroughData is client-authored JSON that the lobby relays between clients without ever
	// reading a key from it. Carried as text, not a decoded map: decoding would buy nothing (no code
	// here indexes it) and a map in a component makes a copy alias the stored value. Validity is the
	// client's business — proto rejects non-UTF-8 at the boundary, which is the only guarantee the
	// shard needs.
	PassthroughData string `json:"passthrough_data,omitempty"`
	// PendingRequestID holds the StartSessionCommand RequestID while the
	// lobby waits in SessionStateAwaitingAllocation for an external shard
	// assignment. Empty in other states.
	PendingRequestID string `json:"pending_request_id,omitempty"`
	// PendingStartedAt records the unix timestamp (seconds) at which the
	// lobby entered SessionStateAwaitingAllocation. Used by timeout
	// enforcement. Zero in other states.
	PendingStartedAt int64 `json:"pending_started_at,omitempty"`
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

	// PlayerIDs is the lobby-wide roster. Only the first PlayerCount entries are live; the rest are
	// zero. Reads go through Players() rather than ranging the array. Order is meaningful: leader
	// succession takes the first entry, so removals shift the tail down rather than swapping.
	PlayerIDs   [MaxLobbyPlayers]string `json:"player_ids"`
	PlayerCount int                     `json:"player_count"`

	// Teams holds the lobby's team configuration and live per-team counts. Only the first TeamCount
	// entries are live.
	Teams     [MaxLobbyTeams]Team `json:"teams"`
	TeamCount int                 `json:"team_count"`

	// InviteCode is the code for others to join.
	InviteCode string `json:"invite_code"`

	// GameWorld is the target game shard address.
	GameWorld ShardAddress `json:"game_world"`

	// Session is the current session state.
	Session Session `json:"session"`

	// CreatedAt is the Unix timestamp when the lobby was created.
	CreatedAt int64 `json:"created_at"`
}

// Name returns the component name for ECS registration.
func (LobbyComponent) Name() string { return "lobby" }

// Players returns the live roster. Callers must not retain it across a mutation of the lobby.
// livePlayers and liveTeams clamp the stored counts to what the arrays can actually hold. The
// generated decoder caps its array writes but copies the counts verbatim, so a snapshot written when
// the caps were larger arrives with a count no read can satisfy. Every read of the roster or the team
// list goes through these.
func (l *LobbyComponent) livePlayers() int { return min(l.PlayerCount, MaxLobbyPlayers) }
func (l *LobbyComponent) liveTeams() int   { return min(l.TeamCount, MaxLobbyTeams) }

func (l *LobbyComponent) Players() []string { return l.PlayerIDs[:l.livePlayers()] }

// TeamList returns the lobby's live teams.
//
// Exported because the clamp has to be reachable from outside this package: iterating TeamCount
// directly indexes past Teams when a snapshot restores a count from a build with a larger
// MaxLobbyTeams, and the entries between the live count and the raw one are zero Teams whose
// MaxPlayers of 0 reads as unlimited — a phantom team with an empty ID that accepts players.
func (l *LobbyComponent) TeamList() []Team { return l.Teams[:l.liveTeams()] }

// GetTeam returns a team by ID, or nil.
func (l *LobbyComponent) GetTeam(teamID string) *Team {
	if teamID == "" {
		return nil // a zero entry has an empty ID; matching it would hand back a team nobody added
	}
	for i := range l.liveTeams() {
		if l.Teams[i].TeamID == teamID {
			return &l.Teams[i]
		}
	}
	return nil
}

// AddTeam appends a team. Returns false if the lobby already holds MaxLobbyTeams.
func (l *LobbyComponent) AddTeam(team Team) bool {
	if l.TeamCount >= MaxLobbyTeams {
		return false
	}
	l.Teams[l.TeamCount] = team
	l.TeamCount++
	return true
}

// HasPlayer returns true if the player is in this lobby.
func (l *LobbyComponent) HasPlayer(playerID string) bool {
	for _, pid := range l.Players() {
		if pid == playerID {
			return true
		}
	}
	return false
}

// IsLeader returns true if the player is the lobby leader.
func (l *LobbyComponent) IsLeader(playerID string) bool {
	return l.LeaderID == playerID
}

// GetAllPlayerIDs returns all player IDs in the lobby.
func (l *LobbyComponent) GetAllPlayerIDs() []string {
	return l.Players()
}

// AddPlayerToTeam records a player in the roster and charges them to a team.
// Returns false if the player is already here, the roster is at MaxLobbyPlayers, the team does not
// exist, or the team is at its configured capacity.
// Note: This only updates lobby membership. PlayerComponent must be created separately.
func (l *LobbyComponent) AddPlayerToTeam(playerID, teamID string) bool {
	if l.HasPlayer(playerID) || l.PlayerCount >= MaxLobbyPlayers {
		return false
	}
	team := l.GetTeam(teamID)
	if team == nil || team.IsFull() {
		return false
	}
	l.PlayerIDs[l.PlayerCount] = playerID
	l.PlayerCount++
	team.PlayerCount++
	return true
}

// RemovePlayerFromTeam drops a player from the roster and uncharges their team.
// Note: This only updates lobby membership. PlayerComponent must be deleted separately.
func (l *LobbyComponent) RemovePlayerFromTeam(playerID, teamID string) bool {
	if !l.removeFromRoster(playerID) {
		return false
	}
	if team := l.GetTeam(teamID); team != nil && team.PlayerCount > 0 {
		team.PlayerCount--
	}
	return true
}

// MovePlayerToTeam moves a player between teams. The roster is unchanged — only the per-team counts
// and the caller's PlayerComponent.TeamID move. Returns false if the player is not in this lobby, the
// target team does not exist, or the target is full.
func (l *LobbyComponent) MovePlayerToTeam(playerID, oldTeamID, newTeamID string) bool {
	if !l.HasPlayer(playerID) {
		return false
	}
	if oldTeamID == newTeamID {
		return true
	}
	newTeam := l.GetTeam(newTeamID)
	if newTeam == nil || newTeam.IsFull() {
		return false
	}
	if oldTeam := l.GetTeam(oldTeamID); oldTeam != nil && oldTeam.PlayerCount > 0 {
		oldTeam.PlayerCount--
	}
	newTeam.PlayerCount++
	return true
}

// removeFromRoster deletes playerID, keeping live entries contiguous by shifting the tail down.
// Shifted rather than swapped with the last entry because order is load-bearing: leader succession
// takes the first remaining player, and a swap would make that an arbitrary one.
func (l *LobbyComponent) removeFromRoster(playerID string) bool {
	n := l.livePlayers()
	for i, pid := range l.PlayerIDs[:n] {
		if pid != playerID {
			continue
		}
		copy(l.PlayerIDs[i:n-1], l.PlayerIDs[i+1:n])
		l.PlayerIDs[n-1] = ""
		l.PlayerCount = n - 1
		return true
	}
	return false
}
