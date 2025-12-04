package types

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// BackfillRequest represents a request from Lobby Shard to fill vacant slots.
type BackfillRequest struct {
	ID               string
	MatchID          string
	MatchProfileName string
	TeamName         string
	SlotsNeeded      []SlotNeeded
	LobbyAddress     *microv1.ServiceAddress
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

// SlotNeeded specifies a slot to fill in a backfill request.
type SlotNeeded struct {
	PoolName string
	Count    int
}

// IsExpired returns true if the backfill request has expired.
func (r *BackfillRequest) IsExpired(now time.Time) bool {
	return now.After(r.ExpiresAt)
}

// TotalSlotsNeeded returns the total number of players needed.
func (r *BackfillRequest) TotalSlotsNeeded() int {
	total := 0
	for _, slot := range r.SlotsNeeded {
		total += slot.Count
	}
	return total
}

// ToProto converts the backfill request to its protobuf representation.
func (r *BackfillRequest) ToProto() *matchmakingv1.BackfillRequest {
	slots := make([]*matchmakingv1.SlotNeeded, len(r.SlotsNeeded))
	for i, s := range r.SlotsNeeded {
		slots[i] = &matchmakingv1.SlotNeeded{
			PoolName: s.PoolName,
			Count:    int32(s.Count),
		}
	}

	return &matchmakingv1.BackfillRequest{
		Id:               r.ID,
		MatchId:          r.MatchID,
		MatchProfileName: r.MatchProfileName,
		TeamName:         r.TeamName,
		SlotsNeeded:      slots,
		LobbyAddress:     r.LobbyAddress,
		CreatedAt:        timestamppb.New(r.CreatedAt),
		ExpiresAt:        timestamppb.New(r.ExpiresAt),
	}
}

// BackfillRequestFromProto creates a BackfillRequest from its protobuf representation.
func BackfillRequestFromProto(proto *matchmakingv1.BackfillRequest) *BackfillRequest {
	slots := make([]SlotNeeded, len(proto.SlotsNeeded))
	for i, s := range proto.SlotsNeeded {
		slots[i] = SlotNeeded{
			PoolName: s.PoolName,
			Count:    int(s.Count),
		}
	}

	return &BackfillRequest{
		ID:               proto.Id,
		MatchID:          proto.MatchId,
		MatchProfileName: proto.MatchProfileName,
		TeamName:         proto.TeamName,
		SlotsNeeded:      slots,
		LobbyAddress:     proto.LobbyAddress,
		CreatedAt:        proto.CreatedAt.AsTime(),
		ExpiresAt:        proto.ExpiresAt.AsTime(),
	}
}
