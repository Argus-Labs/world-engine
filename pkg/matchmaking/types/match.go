package types

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// Match represents a successful matchmaking result.
type Match struct {
	ID                 string
	Teams              []MatchTeam
	MatchProfileName   string
	EnableBackfill     bool
	AutoBackfill       bool
	Config             map[string]any
	MatchmakingAddress *microv1.ServiceAddress
	TargetAddress      *microv1.ServiceAddress
	CreatedAt          time.Time
}

// MatchTeam represents a team in a match.
type MatchTeam struct {
	Name    string
	Tickets []*Ticket
}

// TotalPlayers returns the total number of players in the match.
func (m *Match) TotalPlayers() int {
	count := 0
	for _, team := range m.Teams {
		for _, t := range team.Tickets {
			count += t.PlayerCount()
		}
	}
	return count
}

// ToProto converts the match to its protobuf representation.
func (m *Match) ToProto() *matchmakingv1.Match {
	teams := make([]*matchmakingv1.Team, len(m.Teams))
	for i, team := range m.Teams {
		tickets := make([]*matchmakingv1.TicketReference, len(team.Tickets))
		for j, t := range team.Tickets {
			tickets[j] = t.ToReference()
		}
		teams[i] = &matchmakingv1.Team{
			Name:    team.Name,
			Tickets: tickets,
		}
	}

	return &matchmakingv1.Match{
		Id:                 m.ID,
		Teams:              teams,
		MatchProfileName:   m.MatchProfileName,
		EnableBackfill:     m.EnableBackfill,
		AutoBackfill:       m.AutoBackfill,
		MatchmakingAddress: m.MatchmakingAddress,
		CreatedAt:          timestamppb.New(m.CreatedAt),
	}
}

// BackfillMatch represents a successful backfill matching result.
type BackfillMatch struct {
	ID                string
	BackfillRequestID string
	MatchID           string
	TeamName          string
	Tickets           []*Ticket
	CreatedAt         time.Time
}

// ToProto converts the backfill match to its protobuf representation.
func (bm *BackfillMatch) ToProto() *matchmakingv1.BackfillMatch {
	tickets := make([]*matchmakingv1.TicketReference, len(bm.Tickets))
	for i, t := range bm.Tickets {
		tickets[i] = t.ToReference()
	}

	return &matchmakingv1.BackfillMatch{
		Id:                bm.ID,
		BackfillRequestId: bm.BackfillRequestID,
		MatchId:           bm.MatchID,
		TeamName:          bm.TeamName,
		Tickets:           tickets,
		CreatedAt:         timestamppb.New(bm.CreatedAt),
	}
}

// Assignment represents a ticket assigned to a team.
type Assignment struct {
	Ticket    *Ticket
	TeamIndex int
	TeamName  string
}

// MatchResult is the output of the matchmaking algorithm.
type MatchResult struct {
	Success     bool
	Assignments []Assignment
	TotalWait   time.Duration
}

// ToMatch creates a Match from a MatchResult.
func (mr *MatchResult) ToMatch(
	id string,
	prof *Profile,
	matchmakingAddress *microv1.ServiceAddress,
	createdAt time.Time,
) *Match {
	if !mr.Success {
		return nil
	}

	// Group assignments by team
	teamTickets := make(map[int][]*Ticket)
	for _, a := range mr.Assignments {
		teamTickets[a.TeamIndex] = append(teamTickets[a.TeamIndex], a.Ticket)
	}

	// Build teams
	teams := make([]MatchTeam, prof.GetTeamCount())
	for i := range teams {
		teams[i] = MatchTeam{
			Name:    prof.GetTeamName(i),
			Tickets: teamTickets[i],
		}
	}

	return &Match{
		ID:                 id,
		Teams:              teams,
		MatchProfileName:   prof.Name,
		EnableBackfill:     prof.EnableBackfill,
		AutoBackfill:       prof.AutoBackfill,
		Config:             prof.Config,
		MatchmakingAddress: matchmakingAddress,
		TargetAddress:      prof.TargetAddress,
		CreatedAt:          createdAt,
	}
}
