package system

import (
	"time"

	"github.com/google/uuid"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/matchmaking/algorithm"
	"github.com/argus-labs/world-engine/pkg/matchmaking/component"
)

// -----------------------------------------------------------------------------
// Commands
// -----------------------------------------------------------------------------

// CreateTicketCommand creates a matchmaking ticket.
type CreateTicketCommand struct {
	cardinal.BaseCommand
	PartyID       string                 `json:"party_id"`
	ProfileName   string                 `json:"profile_name"`
	Players       []component.PlayerInfo `json:"players"`
	AllowBackfill bool                   `json:"allow_backfill"`
	TTLSeconds    int64                  `json:"ttl_seconds,omitempty"`
	Attributes    map[string]string      `json:"attributes,omitempty"`
}

// Name returns the command name.
func (CreateTicketCommand) Name() string { return "matchmaking_create_ticket" }

// CancelTicketCommand cancels a matchmaking ticket.
type CancelTicketCommand struct {
	cardinal.BaseCommand
	TicketID string `json:"ticket_id"`
}

// Name returns the command name.
func (CancelTicketCommand) Name() string { return "matchmaking_cancel_ticket" }

// CreateBackfillCommand creates a backfill request.
type CreateBackfillCommand struct {
	cardinal.BaseCommand
	MatchID     string `json:"match_id"`
	ProfileName string `json:"profile_name"`
	TeamName    string `json:"team_name"`
	SlotsNeeded int    `json:"slots_needed"`
	TTLSeconds  int64  `json:"ttl_seconds,omitempty"`
}

// Name returns the command name.
func (CreateBackfillCommand) Name() string { return "matchmaking_create_backfill" }

// CancelBackfillCommand cancels a backfill request.
type CancelBackfillCommand struct {
	cardinal.BaseCommand
	BackfillID string `json:"backfill_id"`
}

// Name returns the command name.
func (CancelBackfillCommand) Name() string { return "matchmaking_cancel_backfill" }

// -----------------------------------------------------------------------------
// Events
// -----------------------------------------------------------------------------

// TicketCreatedEvent is emitted when a ticket is created.
type TicketCreatedEvent struct {
	cardinal.BaseEvent
	TicketID string `json:"ticket_id"`
	PartyID  string `json:"party_id"`
}

// Name returns the event name.
func (TicketCreatedEvent) Name() string { return "matchmaking_ticket_created" }

// TicketCancelledEvent is emitted when a ticket is cancelled.
type TicketCancelledEvent struct {
	cardinal.BaseEvent
	TicketID string `json:"ticket_id"`
}

// Name returns the event name.
func (TicketCancelledEvent) Name() string { return "matchmaking_ticket_cancelled" }

// TicketErrorEvent is emitted when ticket creation fails.
type TicketErrorEvent struct {
	cardinal.BaseEvent
	PartyID string `json:"party_id"`
	Error   string `json:"error"`
}

// Name returns the event name.
func (TicketErrorEvent) Name() string { return "matchmaking_ticket_error" }

// MatchTeam represents a team in a match.
type MatchTeam struct {
	TeamName  string   `json:"team_name"`
	TicketIDs []string `json:"ticket_ids"`
}

// MatchFoundEvent is emitted when a match is found.
type MatchFoundEvent struct {
	cardinal.BaseEvent
	MatchID     string      `json:"match_id"`
	ProfileName string      `json:"profile_name"`
	Teams       []MatchTeam `json:"teams"`
}

// Name returns the event name.
func (MatchFoundEvent) Name() string { return "matchmaking_match_found" }

// BackfillMatchEvent is emitted when backfill tickets are matched.
type BackfillMatchEvent struct {
	cardinal.BaseEvent
	BackfillID string   `json:"backfill_id"`
	MatchID    string   `json:"match_id"`
	TeamName   string   `json:"team_name"`
	TicketIDs  []string `json:"ticket_ids"`
}

// Name returns the event name.
func (BackfillMatchEvent) Name() string { return "matchmaking_backfill_match" }

// -----------------------------------------------------------------------------
// System Events (for same-shard communication)
// -----------------------------------------------------------------------------

// LobbyTeamInfo represents a team for lobby creation.
type LobbyTeamInfo struct {
	TeamName string   `json:"team_name"`
	PartyIDs []string `json:"party_ids"` // PartyIDs are the same as TicketIDs for matchmaking
}

// CreateLobbyFromMatchEvent is a system event sent to lobby system (same shard).
// This is used when matchmaking and lobby are in the same shard.
type CreateLobbyFromMatchEvent struct {
	MatchID     string          `json:"match_id"`
	ProfileName string          `json:"profile_name"`
	Teams       []LobbyTeamInfo `json:"teams"`
}

// Name returns the system event name.
func (CreateLobbyFromMatchEvent) Name() string { return "matchmaking_create_lobby_from_match" }

// CreateLobbyFromMatchCommand is sent cross-shard to lobby shard.
// This is used when matchmaking and lobby are in different shards.
type CreateLobbyFromMatchCommand struct {
	cardinal.BaseCommand
	MatchID     string          `json:"match_id"`
	ProfileName string          `json:"profile_name"`
	Teams       []LobbyTeamInfo `json:"teams"`
}

// Name returns the command name.
func (CreateLobbyFromMatchCommand) Name() string { return "matchmaking_create_lobby_from_match" }

// -----------------------------------------------------------------------------
// System State
// -----------------------------------------------------------------------------

// MatchmakingSystemState is the state for the matchmaking system.
type MatchmakingSystemState struct {
	cardinal.BaseSystemState

	// Commands
	CreateTicketCmds   cardinal.WithCommand[CreateTicketCommand]
	CancelTicketCmds   cardinal.WithCommand[CancelTicketCommand]
	CreateBackfillCmds cardinal.WithCommand[CreateBackfillCommand]
	CancelBackfillCmds cardinal.WithCommand[CancelBackfillCommand]

	// Entities
	Tickets cardinal.Contains[struct {
		Ticket cardinal.Ref[component.TicketComponent]
	}]

	TicketIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.TicketIndexComponent]
	}]

	Profiles cardinal.Contains[struct {
		Profile cardinal.Ref[component.ProfileComponent]
	}]

	ProfileIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.ProfileIndexComponent]
	}]

	Backfills cardinal.Contains[struct {
		Backfill cardinal.Ref[component.BackfillComponent]
	}]

	BackfillIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.BackfillIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events (client-facing)
	TicketCreatedEvents   cardinal.WithEvent[TicketCreatedEvent]
	TicketCancelledEvents cardinal.WithEvent[TicketCancelledEvent]
	TicketErrorEvents     cardinal.WithEvent[TicketErrorEvent]
	MatchFoundEvents      cardinal.WithEvent[MatchFoundEvent]
	BackfillMatchEvents   cardinal.WithEvent[BackfillMatchEvent]

	// System Events (same-shard communication to lobby)
	CreateLobbyEvents cardinal.WithSystemEventEmitter[CreateLobbyFromMatchEvent]
}

// MatchmakingSystem processes matchmaking commands and runs the matching algorithm.
func MatchmakingSystem(state *MatchmakingSystemState) error {
	now := state.Timestamp().Unix()

	// Get indexes
	var ticketIndex component.TicketIndexComponent
	var ticketIndexEntityID ecs.EntityID
	for eid, idx := range state.TicketIndexes.Iter() {
		ticketIndex = idx.Index.Get()
		ticketIndexEntityID = eid
		break
	}

	var profileIndex component.ProfileIndexComponent
	for _, idx := range state.ProfileIndexes.Iter() {
		profileIndex = idx.Index.Get()
		break
	}

	var backfillIndex component.BackfillIndexComponent
	var backfillIndexEntityID ecs.EntityID
	for eid, idx := range state.BackfillIndexes.Iter() {
		backfillIndex = idx.Index.Get()
		backfillIndexEntityID = eid
		break
	}

	var config component.ConfigComponent
	for _, cfg := range state.Configs.Iter() {
		config = cfg.Config.Get()
		break
	}

	// Process create ticket commands
	for cmd := range state.CreateTicketCmds.Iter() {
		payload := cmd.Payload()

		// Validate: check if any player already has a ticket
		hasExisting := false
		for _, player := range payload.Players {
			if ticketIndex.HasPlayer(player.PlayerID) {
				state.TicketErrorEvents.Emit(TicketErrorEvent{
					PartyID: payload.PartyID,
					Error:   "player " + player.PlayerID + " already has an active ticket",
				})
				hasExisting = true
				break
			}
		}
		if hasExisting {
			continue
		}

		// Validate: check if profile exists
		if _, exists := profileIndex.GetEntityID(payload.ProfileName); !exists {
			state.TicketErrorEvents.Emit(TicketErrorEvent{
				PartyID: payload.PartyID,
				Error:   "unknown profile: " + payload.ProfileName,
			})
			continue
		}

		// Calculate TTL
		ttl := payload.TTLSeconds
		if ttl <= 0 {
			ttl = config.DefaultTTLSeconds
		}
		if ttl <= 0 {
			ttl = 300 // fallback 5 minutes
		}

		// Create ticket
		ticketID := uuid.New().String()
		eid, ticketEntity := state.Tickets.Create()
		ticketEntity.Ticket.Set(component.TicketComponent{
			ID:            ticketID,
			PartyID:       payload.PartyID,
			ProfileName:   payload.ProfileName,
			Players:       payload.Players,
			AllowBackfill: payload.AllowBackfill,
			CreatedAt:     now,
			ExpiresAt:     now + ttl,
			Attributes:    payload.Attributes,
		})

		// Update index
		playerIDs := make([]string, len(payload.Players))
		for i, p := range payload.Players {
			playerIDs[i] = p.PlayerID
		}
		ticketIndex.AddTicket(ticketID, uint32(eid), payload.ProfileName, playerIDs, payload.AllowBackfill)

		// Emit event
		state.TicketCreatedEvents.Emit(TicketCreatedEvent{
			TicketID: ticketID,
			PartyID:  payload.PartyID,
		})

		state.Logger().Debug().
			Str("ticket_id", ticketID).
			Str("party_id", payload.PartyID).
			Str("profile", payload.ProfileName).
			Msg("Created ticket")
	}

	// Process cancel ticket commands
	for cmd := range state.CancelTicketCmds.Iter() {
		payload := cmd.Payload()

		entityID, exists := ticketIndex.GetEntityID(payload.TicketID)
		if !exists {
			continue
		}

		ticketEntity, ok := state.Tickets.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		ticket := ticketEntity.Ticket.Get()

		// Update index
		ticketIndex.RemoveTicket(ticket.ID, ticket.ProfileName, ticket.GetPlayerIDs(), ticket.AllowBackfill)

		// Destroy entity
		state.Tickets.Destroy(ecs.EntityID(entityID))

		// Emit event
		state.TicketCancelledEvents.Emit(TicketCancelledEvent{
			TicketID: payload.TicketID,
		})

		state.Logger().Debug().
			Str("ticket_id", payload.TicketID).
			Msg("Cancelled ticket")
	}

	// Process create backfill commands
	for cmd := range state.CreateBackfillCmds.Iter() {
		payload := cmd.Payload()

		// Calculate TTL
		ttl := payload.TTLSeconds
		if ttl <= 0 {
			ttl = config.BackfillTTLSeconds
		}
		if ttl <= 0 {
			ttl = 60 // fallback 1 minute
		}

		// Create backfill request
		backfillID := uuid.New().String()
		eid, backfillEntity := state.Backfills.Create()
		backfillEntity.Backfill.Set(component.BackfillComponent{
			ID:          backfillID,
			MatchID:     payload.MatchID,
			ProfileName: payload.ProfileName,
			TeamName:    payload.TeamName,
			SlotsNeeded: payload.SlotsNeeded,
			CreatedAt:   now,
			ExpiresAt:   now + ttl,
		})

		// Update index
		backfillIndex.AddBackfill(backfillID, uint32(eid), payload.MatchID, payload.ProfileName)

		state.Logger().Debug().
			Str("backfill_id", backfillID).
			Str("match_id", payload.MatchID).
			Int("slots", payload.SlotsNeeded).
			Msg("Created backfill request")
	}

	// Process cancel backfill commands
	for cmd := range state.CancelBackfillCmds.Iter() {
		payload := cmd.Payload()

		entityID, exists := backfillIndex.GetEntityID(payload.BackfillID)
		if !exists {
			continue
		}

		backfillEntity, ok := state.Backfills.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		backfill := backfillEntity.Backfill.Get()

		// Update index
		backfillIndex.RemoveBackfill(backfill.ID, backfill.MatchID, backfill.ProfileName)

		// Destroy entity
		state.Backfills.Destroy(ecs.EntityID(entityID))

		state.Logger().Debug().
			Str("backfill_id", payload.BackfillID).
			Msg("Cancelled backfill request")
	}

	// Expire old tickets
	expiredTickets := []struct {
		entityID  ecs.EntityID
		ticketID  string
		profile   string
		playerIDs []string
		backfill  bool
	}{}

	for eid, ticketEntity := range state.Tickets.Iter() {
		ticket := ticketEntity.Ticket.Get()
		if ticket.IsExpired(now) {
			expiredTickets = append(expiredTickets, struct {
				entityID  ecs.EntityID
				ticketID  string
				profile   string
				playerIDs []string
				backfill  bool
			}{
				entityID:  eid,
				ticketID:  ticket.ID,
				profile:   ticket.ProfileName,
				playerIDs: ticket.GetPlayerIDs(),
				backfill:  ticket.AllowBackfill,
			})
		}
	}

	for _, expired := range expiredTickets {
		ticketIndex.RemoveTicket(expired.ticketID, expired.profile, expired.playerIDs, expired.backfill)
		state.Tickets.Destroy(expired.entityID)
	}

	if len(expiredTickets) > 0 {
		state.Logger().Debug().Int("count", len(expiredTickets)).Msg("Expired tickets")
	}

	// Expire old backfill requests
	expiredBackfills := []struct {
		entityID   ecs.EntityID
		backfillID string
		matchID    string
		profile    string
	}{}

	for eid, backfillEntity := range state.Backfills.Iter() {
		backfill := backfillEntity.Backfill.Get()
		if backfill.IsExpired(now) {
			expiredBackfills = append(expiredBackfills, struct {
				entityID   ecs.EntityID
				backfillID string
				matchID    string
				profile    string
			}{
				entityID:   eid,
				backfillID: backfill.ID,
				matchID:    backfill.MatchID,
				profile:    backfill.ProfileName,
			})
		}
	}

	for _, expired := range expiredBackfills {
		backfillIndex.RemoveBackfill(expired.backfillID, expired.matchID, expired.profile)
		state.Backfills.Destroy(expired.entityID)
	}

	if len(expiredBackfills) > 0 {
		state.Logger().Debug().Int("count", len(expiredBackfills)).Msg("Expired backfill requests")
	}

	// Run matching algorithm for each profile
	runMatching(state, &ticketIndex, &backfillIndex, &config, time.Unix(now, 0))

	// Save indexes back
	if ticketIndexEntity, ok := state.TicketIndexes.GetByID(ticketIndexEntityID); ok {
		ticketIndexEntity.Index.Set(ticketIndex)
	}

	if backfillIndexEntity, ok := state.BackfillIndexes.GetByID(backfillIndexEntityID); ok {
		backfillIndexEntity.Index.Set(backfillIndex)
	}

	return nil
}

// runMatching runs the matching algorithm for all profiles.
func runMatching(
	state *MatchmakingSystemState,
	ticketIndex *component.TicketIndexComponent,
	backfillIndex *component.BackfillIndexComponent,
	config *component.ConfigComponent,
	now time.Time,
) {
	// Get all profiles
	profiles := make(map[string]component.ProfileComponent)
	for _, profileEntity := range state.Profiles.Iter() {
		profile := profileEntity.Profile.Get()
		profiles[profile.ProfileName] = profile
	}

	// For each profile, run matching
	for profileName, profile := range profiles {
		// Get tickets for this profile
		ticketIDs := ticketIndex.GetTicketsByProfile(profileName)
		if len(ticketIDs) == 0 {
			continue
		}

		// Convert tickets to algorithm input
		var candidates []algorithm.Ticket
		ticketMap := make(map[string]*ticketAdapter) // For looking up tickets by ID after matching

		for _, ticketID := range ticketIDs {
			entityID, exists := ticketIndex.GetEntityID(ticketID)
			if !exists {
				continue
			}
			ticketEntity, ok := state.Tickets.GetByID(ecs.EntityID(entityID))
			if !ok {
				continue
			}
			ticket := ticketEntity.Ticket.Get()
			adapter := &ticketAdapter{ticket: ticket, entityID: entityID}
			candidates = append(candidates, adapter)
			ticketMap[ticketID] = adapter
		}

		if len(candidates) == 0 {
			continue
		}

		// Create profile adapter
		profileAdapter := &profileAdapter{profile: profile}

		// Run the algorithm
		input := algorithm.NewInput(candidates, profileAdapter, now)
		output := algorithm.Run(input)

		if !output.Success {
			continue
		}

		// Process the match result
		matchID := uuid.New().String()
		teams := make([]MatchTeam, len(profile.Teams))
		for i := range teams {
			teams[i] = MatchTeam{
				TeamName:  profile.Teams[i].Name,
				TicketIDs: []string{},
			}
		}

		// Group assignments by team
		for _, assignment := range output.Assignments {
			if assignment.TeamIndex >= 0 && assignment.TeamIndex < len(teams) {
				teams[assignment.TeamIndex].TicketIDs = append(
					teams[assignment.TeamIndex].TicketIDs,
					assignment.Ticket.GetID(),
				)
			}
		}

		// Remove matched tickets from the index and destroy entities
		for _, assignment := range output.Assignments {
			ticketID := assignment.Ticket.GetID()
			adapter, ok := ticketMap[ticketID]
			if !ok {
				continue
			}

			// Remove from index
			ticket := adapter.ticket
			ticketIndex.RemoveTicket(
				ticket.ID,
				ticket.ProfileName,
				ticket.GetPlayerIDs(),
				ticket.AllowBackfill,
			)

			// Destroy entity
			state.Tickets.Destroy(ecs.EntityID(adapter.entityID))
		}

		// Emit match found event (client-facing)
		state.MatchFoundEvents.Emit(MatchFoundEvent{
			MatchID:     matchID,
			ProfileName: profileName,
			Teams:       teams,
		})

		// Convert teams to LobbyTeamInfo format
		lobbyTeams := make([]LobbyTeamInfo, len(teams))
		for i, team := range teams {
			lobbyTeams[i] = LobbyTeamInfo{
				TeamName: team.TeamName,
				PartyIDs: team.TicketIDs, // In matchmaking context, ticket IDs are party IDs
			}
		}

		// Send to lobby system (same-shard or cross-shard)
		if config.LobbyShardID != "" {
			// Cross-shard: send command to lobby shard
			lobbyWorld := cardinal.OtherWorld{
				ShardID: config.LobbyShardID,
				// Region, Organization, Project will use defaults from the shard
			}
			lobbyWorld.Send(&state.BaseSystemState, CreateLobbyFromMatchCommand{
				MatchID:     matchID,
				ProfileName: profileName,
				Teams:       lobbyTeams,
			})
			state.Logger().Debug().
				Str("match_id", matchID).
				Str("lobby_shard", config.LobbyShardID).
				Msg("Sent CreateLobbyFromMatchCommand to lobby shard")
		} else {
			// Same-shard: emit system event for lobby system to receive
			state.CreateLobbyEvents.Emit(CreateLobbyFromMatchEvent{
				MatchID:     matchID,
				ProfileName: profileName,
				Teams:       lobbyTeams,
			})
			state.Logger().Debug().
				Str("match_id", matchID).
				Msg("Emitted CreateLobbyFromMatchEvent (same-shard)")
		}

		state.Logger().Info().
			Str("match_id", matchID).
			Str("profile", profileName).
			Int("tickets", len(output.Assignments)).
			Msg("Match found")
	}

	// TODO: Handle backfill matching
	// This would match waiting tickets to active backfill requests
}
