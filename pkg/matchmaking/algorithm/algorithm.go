// Package algorithm provides algorithms for assigning tickets to teams.
//
// Current approach: Custom or simpler algorithms can be added directly in Go within this package.
// Far future: WASM support for full customization without modifying this package.
package algorithm

import (
	"time"
)

// Input represents the input to an assignment algorithm.
// Use NewInput or NewBackfillInput constructors to create instances.
type Input struct {
	candidates  []Ticket
	profile     Profile
	slotsNeeded []SlotNeeded
	now         time.Time
	debug       bool
}

// NewInput creates input for new match assignment.
func NewInput(candidates []Ticket, profile Profile, now time.Time) Input {
	return Input{
		candidates: candidates,
		profile:    profile,
		now:        now,
	}
}

// NewBackfillInput creates input for backfill assignment.
func NewBackfillInput(candidates []Ticket, slotsNeeded []SlotNeeded, now time.Time) Input {
	return Input{
		candidates:  candidates,
		slotsNeeded: slotsNeeded,
		now:         now,
	}
}

// WithDebug enables debug statistics collection (StatesExplored, Duration).
func (i Input) WithDebug() Input {
	i.debug = true
	return i
}

// Getters for internal use by algorithms
func (i Input) GetCandidates() []Ticket      { return i.candidates }
func (i Input) GetProfile() Profile          { return i.profile }
func (i Input) GetSlotsNeeded() []SlotNeeded { return i.slotsNeeded }
func (i Input) GetNow() time.Time            { return i.now }
func (i Input) GetDebug() bool               { return i.debug }
func (i Input) IsBackfill() bool             { return len(i.slotsNeeded) > 0 }

// Output represents the result of an assignment algorithm.
type Output struct {
	// Success indicates whether a valid assignment was found.
	Success bool

	// Assignments maps tickets to their assigned teams.
	// Only populated if Success is true.
	Assignments []Assignment

	// TotalWait is the combined wait time of all assigned tickets.
	TotalWait time.Duration

	// Stats contains optional statistics for debugging/monitoring.
	Stats Stats
}

// Stats contains statistics about the assignment process.
type Stats struct {
	// CandidatesConsidered is the number of tickets evaluated.
	CandidatesConsidered int

	// StatesExplored is the number of DP states explored (if applicable).
	StatesExplored int

	// Duration is how long the assignment took.
	Duration time.Duration
}

// Assignment represents a ticket assigned to a team.
type Assignment struct {
	// Ticket is the assigned ticket.
	Ticket Ticket

	// TeamIndex is the zero-based team index.
	TeamIndex int

	// TeamName is the human-readable team name.
	TeamName string
}

// SlotNeeded specifies a slot to fill in a backfill request.
type SlotNeeded struct {
	PoolName string
	Count    int
}

// Run executes the default algorithm (BoundedDP) on the given input.
// For new matches, use NewInput. For backfill, use NewBackfillInput.
func Run(input Input) Output {
	return defaultAlgorithm.Run(input)
}

// defaultAlgorithm is the shared algorithm instance.
var defaultAlgorithm = &BoundedDP{}
