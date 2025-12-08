package system

import (
	"github.com/google/uuid"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
)

// -----------------------------------------------------------------------------
// Party Commands
// -----------------------------------------------------------------------------

// CreatePartyCommand creates a new party with the sender as leader.
type CreatePartyCommand struct {
	cardinal.BaseCommand
	PlayerID string `json:"player_id"`
	MaxSize  int    `json:"max_size,omitempty"`
	IsOpen   bool   `json:"is_open"`
}

// Name returns the command name.
func (CreatePartyCommand) Name() string { return "lobby_create_party" }

// JoinPartyCommand adds a player to an existing party.
type JoinPartyCommand struct {
	cardinal.BaseCommand
	PlayerID string `json:"player_id"`
	PartyID  string `json:"party_id"`
}

// Name returns the command name.
func (JoinPartyCommand) Name() string { return "lobby_join_party" }

// LeavePartyCommand removes a player from their party.
type LeavePartyCommand struct {
	cardinal.BaseCommand
	PlayerID string `json:"player_id"`
}

// Name returns the command name.
func (LeavePartyCommand) Name() string { return "lobby_leave_party" }

// SetPartyOpenCommand sets whether a party is open for joining.
type SetPartyOpenCommand struct {
	cardinal.BaseCommand
	PlayerID string `json:"player_id"` // Must be leader
	IsOpen   bool   `json:"is_open"`
}

// Name returns the command name.
func (SetPartyOpenCommand) Name() string { return "lobby_set_party_open" }

// PromoteLeaderCommand promotes another member to party leader.
type PromoteLeaderCommand struct {
	cardinal.BaseCommand
	PlayerID    string `json:"player_id"` // Current leader
	NewLeaderID string `json:"new_leader_id"`
}

// Name returns the command name.
func (PromoteLeaderCommand) Name() string { return "lobby_promote_leader" }

// KickFromPartyCommand removes a player from a party (leader only).
type KickFromPartyCommand struct {
	cardinal.BaseCommand
	PlayerID     string `json:"player_id"` // Must be leader
	TargetPlayer string `json:"target_player"`
}

// Name returns the command name.
func (KickFromPartyCommand) Name() string { return "lobby_kick_from_party" }

// -----------------------------------------------------------------------------
// Party Events
// -----------------------------------------------------------------------------

// PartyCreatedEvent is emitted when a party is created.
type PartyCreatedEvent struct {
	cardinal.BaseEvent
	PartyID  string `json:"party_id"`
	LeaderID string `json:"leader_id"`
}

// Name returns the event name.
func (PartyCreatedEvent) Name() string { return "lobby_party_created" }

// PlayerJoinedPartyEvent is emitted when a player joins a party.
type PlayerJoinedPartyEvent struct {
	cardinal.BaseEvent
	PartyID  string `json:"party_id"`
	PlayerID string `json:"player_id"`
}

// Name returns the event name.
func (PlayerJoinedPartyEvent) Name() string { return "lobby_player_joined_party" }

// PlayerLeftPartyEvent is emitted when a player leaves a party.
type PlayerLeftPartyEvent struct {
	cardinal.BaseEvent
	PartyID  string `json:"party_id"`
	PlayerID string `json:"player_id"`
}

// Name returns the event name.
func (PlayerLeftPartyEvent) Name() string { return "lobby_player_left_party" }

// PartyDisbandedEvent is emitted when a party is disbanded.
type PartyDisbandedEvent struct {
	cardinal.BaseEvent
	PartyID string `json:"party_id"`
}

// Name returns the event name.
func (PartyDisbandedEvent) Name() string { return "lobby_party_disbanded" }

// LeaderChangedEvent is emitted when the party leader changes.
type LeaderChangedEvent struct {
	cardinal.BaseEvent
	PartyID     string `json:"party_id"`
	NewLeaderID string `json:"new_leader_id"`
}

// Name returns the event name.
func (LeaderChangedEvent) Name() string { return "lobby_leader_changed" }

// PartyErrorEvent is emitted when a party operation fails.
type PartyErrorEvent struct {
	cardinal.BaseEvent
	PlayerID string `json:"player_id"`
	Error    string `json:"error"`
}

// Name returns the event name.
func (PartyErrorEvent) Name() string { return "lobby_party_error" }

// -----------------------------------------------------------------------------
// Party System State
// -----------------------------------------------------------------------------

// PartySystemState is the state for the party system.
type PartySystemState struct {
	cardinal.BaseSystemState

	// Commands
	CreatePartyCmds   cardinal.WithCommand[CreatePartyCommand]
	JoinPartyCmds     cardinal.WithCommand[JoinPartyCommand]
	LeavePartyCmds    cardinal.WithCommand[LeavePartyCommand]
	SetPartyOpenCmds  cardinal.WithCommand[SetPartyOpenCommand]
	PromoteLeaderCmds cardinal.WithCommand[PromoteLeaderCommand]
	KickFromPartyCmds cardinal.WithCommand[KickFromPartyCommand]

	// Entities
	Parties cardinal.Contains[struct {
		Party cardinal.Ref[component.PartyComponent]
	}]

	PartyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.PartyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events
	PartyCreatedEvents      cardinal.WithEvent[PartyCreatedEvent]
	PlayerJoinedPartyEvents cardinal.WithEvent[PlayerJoinedPartyEvent]
	PlayerLeftPartyEvents   cardinal.WithEvent[PlayerLeftPartyEvent]
	PartyDisbandedEvents    cardinal.WithEvent[PartyDisbandedEvent]
	LeaderChangedEvents     cardinal.WithEvent[LeaderChangedEvent]
	PartyErrorEvents        cardinal.WithEvent[PartyErrorEvent]
}

// PartySystem processes party commands.
func PartySystem(state *PartySystemState) error {
	now := state.Timestamp().Unix()

	// Get party index
	var partyIndex component.PartyIndexComponent
	var partyIndexEntityID ecs.EntityID
	for eid, idx := range state.PartyIndexes.Iter() {
		partyIndex = idx.Index.Get()
		partyIndexEntityID = eid
		break
	}

	// Get config
	var config component.ConfigComponent
	for _, cfg := range state.Configs.Iter() {
		config = cfg.Config.Get()
		break
	}

	// Process create party commands
	for cmd := range state.CreatePartyCmds.Iter() {
		payload := cmd.Payload()

		// Check if player already has a party
		if partyIndex.HasPlayer(payload.PlayerID) {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "player already has a party",
			})
			continue
		}

		// Determine max size
		maxSize := payload.MaxSize
		if maxSize <= 0 {
			maxSize = config.DefaultMaxPartySize
		}
		if maxSize <= 0 {
			maxSize = 4 // fallback
		}

		// Create party
		partyID := uuid.New().String()
		eid, partyEntity := state.Parties.Create()
		partyEntity.Party.Set(component.PartyComponent{
			ID:        partyID,
			LeaderID:  payload.PlayerID,
			Members:   []string{payload.PlayerID},
			IsOpen:    payload.IsOpen,
			MaxSize:   maxSize,
			CreatedAt: now,
		})

		// Update index
		partyIndex.AddParty(partyID, uint32(eid), []string{payload.PlayerID})

		// Emit event
		state.PartyCreatedEvents.Emit(PartyCreatedEvent{
			PartyID:  partyID,
			LeaderID: payload.PlayerID,
		})

		state.Logger().Debug().
			Str("party_id", partyID).
			Str("leader", payload.PlayerID).
			Msg("Created party")
	}

	// Process join party commands
	for cmd := range state.JoinPartyCmds.Iter() {
		payload := cmd.Payload()

		// Check if player already has a party
		if partyIndex.HasPlayer(payload.PlayerID) {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "player already has a party",
			})
			continue
		}

		// Get party
		entityID, exists := partyIndex.GetEntityID(payload.PartyID)
		if !exists {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "party not found",
			})
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()

		// Check if party is open
		if !party.IsOpen {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "party is closed",
			})
			continue
		}

		// Check if party is full
		if party.IsFull() {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "party is full",
			})
			continue
		}

		// Check if party is in lobby (can't join)
		if party.InLobby() {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "party is in a lobby",
			})
			continue
		}

		// Add player to party
		party.AddMember(payload.PlayerID)
		partyEntity.Party.Set(party)

		// Update index
		partyIndex.AddPlayerToParty(payload.PlayerID, party.ID)

		// Emit event
		state.PlayerJoinedPartyEvents.Emit(PlayerJoinedPartyEvent{
			PartyID:  party.ID,
			PlayerID: payload.PlayerID,
		})

		state.Logger().Debug().
			Str("party_id", party.ID).
			Str("player", payload.PlayerID).
			Msg("Player joined party")
	}

	// Process leave party commands
	for cmd := range state.LeavePartyCmds.Iter() {
		payload := cmd.Payload()

		// Get player's party
		partyID, exists := partyIndex.GetPartyByPlayer(payload.PlayerID)
		if !exists {
			continue
		}

		entityID, exists := partyIndex.GetEntityID(partyID)
		if !exists {
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()

		// Check if party is in lobby (can't leave during game)
		if party.InLobby() {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "cannot leave party while in lobby",
			})
			continue
		}

		// Remove player from party
		party.RemoveMember(payload.PlayerID)

		// Update index
		partyIndex.RemovePlayerFromParty(payload.PlayerID)

		// Emit event
		state.PlayerLeftPartyEvents.Emit(PlayerLeftPartyEvent{
			PartyID:  party.ID,
			PlayerID: payload.PlayerID,
		})

		state.Logger().Debug().
			Str("party_id", party.ID).
			Str("player", payload.PlayerID).
			Msg("Player left party")

		// If party is empty, disband it
		if party.Size() == 0 {
			partyIndex.RemoveParty(party.ID, []string{})
			state.Parties.Destroy(ecs.EntityID(entityID))
			state.PartyDisbandedEvents.Emit(PartyDisbandedEvent{
				PartyID: party.ID,
			})
			state.Logger().Debug().
				Str("party_id", party.ID).
				Msg("Party disbanded (empty)")
		} else {
			// If leader left, promote next member
			if party.LeaderID == payload.PlayerID && len(party.Members) > 0 {
				party.LeaderID = party.Members[0]
				state.LeaderChangedEvents.Emit(LeaderChangedEvent{
					PartyID:     party.ID,
					NewLeaderID: party.LeaderID,
				})
			}
			partyEntity.Party.Set(party)
		}
	}

	// Process set party open commands
	for cmd := range state.SetPartyOpenCmds.Iter() {
		payload := cmd.Payload()

		partyID, exists := partyIndex.GetPartyByPlayer(payload.PlayerID)
		if !exists {
			continue
		}

		entityID, exists := partyIndex.GetEntityID(partyID)
		if !exists {
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()

		// Only leader can change open status
		if !party.IsLeader(payload.PlayerID) {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "only party leader can change open status",
			})
			continue
		}

		party.IsOpen = payload.IsOpen
		partyEntity.Party.Set(party)

		state.Logger().Debug().
			Str("party_id", party.ID).
			Bool("is_open", payload.IsOpen).
			Msg("Party open status changed")
	}

	// Process promote leader commands
	for cmd := range state.PromoteLeaderCmds.Iter() {
		payload := cmd.Payload()

		partyID, exists := partyIndex.GetPartyByPlayer(payload.PlayerID)
		if !exists {
			continue
		}

		entityID, exists := partyIndex.GetEntityID(partyID)
		if !exists {
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()

		// Only leader can promote
		if !party.IsLeader(payload.PlayerID) {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "only party leader can promote",
			})
			continue
		}

		// Check if new leader is in party
		if !party.HasMember(payload.NewLeaderID) {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "new leader is not in party",
			})
			continue
		}

		party.LeaderID = payload.NewLeaderID
		partyEntity.Party.Set(party)

		state.LeaderChangedEvents.Emit(LeaderChangedEvent{
			PartyID:     party.ID,
			NewLeaderID: payload.NewLeaderID,
		})

		state.Logger().Debug().
			Str("party_id", party.ID).
			Str("new_leader", payload.NewLeaderID).
			Msg("Party leader changed")
	}

	// Process kick from party commands
	for cmd := range state.KickFromPartyCmds.Iter() {
		payload := cmd.Payload()

		partyID, exists := partyIndex.GetPartyByPlayer(payload.PlayerID)
		if !exists {
			continue
		}

		entityID, exists := partyIndex.GetEntityID(partyID)
		if !exists {
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(entityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()

		// Only leader can kick
		if !party.IsLeader(payload.PlayerID) {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "only party leader can kick members",
			})
			continue
		}

		// Can't kick yourself
		if payload.TargetPlayer == payload.PlayerID {
			continue
		}

		// Check if target is in party
		if !party.HasMember(payload.TargetPlayer) {
			continue
		}

		// Can't kick while in lobby
		if party.InLobby() {
			state.PartyErrorEvents.Emit(PartyErrorEvent{
				PlayerID: payload.PlayerID,
				Error:    "cannot kick while in lobby",
			})
			continue
		}

		// Remove player from party
		party.RemoveMember(payload.TargetPlayer)
		partyEntity.Party.Set(party)

		// Update index
		partyIndex.RemovePlayerFromParty(payload.TargetPlayer)

		// Emit event
		state.PlayerLeftPartyEvents.Emit(PlayerLeftPartyEvent{
			PartyID:  party.ID,
			PlayerID: payload.TargetPlayer,
		})

		state.Logger().Debug().
			Str("party_id", party.ID).
			Str("kicked_player", payload.TargetPlayer).
			Msg("Player kicked from party")
	}

	// Save index back
	if partyIndexEntity, ok := state.PartyIndexes.GetByID(partyIndexEntityID); ok {
		partyIndexEntity.Index.Set(partyIndex)
	}

	return nil
}
