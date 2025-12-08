package algorithm

import (
	"slices"
	"sort"
	"time"
)

// BoundedDP implements assignment using bounded Dynamic Programming.
// It explores ticket combinations to find optimal assignments prioritizing older tickets.
//
// The algorithm is "bounded" because:
// 1. State space is limited by team sizes and role counts
// 2. Early termination when a valid assignment is found
// 3. Candidates are sorted by wait time (oldest first)
type BoundedDP struct{}

// Name returns the algorithm name for logging.
func (b *BoundedDP) Name() string {
	return "bounded-dp"
}

// Run executes the bounded DP algorithm.
// Handles both new match assignment (via NewInput) and backfill (via NewBackfillInput).
func (b *BoundedDP) Run(input Input) Output {
	var startTime time.Time
	if input.GetDebug() {
		startTime = time.Now()
	}

	candidates := input.GetCandidates()
	if len(candidates) == 0 {
		return Output{Success: false}
	}

	// Sort candidates by created_at (oldest first) for priority
	// Use first player ID as tiebreaker for deterministic ordering when timestamps are equal
	sortedCandidates := make([]Ticket, len(candidates))
	copy(sortedCandidates, candidates)
	sort.Slice(sortedCandidates, func(i, j int) bool {
		ti, tj := sortedCandidates[i].GetCreatedAt(), sortedCandidates[j].GetCreatedAt()
		if ti.Equal(tj) {
			return sortedCandidates[i].GetFirstPlayerID() < sortedCandidates[j].GetFirstPlayerID()
		}
		return ti.Before(tj)
	})

	// Convert input to dpConfig based on type
	var config *dpConfig
	if input.IsBackfill() {
		config = toBackfillConfig(input.GetSlotsNeeded())
	} else {
		config = toConfig(input.GetProfile())
	}

	result := solve(sortedCandidates, config, input.GetNow(), input.GetDebug())

	output := Output{
		Success:     result.success,
		Assignments: result.assignments,
		TotalWait:   result.totalWait,
	}

	if input.GetDebug() {
		output.Stats = Stats{
			CandidatesConsidered: len(candidates),
			StatesExplored:       result.statesExplored,
			Duration:             time.Since(startTime),
		}
	}

	return output
}

// dpResult is the internal result type for DP algorithms.
type dpResult struct {
	success        bool
	assignments    []Assignment
	totalWait      time.Duration
	statesExplored int
}

// dpConfig is the unified configuration for the DP algorithm.
// It normalizes different input types (match profile, backfill) into a single format.
type dpConfig struct {
	// teams defines each team's requirements
	teams []teamConfig
}

// teamConfig defines a single team's requirements.
type teamConfig struct {
	// name is the team name for assignments
	name string
	// maxSize is the maximum number of players allowed
	maxSize int
	// minSize is the minimum number of players required for completion
	minSize int
	// composition maps pool names to required counts (nil or empty = no role requirements)
	composition map[string]int
}

// toConfig converts a Profile to dpConfig.
func toConfig(profile Profile) *dpConfig {
	teamCount := profile.GetTeamCount()
	teams := make([]teamConfig, teamCount)

	for i := 0; i < teamCount; i++ {
		teams[i] = teamConfig{
			name:        profile.GetTeamName(i),
			maxSize:     profile.GetTeamSize(i),
			minSize:     profile.GetTeamMinSize(i),
			composition: profile.GetTeamCompositionMap(i),
		}
	}

	return &dpConfig{teams: teams}
}

// toBackfillConfig converts backfill slots to dpConfig (single team).
func toBackfillConfig(slotsNeeded []SlotNeeded) *dpConfig {
	composition := make(map[string]int)
	totalNeeded := 0
	for _, slot := range slotsNeeded {
		composition[slot.PoolName] = slot.Count
		totalNeeded += slot.Count
	}

	return &dpConfig{
		teams: []teamConfig{
			{
				name:        "", // Will be set by caller if needed
				maxSize:     totalNeeded,
				minSize:     totalNeeded,
				composition: composition,
			},
		},
	}
}

// dpState represents a state in the DP algorithm.
type dpState struct {
	// teamSizes[teamIndex] = current player count
	teamSizes []int
	// teamCounts[teamIndex][poolName] = current count per pool
	teamCounts []map[string]int
	// assignments made so far
	assignments []Assignment
	// totalWaitMS for priority (higher is better - older tickets)
	totalWaitMS int64
}

// clone creates a deep copy of the state.
func (s *dpState) clone() *dpState {
	newState := &dpState{
		teamSizes:   make([]int, len(s.teamSizes)),
		teamCounts:  make([]map[string]int, len(s.teamCounts)),
		totalWaitMS: s.totalWaitMS,
	}
	copy(newState.teamSizes, s.teamSizes)

	for i, tc := range s.teamCounts {
		newState.teamCounts[i] = make(map[string]int, len(tc))
		for k, v := range tc {
			newState.teamCounts[i][k] = v
		}
	}

	newState.assignments = make([]Assignment, len(s.assignments))
	copy(newState.assignments, s.assignments)

	return newState
}

// stateKey generates a string key for the state (for deduplication).
func (s *dpState) stateKey() string {
	var key string
	for i, tc := range s.teamCounts {
		// Include team size
		key += string(rune('0' + s.teamSizes[i]))

		// Sort pool names for deterministic key
		poolNames := make([]string, 0, len(tc))
		for poolName := range tc {
			poolNames = append(poolNames, poolName)
		}
		slices.Sort(poolNames)

		for _, poolName := range poolNames {
			count := tc[poolName]
			key += poolName + ":" + string(rune('0'+count)) + ";"
		}
		key += "|"
	}
	return key
}

// solve is the core DP algorithm that handles all assignment scenarios.
// When debug is true, statesExplored is tracked for diagnostics.
func solve(candidates []Ticket, config *dpConfig, now time.Time, debug bool) *dpResult {
	teamCount := len(config.teams)

	// Initialize DP with empty state
	initialState := &dpState{
		teamSizes:   make([]int, teamCount),
		teamCounts:  make([]map[string]int, teamCount),
		assignments: []Assignment{},
		totalWaitMS: 0,
	}
	for i := 0; i < teamCount; i++ {
		initialState.teamCounts[i] = make(map[string]int)
	}

	// DP table: stateKey -> best state for that configuration
	dp := map[string]*dpState{
		initialState.stateKey(): initialState,
	}

	var bestComplete *dpState
	var statesExplored int

	// Process each ticket
	for _, ticket := range candidates {
		playerCount := ticket.PlayerCount()
		poolCounts := ticket.GetPoolCounts()
		waitMS := WaitTime(ticket, now).Milliseconds()

		// Get all current states (copy keys to avoid modification during iteration)
		// Sort keys for deterministic ordering
		stateKeys := make([]string, 0, len(dp))
		for k := range dp {
			stateKeys = append(stateKeys, k)
		}
		slices.Sort(stateKeys)

		for _, key := range stateKeys {
			state := dp[key]
			if debug {
				statesExplored++
			}

			// Try assigning this ticket to each team
			for teamIdx := 0; teamIdx < teamCount; teamIdx++ {
				team := &config.teams[teamIdx]

				// Check if ticket fits in team (size constraint)
				newSize := state.teamSizes[teamIdx] + playerCount
				if newSize > team.maxSize {
					continue
				}

				// Check role constraints (if any)
				if !canAssign(state.teamCounts[teamIdx], poolCounts, team.composition) {
					continue
				}

				// Create new state
				newState := state.clone()
				newState.teamSizes[teamIdx] = newSize
				for poolName, count := range poolCounts {
					newState.teamCounts[teamIdx][poolName] += count
				}
				newState.totalWaitMS += waitMS
				newState.assignments = append(newState.assignments, Assignment{
					Ticket:    ticket,
					TeamIndex: teamIdx,
					TeamName:  team.name,
				})

				newKey := newState.stateKey()

				// Check if this is a complete assignment
				if isComplete(newState, config) {
					if bestComplete == nil || isBetterState(newState, bestComplete) {
						bestComplete = newState
					}
				}

				// Update DP table if better
				if existing, ok := dp[newKey]; !ok || isBetterState(newState, existing) {
					dp[newKey] = newState
				}
			}
		}

		// Early exit if we found a complete assignment
		if bestComplete != nil {
			break
		}
	}

	if bestComplete == nil {
		return &dpResult{success: false, statesExplored: statesExplored}
	}

	return &dpResult{
		success:        true,
		assignments:    bestComplete.assignments,
		totalWait:      time.Duration(bestComplete.totalWaitMS) * time.Millisecond,
		statesExplored: statesExplored,
	}
}

// solveGreedy uses simple greedy assignment when all tickets have equal timestamps.
// Each ticket is assigned to the first valid team, ensuring deterministic results.
func solveGreedy(candidates []Ticket, config *dpConfig, now time.Time, debug bool) *dpResult {
	teamCount := len(config.teams)

	// Track current state
	teamSizes := make([]int, teamCount)
	teamCounts := make([]map[string]int, teamCount)
	for i := 0; i < teamCount; i++ {
		teamCounts[i] = make(map[string]int)
	}

	var assignments []Assignment
	var totalWaitMS int64
	var statesExplored int

	// Process each ticket in sorted order
	for _, ticket := range candidates {
		playerCount := ticket.PlayerCount()
		poolCounts := ticket.GetPoolCounts()
		waitMS := WaitTime(ticket, now).Milliseconds()

		if debug {
			statesExplored++
		}

		// Try assigning to each team (first valid wins)
		for teamIdx := 0; teamIdx < teamCount; teamIdx++ {
			team := &config.teams[teamIdx]

			// Check size constraint
			newSize := teamSizes[teamIdx] + playerCount
			if newSize > team.maxSize {
				continue
			}

			// Check role constraints
			if !canAssign(teamCounts[teamIdx], poolCounts, team.composition) {
				continue
			}

			// Assign to this team
			teamSizes[teamIdx] = newSize
			for poolName, count := range poolCounts {
				teamCounts[teamIdx][poolName] += count
			}
			totalWaitMS += waitMS
			assignments = append(assignments, Assignment{
				Ticket:    ticket,
				TeamIndex: teamIdx,
				TeamName:  team.name,
			})
			break // First valid team wins
		}
	}

	// Check if complete
	complete := true
	for i, team := range config.teams {
		if teamSizes[i] < team.minSize {
			complete = false
			break
		}
		if len(team.composition) > 0 {
			for poolName, required := range team.composition {
				if teamCounts[i][poolName] < required {
					complete = false
					break
				}
			}
		}
		if !complete {
			break
		}
	}

	if !complete {
		return &dpResult{success: false, statesExplored: statesExplored}
	}

	return &dpResult{
		success:        true,
		assignments:    assignments,
		totalWait:      time.Duration(totalWaitMS) * time.Millisecond,
		statesExplored: statesExplored,
	}
}

// canAssign checks if ticket's pool counts can be added to a team.
// Returns true if the assignment is valid (doesn't exceed any role limits).
func canAssign(currentCounts, ticketCounts, targetCounts map[string]int) bool {
	// If no composition requirements, any assignment is valid
	if len(targetCounts) == 0 {
		return true
	}

	for poolName, ticketCount := range ticketCounts {
		target, hasTarget := targetCounts[poolName]
		if !hasTarget {
			// Pool not required for this team - that's okay
			continue
		}
		current := currentCounts[poolName]
		if current+ticketCount > target {
			return false
		}
	}
	return true
}

// isComplete checks if all teams meet their requirements.
func isComplete(state *dpState, config *dpConfig) bool {
	for i, team := range config.teams {
		// Check minimum size
		if state.teamSizes[i] < team.minSize {
			return false
		}

		// Check role requirements (if any)
		if len(team.composition) > 0 {
			for poolName, required := range team.composition {
				if state.teamCounts[i][poolName] < required {
					return false
				}
			}
		}
	}
	return true
}

// isBetterState compares two states and returns true if newState is better than existing.
// Primary: higher totalWaitMS is better (prioritizes older tickets).
// Tiebreaker: when equal, prefer state where team_1 has alphabetically earlier players
// (ensures deterministic results when tickets have equal timestamps).
func isBetterState(newState, existing *dpState) bool {
	if newState.totalWaitMS != existing.totalWaitMS {
		return newState.totalWaitMS > existing.totalWaitMS
	}
	// Tiebreaker: prefer state where earlier teams have alphabetically earlier players
	// Compare by (teamIndex, playerID) pairs
	newPairs := assignmentPairs(newState.assignments)
	existingPairs := assignmentPairs(existing.assignments)

	minLen := len(newPairs)
	if len(existingPairs) < minLen {
		minLen = len(existingPairs)
	}
	for i := 0; i < minLen; i++ {
		// Compare team index first (prefer lower team index)
		if newPairs[i].teamIdx != existingPairs[i].teamIdx {
			return newPairs[i].teamIdx < existingPairs[i].teamIdx
		}
		// Then compare player ID
		if newPairs[i].playerID != existingPairs[i].playerID {
			return newPairs[i].playerID < existingPairs[i].playerID
		}
	}
	return len(newState.assignments) < len(existing.assignments)
}

type assignmentPair struct {
	teamIdx  int
	playerID string
}

func assignmentPairs(assignments []Assignment) []assignmentPair {
	pairs := make([]assignmentPair, len(assignments))
	for i, a := range assignments {
		pairs[i] = assignmentPair{
			teamIdx:  a.TeamIndex,
			playerID: a.Ticket.GetFirstPlayerID(),
		}
	}
	// Sort by teamIdx, then playerID for consistent comparison
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].teamIdx != pairs[j].teamIdx {
			return pairs[i].teamIdx < pairs[j].teamIdx
		}
		return pairs[i].playerID < pairs[j].playerID
	})
	return pairs
}
