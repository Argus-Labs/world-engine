package matchmaking

import (
	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
)

// CreateTicketCommand is the command to create a matchmaking ticket.
type CreateTicketCommand struct {
	PartyID          string             `json:"party_id"`
	MatchProfileName string             `json:"match_profile_name"`
	AllowBackfill    bool               `json:"allow_backfill"`
	Players          []types.PlayerInfo `json:"players"`
}

// Name returns the command name for registration.
func (c CreateTicketCommand) Name() string {
	return "create-ticket"
}

// CancelTicketCommand is the command to cancel a matchmaking ticket.
type CancelTicketCommand struct {
	TicketID string `json:"ticket_id"`
	PartyID  string `json:"party_id"`
}

// Name returns the command name for registration.
func (c CancelTicketCommand) Name() string {
	return "cancel-ticket"
}

// CreateBackfillCommand is the command to create a backfill request.
type CreateBackfillCommand struct {
	MatchID          string            `json:"match_id"`
	MatchProfileName string            `json:"match_profile_name"`
	TeamName         string            `json:"team_name"`
	SlotsNeeded      []types.SlotNeeded `json:"slots_needed"`
	LobbyAddress     *LobbyAddress     `json:"lobby_address"`
}

// LobbyAddress represents a service address for the lobby.
type LobbyAddress struct {
	Region       string `json:"region"`
	Organization string `json:"organization"`
	Project      string `json:"project"`
	ServiceID    string `json:"service_id"`
	Realm        string `json:"realm"`
}

// Name returns the command name for registration.
func (c CreateBackfillCommand) Name() string {
	return "create-backfill"
}

// CancelBackfillCommand is the command to cancel a backfill request.
type CancelBackfillCommand struct {
	BackfillRequestID string `json:"backfill_request_id"`
}

// Name returns the command name for registration.
func (c CancelBackfillCommand) Name() string {
	return "cancel-backfill"
}
