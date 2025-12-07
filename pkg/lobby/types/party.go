package types

import (
	"time"
)

// Party represents a group of 1 or more players.
// Has a leader, can be open/closed. Exists while members are connected.
// From ADR-030: Party is the atomic unit for matchmaking - parties are never split across teams.
type Party struct {
	ID        string    `json:"id"`
	LeaderID  string    `json:"leader_id"`
	Members   []string  `json:"members"`
	IsOpen    bool      `json:"is_open"`
	MaxSize   int       `json:"max_size"`
	LobbyID   string    `json:"lobby_id,omitempty"`
	IsReady   bool      `json:"is_ready,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Size returns the number of members in the party.
func (p *Party) Size() int {
	return len(p.Members)
}

// IsFull returns true if the party has reached max capacity.
func (p *Party) IsFull() bool {
	return len(p.Members) >= p.MaxSize
}

// HasMember returns true if the given player is in the party.
func (p *Party) HasMember(playerID string) bool {
	for _, m := range p.Members {
		if m == playerID {
			return true
		}
	}
	return false
}

// IsLeader returns true if the given player is the party leader.
func (p *Party) IsLeader(playerID string) bool {
	return p.LeaderID == playerID
}

// InLobby returns true if the party is currently in a lobby.
func (p *Party) InLobby() bool {
	return p.LobbyID != ""
}
