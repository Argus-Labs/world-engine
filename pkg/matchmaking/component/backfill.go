package component

// BackfillSlot represents a slot needed for a specific pool/role.
type BackfillSlot struct {
	PoolName string `json:"pool_name"`
	Count    int    `json:"count"`
}

// BackfillComponent represents a backfill request for an ongoing match.
type BackfillComponent struct {
	ID          string         `json:"id"`
	MatchID     string         `json:"match_id"`
	ProfileName string         `json:"profile_name"`
	TeamName    string         `json:"team_name"`
	SlotsNeeded int            `json:"slots_needed"`       // Total slots needed (for simple backfill)
	Slots       []BackfillSlot `json:"slots,omitempty"`    // Role-specific slots (for role-based backfill)
	CreatedAt   int64          `json:"created_at"`
	ExpiresAt   int64          `json:"expires_at"`
}

// Name returns the component name for ECS registration.
func (BackfillComponent) Name() string { return "matchmaking_backfill" }

// IsExpired checks if the backfill request has expired.
func (b *BackfillComponent) IsExpired(now int64) bool {
	return now >= b.ExpiresAt
}

// TotalSlotsNeeded returns the total number of slots needed.
func (b *BackfillComponent) TotalSlotsNeeded() int {
	if len(b.Slots) > 0 {
		total := 0
		for _, slot := range b.Slots {
			total += slot.Count
		}
		return total
	}
	return b.SlotsNeeded
}
