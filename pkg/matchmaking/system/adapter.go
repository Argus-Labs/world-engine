package system

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/matchmaking/algorithm"
	"github.com/argus-labs/world-engine/pkg/matchmaking/component"
)

// ticketAdapter adapts TicketComponent to the algorithm.Ticket interface.
type ticketAdapter struct {
	ticket   component.TicketComponent
	entityID uint32
}

// Compile-time check that ticketAdapter implements algorithm.Ticket
var _ algorithm.Ticket = (*ticketAdapter)(nil)

func (t *ticketAdapter) GetID() string {
	return t.ticket.ID
}

func (t *ticketAdapter) GetCreatedAt() time.Time {
	return time.Unix(t.ticket.CreatedAt, 0)
}

func (t *ticketAdapter) GetPoolCounts() map[string]int {
	if t.ticket.PoolCounts != nil {
		return t.ticket.PoolCounts
	}
	// Default: count all players as a single pool
	return map[string]int{"default": len(t.ticket.Players)}
}

func (t *ticketAdapter) PlayerCount() int {
	return len(t.ticket.Players)
}

func (t *ticketAdapter) GetFirstPlayerID() string {
	if len(t.ticket.Players) > 0 {
		return t.ticket.Players[0].PlayerID
	}
	return ""
}

// profileAdapter adapts ProfileComponent to the algorithm.Profile interface.
type profileAdapter struct {
	profile component.ProfileComponent
}

// Compile-time check that profileAdapter implements algorithm.Profile
var _ algorithm.Profile = (*profileAdapter)(nil)

func (p *profileAdapter) GetTeamCount() int {
	return len(p.profile.Teams)
}

func (p *profileAdapter) GetTeamSize(teamIndex int) int {
	if teamIndex < 0 || teamIndex >= len(p.profile.Teams) {
		return 0
	}
	return p.profile.Teams[teamIndex].MaxPlayers
}

func (p *profileAdapter) GetTeamMinSize(teamIndex int) int {
	if teamIndex < 0 || teamIndex >= len(p.profile.Teams) {
		return 0
	}
	return p.profile.Teams[teamIndex].MinPlayers
}

func (p *profileAdapter) GetTeamName(teamIndex int) string {
	if teamIndex < 0 || teamIndex >= len(p.profile.Teams) {
		return ""
	}
	return p.profile.Teams[teamIndex].Name
}

func (p *profileAdapter) GetTeamCompositionMap(teamIndex int) map[string]int {
	if teamIndex < 0 || teamIndex >= len(p.profile.Teams) {
		return nil
	}
	team := p.profile.Teams[teamIndex]

	// Build composition by counting pool occurrences in team.Pools
	// e.g., ["tank", "dps", "dps", "dps", "support"] -> {tank: 1, dps: 3, support: 1}
	if len(team.Pools) == 0 {
		return nil
	}

	composition := make(map[string]int)
	for _, poolName := range team.Pools {
		composition[poolName]++
	}
	return composition
}

func (p *profileAdapter) HasRoles() bool {
	// Has roles if any team references pools
	for _, team := range p.profile.Teams {
		if len(team.Pools) > 0 {
			return true
		}
	}
	return false
}
