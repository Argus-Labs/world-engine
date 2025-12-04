// Package types provides data types for matchmaking.
package types

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
)

// Ticket represents a matchmaking request for a party.
type Ticket struct {
	ID               string
	PartyID          string
	MatchProfileName string
	AllowBackfill    bool
	Players          []PlayerInfo
	CreatedAt        time.Time
	ExpiresAt        time.Time

	// Cached pool counts computed during creation
	PoolCounts map[string]int
}

// PlayerInfo represents a player in a ticket.
type PlayerInfo struct {
	PlayerID     string       `json:"player_id"`
	SearchFields SearchFields `json:"search_fields"`
}

// SearchFields contains properties for filter matching.
type SearchFields struct {
	StringArgs map[string]string  `json:"string_args"`
	DoubleArgs map[string]float64 `json:"double_args"`
	Tags       []string           `json:"tags"`
}

// PlayerCount returns the total number of players in the ticket.
func (t *Ticket) PlayerCount() int {
	return len(t.Players)
}

// GetID returns the ticket ID (implements algorithm.Ticket interface).
func (t *Ticket) GetID() string {
	return t.ID
}

// GetCreatedAt returns when the ticket was created (implements algorithm.Ticket interface).
func (t *Ticket) GetCreatedAt() time.Time {
	return t.CreatedAt
}

// GetPoolCounts returns the pool counts (implements algorithm.Ticket interface).
func (t *Ticket) GetPoolCounts() map[string]int {
	return t.PoolCounts
}

// PlayerIDs returns all player IDs in the ticket.
func (t *Ticket) PlayerIDs() []string {
	ids := make([]string, len(t.Players))
	for i, p := range t.Players {
		ids[i] = p.PlayerID
	}
	return ids
}

// IsExpired returns true if the ticket has expired.
func (t *Ticket) IsExpired(now time.Time) bool {
	return now.After(t.ExpiresAt)
}

// WaitTime returns how long the ticket has been waiting.
func (t *Ticket) WaitTime(now time.Time) time.Duration {
	return now.Sub(t.CreatedAt)
}

// TicketToProto converts the ticket to its protobuf representation.
func (t *Ticket) ToProto() *matchmakingv1.Ticket {
	players := make([]*matchmakingv1.PlayerInfo, len(t.Players))
	for i, p := range t.Players {
		players[i] = &matchmakingv1.PlayerInfo{
			PlayerId: p.PlayerID,
			SearchFields: &matchmakingv1.SearchFields{
				StringArgs: p.SearchFields.StringArgs,
				DoubleArgs: p.SearchFields.DoubleArgs,
				Tags:       p.SearchFields.Tags,
			},
		}
	}

	return &matchmakingv1.Ticket{
		Id:               t.ID,
		PartyId:          t.PartyID,
		MatchProfileName: t.MatchProfileName,
		AllowBackfill:    t.AllowBackfill,
		Players:          players,
		CreatedAt:        timestamppb.New(t.CreatedAt),
		ExpiresAt:        timestamppb.New(t.ExpiresAt),
	}
}

// TicketFromProto creates a Ticket from its protobuf representation.
func TicketFromProto(proto *matchmakingv1.Ticket) *Ticket {
	players := make([]PlayerInfo, len(proto.Players))
	for i, p := range proto.Players {
		var sf SearchFields
		if p.SearchFields != nil {
			sf = SearchFields{
				StringArgs: p.SearchFields.StringArgs,
				DoubleArgs: p.SearchFields.DoubleArgs,
				Tags:       p.SearchFields.Tags,
			}
		}
		players[i] = PlayerInfo{
			PlayerID:     p.PlayerId,
			SearchFields: sf,
		}
	}

	return &Ticket{
		ID:               proto.Id,
		PartyID:          proto.PartyId,
		MatchProfileName: proto.MatchProfileName,
		AllowBackfill:    proto.AllowBackfill,
		Players:          players,
		CreatedAt:        proto.CreatedAt.AsTime(),
		ExpiresAt:        proto.ExpiresAt.AsTime(),
	}
}

// ToReference creates a TicketReference from the ticket.
func (t *Ticket) ToReference() *matchmakingv1.TicketReference {
	return &matchmakingv1.TicketReference{
		Id:        t.ID,
		PartyId:   t.PartyID,
		PlayerIds: t.PlayerIDs(),
	}
}
