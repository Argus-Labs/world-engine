package matchmaking

import (
	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

// Filter provides functions to match tickets against pool filter criteria.
// This is Phase 1 of the matchmaking algorithm: determining which pools each player matches.

// DerivePoolCounts computes how many players in a ticket match each pool.
// Returns a map from pool name to count of matching players.
func DerivePoolCounts(t *types.Ticket, prof *types.Profile) map[string]int {
	counts := make(map[string]int)

	for _, pool := range prof.Pools {
		matchCount := 0
		for _, player := range t.Players {
			if PlayerPassesPoolFilters(player.SearchFields, pool) {
				matchCount++
			}
		}
		if matchCount > 0 {
			counts[pool.Name] = matchCount
		}
	}

	return counts
}

// PlayerPassesPoolFilters checks if a player's search fields pass all filters for a pool.
// A player passes if ALL filters match (AND logic).
func PlayerPassesPoolFilters(fields types.SearchFields, pool types.Pool) bool {
	// Check string equals filters
	for _, filter := range pool.StringEqualsFilters {
		if !passesStringEqualsFilter(fields, filter) {
			return false
		}
	}

	// Check double range filters
	for _, filter := range pool.DoubleRangeFilters {
		if !passesDoubleRangeFilter(fields, filter) {
			return false
		}
	}

	// Check tag present filters
	for _, filter := range pool.TagPresentFilters {
		if !passesTagPresentFilter(fields, filter) {
			return false
		}
	}

	return true
}

// passesStringEqualsFilter checks if the field equals the expected value.
func passesStringEqualsFilter(fields types.SearchFields, filter types.StringEqualsFilter) bool {
	if fields.StringArgs == nil {
		return false
	}
	value, exists := fields.StringArgs[filter.Field]
	if !exists {
		return false
	}
	return value == filter.Value
}

// passesDoubleRangeFilter checks if the field is within [min, max].
func passesDoubleRangeFilter(fields types.SearchFields, filter types.DoubleRangeFilter) bool {
	if fields.DoubleArgs == nil {
		return false
	}
	value, exists := fields.DoubleArgs[filter.Field]
	if !exists {
		return false
	}
	return value >= filter.Min && value <= filter.Max
}

// passesTagPresentFilter checks if the tag is present.
func passesTagPresentFilter(fields types.SearchFields, filter types.TagPresentFilter) bool {
	for _, tag := range fields.Tags {
		if tag == filter.Tag {
			return true
		}
	}
	return false
}

// FilterCandidates filters tickets to those that can potentially participate in matches.
// Returns tickets that:
// 1. Match at least one pool
// 2. Have party size <= team size
func FilterCandidates(tickets []*types.Ticket, prof *types.Profile) []*types.Ticket {
	maxTeamSize := getMaxTeamSize(prof)
	candidates := make([]*types.Ticket, 0, len(tickets))

	for _, t := range tickets {
		// Reject if party is larger than max team size
		if t.PlayerCount() > maxTeamSize {
			continue
		}

		// Compute pool counts if not already cached
		if t.PoolCounts == nil {
			t.PoolCounts = DerivePoolCounts(t, prof)
		}

		// Reject if no player matches any pool
		if len(t.PoolCounts) == 0 {
			continue
		}

		candidates = append(candidates, t)
	}

	return candidates
}

// FilterBackfillCandidates filters tickets for backfill matching.
// Similar to FilterCandidates but also checks that tickets are backfill-eligible
// and match the specific slots needed.
func FilterBackfillCandidates(
	tickets []*types.Ticket,
	prof *types.Profile,
	slotsNeeded []types.SlotNeeded,
) []*types.Ticket {
	// Build a set of pools we need
	neededPools := make(map[string]bool)
	for _, slot := range slotsNeeded {
		neededPools[slot.PoolName] = true
	}

	// Get max slot count as max party size
	maxSize := 0
	for _, slot := range slotsNeeded {
		if slot.Count > maxSize {
			maxSize = slot.Count
		}
	}

	candidates := make([]*types.Ticket, 0, len(tickets))

	for _, t := range tickets {
		// Only consider backfill-eligible tickets
		if !t.AllowBackfill {
			continue
		}

		// Reject if party is larger than max slot count
		if t.PlayerCount() > maxSize {
			continue
		}

		// Compute pool counts if not cached
		if t.PoolCounts == nil {
			t.PoolCounts = DerivePoolCounts(t, prof)
		}

		// Check if ticket matches at least one needed pool
		matchesNeeded := false
		for poolName := range t.PoolCounts {
			if neededPools[poolName] {
				matchesNeeded = true
				break
			}
		}
		if !matchesNeeded {
			continue
		}

		candidates = append(candidates, t)
	}

	return candidates
}

// getMaxTeamSize returns the maximum team size from a profile.
func getMaxTeamSize(prof *types.Profile) int {
	if prof.IsSymmetric() {
		return prof.TeamSize
	}
	maxSize := 0
	for _, team := range prof.Teams {
		if team.Size > maxSize {
			maxSize = team.Size
		}
	}
	return maxSize
}
