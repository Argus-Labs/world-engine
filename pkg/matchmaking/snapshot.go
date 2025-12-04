package matchmaking

import (
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
)

// serialize converts the matchmaking state to bytes for snapshot storage.
func (m *matchmaking) serialize() ([]byte, error) {
	snapshot := &matchmakingv1.MatchmakingSnapshot{
		TicketCounter:   m.tickets.GetCounter(),
		MatchCounter:    m.matchCounter,
		BackfillCounter: m.backfills.GetCounter(),
	}

	// Serialize all tickets
	for _, ticket := range m.tickets.All() {
		snapshot.Tickets = append(snapshot.Tickets, ticket.ToProto())
	}

	// Serialize all backfill requests
	for _, req := range m.backfills.All() {
		snapshot.BackfillRequests = append(snapshot.BackfillRequests, req.ToProto())
	}

	data, err := proto.Marshal(snapshot)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal snapshot")
	}

	return data, nil
}

// deserialize restores the matchmaking state from bytes.
func (m *matchmaking) deserialize(data []byte) error {
	var snapshot matchmakingv1.MatchmakingSnapshot
	if err := proto.Unmarshal(data, &snapshot); err != nil {
		return eris.Wrap(err, "failed to unmarshal snapshot")
	}

	// Clear existing state
	m.tickets.Clear()
	m.backfills.Clear()

	// Restore counters
	m.tickets.SetCounter(snapshot.TicketCounter)
	m.matchCounter = snapshot.MatchCounter
	m.backfills.SetCounter(snapshot.BackfillCounter)

	// Restore tickets
	for _, protoTicket := range snapshot.Tickets {
		t := types.TicketFromProto(protoTicket)

		// Recompute pool counts if profile exists
		if prof, ok := m.profiles.Get(t.MatchProfileName); ok {
			t.PoolCounts = DerivePoolCounts(t, prof)
		}

		m.tickets.Restore(t)
	}

	// Restore backfill requests
	for _, protoReq := range snapshot.BackfillRequests {
		req := types.BackfillRequestFromProto(protoReq)
		m.backfills.Restore(req)
	}

	return nil
}
