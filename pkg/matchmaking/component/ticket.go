package component

// SearchFields contains properties for filter matching.
type SearchFields struct {
	StringArgs map[string]string  `json:"string_args,omitempty"`
	DoubleArgs map[string]float64 `json:"double_args,omitempty"`
	Tags       []string           `json:"tags,omitempty"`
}

// PlayerInfo represents a player in a ticket.
type PlayerInfo struct {
	PlayerID     string       `json:"player_id"`
	SearchFields SearchFields `json:"search_fields"`
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

// MatchesPool checks if a player's search fields match a pool's filters.
func (p *PlayerInfo) MatchesPool(pool PoolConfig) bool {
	// Check all string equals filters must match
	for _, f := range pool.StringEqualsFilters {
		playerValue, exists := p.SearchFields.StringArgs[f.Field]
		if !exists || playerValue != f.Value {
			return false
		}
	}

	// Check all double range filters must match
	for _, f := range pool.DoubleRangeFilters {
		playerValue, exists := p.SearchFields.DoubleArgs[f.Field]
		if !exists || playerValue < f.Min || playerValue > f.Max {
			return false
		}
	}

	// Check all tag present filters must match
	for _, f := range pool.TagPresentFilters {
		found := false
		for _, tag := range p.SearchFields.Tags {
			if tag == f.Tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// ComputePoolCounts determines which pools each player in the ticket matches.
func (t *TicketComponent) ComputePoolCounts(pools []PoolConfig) map[string]int {
	counts := make(map[string]int)
	for _, player := range t.Players {
		for _, pool := range pools {
			if player.MatchesPool(pool) {
				counts[pool.Name]++
				break // Each player matches at most one pool
			}
		}
	}
	// If no specific pool matched but we have players, use "default"
	if len(counts) == 0 && len(t.Players) > 0 {
		counts["default"] = len(t.Players)
	}
	return counts
}
