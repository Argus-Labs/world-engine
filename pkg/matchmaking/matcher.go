package matchmaking

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/matchmaking/algorithm"
	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

// RunMatchmaking uses the bounded DP algorithm to find a valid match.
// Returns a MatchResult with assignments if successful, or Success=false if no match found.
func RunMatchmaking(candidates []*types.Ticket, prof *types.Profile, now time.Time) *types.MatchResult {
	// Convert tickets to algorithm.Ticket interface slice
	algoTickets := toAlgorithmTickets(candidates)

	// Run the algorithm - profile implements algorithm.Profile interface
	output := algorithm.Run(algorithm.NewInput(algoTickets, prof, now))

	// Convert result back
	return fromAlgorithmOutput(&output)
}

// RunBackfillMatchmaking uses the bounded DP algorithm to fill backfill slots.
func RunBackfillMatchmaking(
	candidates []*types.Ticket,
	slotsNeeded []types.SlotNeeded,
	now time.Time,
) *types.MatchResult {
	// Convert tickets to algorithm.Ticket interface slice
	algoTickets := toAlgorithmTickets(candidates)
	algoSlots := toAlgorithmSlots(slotsNeeded)

	// Run the algorithm
	output := algorithm.Run(algorithm.NewBackfillInput(algoTickets, algoSlots, now))

	// Convert result back
	return fromAlgorithmOutput(&output)
}

// toAlgorithmTickets converts matchmaking tickets to algorithm.Ticket interface slice.
// Since *types.Ticket implements algorithm.Ticket, we just need to convert the slice type.
func toAlgorithmTickets(tickets []*types.Ticket) []algorithm.Ticket {
	result := make([]algorithm.Ticket, len(tickets))
	for i, t := range tickets {
		result[i] = t
	}
	return result
}

// toAlgorithmSlots converts types.SlotNeeded to algorithm.SlotNeeded.
func toAlgorithmSlots(slots []types.SlotNeeded) []algorithm.SlotNeeded {
	result := make([]algorithm.SlotNeeded, len(slots))
	for i, s := range slots {
		result[i] = algorithm.SlotNeeded{
			PoolName: s.PoolName,
			Count:    s.Count,
		}
	}
	return result
}

// fromAlgorithmOutput converts algorithm output back to MatchResult.
func fromAlgorithmOutput(output *algorithm.Output) *types.MatchResult {
	if !output.Success {
		return &types.MatchResult{Success: false}
	}

	// Convert assignments - the Ticket in Assignment is already *types.Ticket
	// since that's what we passed in
	assignments := make([]types.Assignment, len(output.Assignments))
	for i, a := range output.Assignments {
		assignments[i] = types.Assignment{
			Ticket:    a.Ticket.(*types.Ticket),
			TeamIndex: a.TeamIndex,
			TeamName:  a.TeamName,
		}
	}

	return &types.MatchResult{
		Success:     true,
		Assignments: assignments,
		TotalWait:   output.TotalWait,
	}
}
