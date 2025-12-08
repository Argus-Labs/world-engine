package component

// PartyComponent represents a group of 1 or more players.
// Has a leader, can be open/closed. Exists while members are connected.
// From ADR-030: Party is the atomic unit for matchmaking - parties are never split across teams.
type PartyComponent struct {
	ID        string   `json:"id"`
	LeaderID  string   `json:"leader_id"`
	Members   []string `json:"members"`
	IsOpen    bool     `json:"is_open"`
	MaxSize   int      `json:"max_size"`
	LobbyID   string   `json:"lobby_id,omitempty"`
	IsReady   bool     `json:"is_ready,omitempty"`
	CreatedAt int64    `json:"created_at"`
}

// Name returns the component name for ECS registration.
func (PartyComponent) Name() string { return "lobby_party" }

// Size returns the number of members in the party.
func (p *PartyComponent) Size() int {
	return len(p.Members)
}

// IsFull returns true if the party has reached max capacity.
func (p *PartyComponent) IsFull() bool {
	return len(p.Members) >= p.MaxSize
}

// HasMember returns true if the given player is in the party.
func (p *PartyComponent) HasMember(playerID string) bool {
	for _, m := range p.Members {
		if m == playerID {
			return true
		}
	}
	return false
}

// IsLeader returns true if the given player is the party leader.
func (p *PartyComponent) IsLeader(playerID string) bool {
	return p.LeaderID == playerID
}

// InLobby returns true if the party is currently in a lobby.
func (p *PartyComponent) InLobby() bool {
	return p.LobbyID != ""
}

// AddMember adds a player to the party.
func (p *PartyComponent) AddMember(playerID string) {
	if !p.HasMember(playerID) {
		p.Members = append(p.Members, playerID)
	}
}

// RemoveMember removes a player from the party.
func (p *PartyComponent) RemoveMember(playerID string) {
	for i, m := range p.Members {
		if m == playerID {
			p.Members = append(p.Members[:i], p.Members[i+1:]...)
			return
		}
	}
}
