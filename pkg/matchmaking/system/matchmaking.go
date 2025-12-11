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
	MatchID          string                `json:"match_id"`
	ProfileName      string                `json:"profile_name"`
	TeamName         string                `json:"team_name"`
	SlotsNeeded      int                   `json:"slots_needed"`                         // Simple slot count (for non-role-based backfill)
	SlotsNeededByRole []RequestBackfillSlot `json:"slots_needed_by_role,omitempty"` // Role-specific slots (for role-based backfill)
	TTLSeconds       int64                 `json:"ttl_seconds,omitempty"`
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

// RequestBackfillSlot represents a slot needed for a specific pool/role.
type RequestBackfillSlot struct {
	PoolName string `json:"pool_name"`
	Count    int    `json:"count"`
}

// RequestBackfillCommand is received from lobby shard when backfill is needed.
// This is used for role-based backfill where specific slots are needed.
type RequestBackfillCommand struct {
	cardinal.BaseCommand
	MatchID     string                `json:"match_id"`
	ProfileName string                `json:"profile_name"`
	TeamName    string                `json:"team_name"`
	Slots       []RequestBackfillSlot `json:"slots"` // Role-specific slots needed
}

// Name returns the command name.
func (RequestBackfillCommand) Name() string { return "lobby_request_backfill" }

// GetTicketsCommand queries all tickets in the matchmaking queue.
// Used by game shards to get ticket list.
type GetTicketsCommand struct {
	cardinal.BaseCommand
	ProfileName   string              `json:"profile_name,omitempty"` // Optional filter by profile
	SendbackWorld cardinal.OtherWorld `json:"sendback_world"`         // Where to send the response
}

// Name returns the command name.
func (GetTicketsCommand) Name() string { return "matchmaking_get_tickets" }

// TicketsListResponse is sent back to the requesting shard with ticket list.
type TicketsListResponse struct {
	cardinal.BaseCommand
	Tickets []TicketInfo `json:"tickets"`
	Total   int          `json:"total"`
}

// Name returns the command name.
func (TicketsListResponse) Name() string { return "matchmaking_tickets_list" }

// TicketInfo represents a ticket in the response.
type TicketInfo struct {
	TicketID    string            `json:"ticket_id"`
	PartyID     string            `json:"party_id"`
	ProfileName string            `json:"profile_name"`
	Players     []MatchedPlayer   `json:"players"`
	CreatedAt   int64             `json:"created_at"`
	PoolCounts  map[string]int    `json:"pool_counts,omitempty"`
}

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

// MatchedSearchFields contains search fields for event serialization.
// Uses map[string]any for DoubleArgs to ensure proper JSON/protobuf serialization.
type MatchedSearchFields struct {
	StringArgs map[string]string `json:"string_args,omitempty"`
	DoubleArgs map[string]any    `json:"double_args,omitempty"`
	Tags       []string          `json:"tags,omitempty"`
}

// MatchedPlayer contains player info for event serialization.
type MatchedPlayer struct {
	PlayerID     string              `json:"player_id"`
	SearchFields MatchedSearchFields `json:"search_fields"`
}

// MatchedTicket contains the full ticket info for a matched ticket.
type MatchedTicket struct {
	TicketID  string          `json:"ticket_id"`
	PartyID   string          `json:"party_id"`
	Players   []MatchedPlayer `json:"players"`
	CreatedAt int64           `json:"created_at"`
}

// toMatchedPlayers converts []component.PlayerInfo to []MatchedPlayer.
func toMatchedPlayers(players []component.PlayerInfo) []MatchedPlayer {
	result := make([]MatchedPlayer, len(players))
	for i, p := range players {
		// Convert map[string]float64 to map[string]any for proper serialization
		doubleArgs := make(map[string]any, len(p.SearchFields.DoubleArgs))
		for k, v := range p.SearchFields.DoubleArgs {
			doubleArgs[k] = v
		}
		result[i] = MatchedPlayer{
			PlayerID: p.PlayerID,
			SearchFields: MatchedSearchFields{
				StringArgs: p.SearchFields.StringArgs,
				DoubleArgs: doubleArgs,
				Tags:       p.SearchFields.Tags,
			},
		}
	}
	return result
}

// MatchTeam represents a team in a match.
type MatchTeam struct {
	TeamName string          `json:"team_name"`
	Tickets  []MatchedTicket `json:"tickets"`
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
	BackfillID string          `json:"backfill_id"`
	MatchID    string          `json:"match_id"`
	TeamName   string          `json:"team_name"`
	Tickets    []MatchedTicket `json:"tickets"`
}

// Name returns the event name.
func (BackfillMatchEvent) Name() string { return "matchmaking_backfill_match" }

// -----------------------------------------------------------------------------
// System Events (for same-shard communication)
// -----------------------------------------------------------------------------

// LobbyPlayerInfo represents a player for lobby creation.
type LobbyPlayerInfo struct {
	PlayerID     string            `json:"player_id"`
	SearchFields MatchedSearchFields `json:"search_fields"`
}

// LobbyPartyInfo represents a party for lobby creation.
type LobbyPartyInfo struct {
	PartyID string            `json:"party_id"`
	Players []LobbyPlayerInfo `json:"players"`
}

// LobbyTeamInfo represents a team for lobby creation.
type LobbyTeamInfo struct {
	TeamName string           `json:"team_name"`
	Parties  []LobbyPartyInfo `json:"parties"`
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
	CreateTicketCmds    cardinal.WithCommand[CreateTicketCommand]
	CancelTicketCmds    cardinal.WithCommand[CancelTicketCommand]
	CreateBackfillCmds  cardinal.WithCommand[CreateBackfillCommand]
	CancelBackfillCmds  cardinal.WithCommand[CancelBackfillCommand]
	RequestBackfillCmds cardinal.WithCommand[RequestBackfillCommand]

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

		// Validate: check if profile exists and get it for pool matching
		profileEntityID, profileExists := profileIndex.GetEntityID(payload.ProfileName)
		if !profileExists {
			state.TicketErrorEvents.Emit(TicketErrorEvent{
				PartyID: payload.PartyID,
				Error:   "unknown profile: " + payload.ProfileName,
			})
			continue
		}

		// Get profile to compute pool counts
		var profile component.ProfileComponent
		if profileEntity, ok := state.Profiles.GetByID(ecs.EntityID(profileEntityID)); ok {
			profile = profileEntity.Profile.Get()
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
		ticket := component.TicketComponent{
			ID:            ticketID,
			PartyID:       payload.PartyID,
			ProfileName:   payload.ProfileName,
			Players:       payload.Players,
			AllowBackfill: payload.AllowBackfill,
			CreatedAt:     now,
			ExpiresAt:     now + ttl,
			Attributes:    payload.Attributes,
		}
		// Compute pool counts based on player search fields matching pool filters
		ticket.PoolCounts = ticket.ComputePoolCounts(profile.Pools)
		ticketEntity.Ticket.Set(ticket)

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

		// Handle role-specific slots if provided
		var slots []component.BackfillSlot
		slotsNeeded := payload.SlotsNeeded
		if len(payload.SlotsNeededByRole) > 0 {
			slots = make([]component.BackfillSlot, len(payload.SlotsNeededByRole))
			slotsNeeded = 0
			for i, slot := range payload.SlotsNeededByRole {
				slots[i] = component.BackfillSlot{
					PoolName: slot.PoolName,
					Count:    slot.Count,
				}
				slotsNeeded += slot.Count
			}
		}

		// Create backfill request
		backfillID := uuid.New().String()
		eid, backfillEntity := state.Backfills.Create()
		backfillEntity.Backfill.Set(component.BackfillComponent{
			ID:          backfillID,
			MatchID:     payload.MatchID,
			ProfileName: payload.ProfileName,
			TeamName:    payload.TeamName,
			SlotsNeeded: slotsNeeded,
			Slots:       slots,
			CreatedAt:   now,
			ExpiresAt:   now + ttl,
		})

		// Update index
		backfillIndex.AddBackfill(backfillID, uint32(eid), payload.MatchID, payload.ProfileName)

		state.Logger().Debug().
			Str("backfill_id", backfillID).
			Str("match_id", payload.MatchID).
			Int("slots", slotsNeeded).
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

	// Process RequestBackfill commands (cross-shard from lobby shard)
	for cmd := range state.RequestBackfillCmds.Iter() {
		payload := cmd.Payload()
		state.Logger().Info().
			Str("match_id", payload.MatchID).
			Str("profile", payload.ProfileName).
			Str("team", payload.TeamName).
			Int("slots", len(payload.Slots)).
			Msg("[CROSS-SHARD] Received RequestBackfill command from lobby shard")

		// Calculate TTL
		ttl := config.BackfillTTLSeconds
		if ttl <= 0 {
			ttl = 60 // fallback 1 minute
		}

		// Convert slots
		slots := make([]component.BackfillSlot, len(payload.Slots))
		totalSlots := 0
		for i, slot := range payload.Slots {
			slots[i] = component.BackfillSlot{
				PoolName: slot.PoolName,
				Count:    slot.Count,
			}
			totalSlots += slot.Count
		}

		// Create backfill request
		backfillID := uuid.New().String()
		eid, backfillEntity := state.Backfills.Create()
		backfillEntity.Backfill.Set(component.BackfillComponent{
			ID:          backfillID,
			MatchID:     payload.MatchID,
			ProfileName: payload.ProfileName,
			TeamName:    payload.TeamName,
			SlotsNeeded: totalSlots,
			Slots:       slots,
			CreatedAt:   now,
			ExpiresAt:   now + ttl,
		})

		// Update index
		backfillIndex.AddBackfill(backfillID, uint32(eid), payload.MatchID, payload.ProfileName)

		state.Logger().Info().
			Str("backfill_id", backfillID).
			Str("match_id", payload.MatchID).
			Str("profile", payload.ProfileName).
			Int("total_slots", totalSlots).
			Msg("Created backfill request from lobby disconnect")
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
				TeamName: profile.Teams[i].Name,
				Tickets:  []MatchedTicket{},
			}
		}

		// Group assignments by team with full ticket info
		for _, assignment := range output.Assignments {
			if assignment.TeamIndex >= 0 && assignment.TeamIndex < len(teams) {
				ticketID := assignment.Ticket.GetID()
				adapter := ticketMap[ticketID]
				if adapter != nil {
					teams[assignment.TeamIndex].Tickets = append(
						teams[assignment.TeamIndex].Tickets,
						MatchedTicket{
							TicketID:  adapter.ticket.ID,
							PartyID:   adapter.ticket.PartyID,
							Players:   toMatchedPlayers(adapter.ticket.Players),
							CreatedAt: adapter.ticket.CreatedAt,
						},
					)
				}
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

		// Convert teams to LobbyTeamInfo format with full player info
		lobbyTeams := make([]LobbyTeamInfo, len(teams))
		for i, team := range teams {
			parties := make([]LobbyPartyInfo, len(team.Tickets))
			for j, ticket := range team.Tickets {
				players := make([]LobbyPlayerInfo, len(ticket.Players))
				for k, player := range ticket.Players {
					players[k] = LobbyPlayerInfo{
						PlayerID:     player.PlayerID,
						SearchFields: player.SearchFields,
					}
				}
				parties[j] = LobbyPartyInfo{
					PartyID: ticket.PartyID,
					Players: players,
				}
			}
			lobbyTeams[i] = LobbyTeamInfo{
				TeamName: team.TeamName,
				Parties:  parties,
			}
		}

		// Send to lobby system (same-shard or cross-shard)
		if config.LobbyShardID != "" {
			// Cross-shard: send command to lobby shard
			lobbyWorld := cardinal.OtherWorld{
				Region:       config.LobbyRegion,
				Organization: config.LobbyOrganization,
				Project:      config.LobbyProject,
				ShardID:      config.LobbyShardID,
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

	// Handle backfill matching
	// Match waiting tickets (that allow backfill) to active backfill requests
	runBackfillMatching(state, ticketIndex, backfillIndex, now)
}

// runBackfillMatching matches tickets to backfill requests.
func runBackfillMatching(
	state *MatchmakingSystemState,
	ticketIndex *component.TicketIndexComponent,
	backfillIndex *component.BackfillIndexComponent,
	now time.Time,
) {
	// Get all profiles that have backfill requests
	profiles := make(map[string]bool)
	for _, profileEntity := range state.Profiles.Iter() {
		profile := profileEntity.Profile.Get()
		profiles[profile.ProfileName] = true
	}

	for profileName := range profiles {
		// Get backfill requests for this profile
		backfillIDs := backfillIndex.GetBackfillsByProfile(profileName)
		if len(backfillIDs) == 0 {
			continue
		}

		// Get backfill-eligible tickets for this profile
		ticketIDs := ticketIndex.GetBackfillEligible(profileName)
		if len(ticketIDs) == 0 {
			continue
		}

		// Process each backfill request
		for _, backfillID := range backfillIDs {
			backfillEntityID, exists := backfillIndex.GetEntityID(backfillID)
			if !exists {
				continue
			}

			backfillEntity, ok := state.Backfills.GetByID(ecs.EntityID(backfillEntityID))
			if !ok {
				continue
			}

			backfill := backfillEntity.Backfill.Get()
			slotsNeeded := backfill.TotalSlotsNeeded()

			state.Logger().Debug().
				Str("backfill_id", backfillID).
				Str("match_id", backfill.MatchID).
				Int("slots_needed", slotsNeeded).
				Int("available_tickets", len(ticketIDs)).
				Msg("Processing backfill request")

			// Collect candidates for this backfill
			var candidates []algorithm.Ticket
			ticketMap := make(map[string]*ticketAdapter)

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

			// Build slots needed for algorithm
			var slotsNeededForAlgo []algorithm.SlotNeeded
			if len(backfill.Slots) > 0 {
				// Role-based backfill
				for _, slot := range backfill.Slots {
					slotsNeededForAlgo = append(slotsNeededForAlgo, algorithm.SlotNeeded{
						PoolName: slot.PoolName,
						Count:    slot.Count,
					})
				}
			} else {
				// Simple backfill - use "default" pool
				slotsNeededForAlgo = []algorithm.SlotNeeded{
					{PoolName: "default", Count: slotsNeeded},
				}
			}

			// Run backfill algorithm
			input := algorithm.NewBackfillInput(candidates, slotsNeededForAlgo, now)
			output := algorithm.Run(input)

			if !output.Success {
				state.Logger().Debug().
					Str("backfill_id", backfillID).
					Int("candidates", len(candidates)).
					Msg("Backfill matching failed - not enough candidates")
				continue
			}

			// Collect matched tickets with full info
			var matchedTickets []MatchedTicket
			var matchedTicketIDs []string

			for _, assignment := range output.Assignments {
				ticketID := assignment.Ticket.GetID()
				matchedTicketIDs = append(matchedTicketIDs, ticketID)

				if adapter, ok := ticketMap[ticketID]; ok {
					matchedTickets = append(matchedTickets, MatchedTicket{
						TicketID:  adapter.ticket.ID,
						PartyID:   adapter.ticket.PartyID,
						Players:   toMatchedPlayers(adapter.ticket.Players),
						CreatedAt: adapter.ticket.CreatedAt,
					})

					// Remove ticket from index
					ticketIndex.RemoveTicket(
						adapter.ticket.ID,
						adapter.ticket.ProfileName,
						adapter.ticket.GetPlayerIDs(),
						adapter.ticket.AllowBackfill,
					)

					// Destroy ticket entity
					state.Tickets.Destroy(ecs.EntityID(adapter.entityID))
				}
			}

			// Remove from ticketIDs slice so they're not reused for other backfills
			newTicketIDs := make([]string, 0, len(ticketIDs))
			matchedSet := make(map[string]bool)
			for _, tid := range matchedTicketIDs {
				matchedSet[tid] = true
			}
			for _, tid := range ticketIDs {
				if !matchedSet[tid] {
					newTicketIDs = append(newTicketIDs, tid)
				}
			}
			ticketIDs = newTicketIDs

			// Emit backfill match event
			state.BackfillMatchEvents.Emit(BackfillMatchEvent{
				BackfillID: backfillID,
				MatchID:    backfill.MatchID,
				TeamName:   backfill.TeamName,
				Tickets:    matchedTickets,
			})

			state.Logger().Info().
				Str("backfill_id", backfillID).
				Str("match_id", backfill.MatchID).
				Strs("ticket_ids", matchedTicketIDs).
				Msg("Backfill match found")

			// Remove backfill request
			backfillIndex.RemoveBackfill(backfillID, backfill.MatchID, backfill.ProfileName)
			state.Backfills.Destroy(ecs.EntityID(backfillEntityID))
		}
	}
}
