package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/matchmaking/component"
)

// GetTicketsSystemState is the state for the GetTickets system.
type GetTicketsSystemState struct {
	cardinal.BaseSystemState

	// Commands
	GetTicketsCmds cardinal.WithCommand[GetTicketsCommand]

	// Entities
	Tickets cardinal.Contains[struct {
		Ticket cardinal.Ref[component.TicketComponent]
	}]
}

// GetTicketsSystem handles ticket list queries from other shards.
func GetTicketsSystem(state *GetTicketsSystemState) error {
	for cmd := range state.GetTicketsCmds.Iter() {
		payload := cmd.Payload()

		state.Logger().Debug().
			Str("profile_filter", payload.ProfileName).
			Str("sendback_shard", payload.SendbackWorld.ShardID).
			Msg("Processing GetTicketsCommand")

		// Collect tickets
		var tickets []TicketInfo
		for _, ticketEntity := range state.Tickets.Iter() {
			ticket := ticketEntity.Ticket.Get()

			// Apply profile filter if specified
			if payload.ProfileName != "" && ticket.ProfileName != payload.ProfileName {
				continue
			}

			tickets = append(tickets, TicketInfo{
				TicketID:    ticket.ID,
				PartyID:     ticket.PartyID,
				ProfileName: ticket.ProfileName,
				Players:     toMatchedPlayers(ticket.Players),
				CreatedAt:   ticket.CreatedAt,
				PoolCounts:  ticket.PoolCounts,
			})
		}

		// Send response back to requesting shard
		payload.SendbackWorld.Send(&state.BaseSystemState, TicketsListResponse{
			Tickets: tickets,
			Total:   len(tickets),
		})

		state.Logger().Debug().
			Int("count", len(tickets)).
			Str("sendback_shard", payload.SendbackWorld.ShardID).
			Msg("Sent TicketsListResponse")
	}

	return nil
}
