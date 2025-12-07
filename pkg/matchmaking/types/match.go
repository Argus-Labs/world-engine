package types

import (
	"sort"
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
	Config             map[string]any
	MatchmakingAddress *microv1.ServiceAddress
	TargetAddress      *microv1.ServiceAddress // Game Shard address (for connection_info)
	LobbyAddress       *microv1.ServiceAddress // Lobby Shard address (where to send Match)
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
		MatchmakingAddress: m.MatchmakingAddress,
		TargetAddress:      m.TargetAddress,
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

	// Build teams with sorted tickets for deterministic output
	teams := make([]MatchTeam, prof.GetTeamCount())
	for i := range teams {
		tickets := teamTickets[i]
		// Sort tickets by first player ID for deterministic ordering
		sort.Slice(tickets, func(a, b int) bool {
			return tickets[a].GetFirstPlayerID() < tickets[b].GetFirstPlayerID()
		})
		teams[i] = MatchTeam{
			Name:    prof.GetTeamName(i),
			Tickets: tickets,
		}
	}

	return &Match{
		ID:                 id,
		Teams:              teams,
		MatchProfileName:   prof.Name,
		Config:             prof.Config,
		MatchmakingAddress: matchmakingAddress,
		TargetAddress:      prof.TargetAddress,
		LobbyAddress:       prof.LobbyAddress,
		CreatedAt:          createdAt,
	}
}
