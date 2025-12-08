package component

// PlayerInfo represents a player in a ticket.
type PlayerInfo struct {
	PlayerID   string            `json:"player_id"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// TicketComponent represents a matchmaking ticket.
type TicketComponent struct {
	ID            string            `json:"id"`
	PartyID       string            `json:"party_id"`
	ProfileName   string            `json:"profile_name"`
	Players       []PlayerInfo      `json:"players"`
	AllowBackfill bool              `json:"allow_backfill"`
	CreatedAt     int64             `json:"created_at"`
	ExpiresAt     int64             `json:"expires_at"`
	PoolCounts    map[string]int    `json:"pool_counts,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// Name returns the component name for ECS registration.
func (TicketComponent) Name() string { return "matchmaking_ticket" }

// PlayerCount returns the number of players in the ticket.
func (t *TicketComponent) PlayerCount() int {
	return len(t.Players)
}

// IsExpired checks if the ticket has expired.
func (t *TicketComponent) IsExpired(now int64) bool {
	return now >= t.ExpiresAt
}

// GetPlayerIDs returns all player IDs in the ticket.
func (t *TicketComponent) GetPlayerIDs() []string {
	ids := make([]string, len(t.Players))
	for i, p := range t.Players {
		ids[i] = p.PlayerID
	}
	return ids
}
