package component

// BackfillComponent represents a backfill request for an ongoing match.
type BackfillComponent struct {
	ID          string `json:"id"`
	MatchID     string `json:"match_id"`
	ProfileName string `json:"profile_name"`
	TeamName    string `json:"team_name"`
	SlotsNeeded int    `json:"slots_needed"`
	CreatedAt   int64  `json:"created_at"`
	ExpiresAt   int64  `json:"expires_at"`
}

// Name returns the component name for ECS registration.
func (BackfillComponent) Name() string { return "matchmaking_backfill" }

// IsExpired checks if the backfill request has expired.
func (b *BackfillComponent) IsExpired(now int64) bool {
	return now >= b.ExpiresAt
}
