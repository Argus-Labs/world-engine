package algorithm

import "time"

// Ticket defines the interface for a ticket (candidate for assignment).
type Ticket interface {
	// GetID returns the unique ticket identifier.
	GetID() string

	// GetCreatedAt returns when the ticket was created (for wait time priority).
	GetCreatedAt() time.Time

	// GetPoolCounts returns pool names mapped to the number of players in each pool.
	// For role-based assignment, this indicates which roles the party can fill.
	GetPoolCounts() map[string]int

	// PlayerCount returns the total number of players in this ticket.
	PlayerCount() int

	// GetFirstPlayerID returns the first player ID in the ticket.
	// Used as a tiebreaker for deterministic sorting when timestamps are equal.
	GetFirstPlayerID() string
}

// WaitTime returns the duration a ticket has been waiting.
func WaitTime(t Ticket, now time.Time) time.Duration {
	return now.Sub(t.GetCreatedAt())
}

// Profile defines the interface for assignment requirements.
type Profile interface {
	// GetTeamCount returns the number of teams.
	GetTeamCount() int

	// GetTeamSize returns the maximum team size for a given team index.
	GetTeamSize(teamIndex int) int

	// GetTeamMinSize returns the minimum team size for a given team index.
	GetTeamMinSize(teamIndex int) int

	// GetTeamName returns the name for a given team index.
	GetTeamName(teamIndex int) string

	// GetTeamCompositionMap returns the composition requirements for a team as a map.
	// Returns map[poolName]count for the given team index.
	GetTeamCompositionMap(teamIndex int) map[string]int

	// HasRoles returns true if the profile has role requirements.
	HasRoles() bool
}
