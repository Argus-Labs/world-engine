package system

import (
	"github.com/google/uuid"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
)

// -----------------------------------------------------------------------------
// Lobby Commands
// -----------------------------------------------------------------------------

// CreateLobbyCommand creates a new lobby (usually from matchmaking result).
type CreateLobbyCommand struct {
	cardinal.BaseCommand
	MatchID          string                `json:"match_id,omitempty"` // From matchmaking; if empty, auto-generated
	HostPartyID      string                `json:"host_party_id"`
	Teams            []component.LobbyTeam `json:"teams,omitempty"`
	MatchProfileName string                `json:"match_profile_name,omitempty"`
	MinPlayers       int                   `json:"min_players"`
	MaxPlayers       int                   `json:"max_players"`
	Config           map[string]string     `json:"config,omitempty"`
}

// Name returns the command name.
func (CreateLobbyCommand) Name() string { return "lobby_create" }

// JoinLobbyCommand adds a party to a lobby.
type JoinLobbyCommand struct {
	cardinal.BaseCommand
	PartyID string `json:"party_id"`
	MatchID string `json:"match_id"`
}

// Name returns the command name.
func (JoinLobbyCommand) Name() string { return "lobby_join" }

// LeaveLobbyCommand removes a party from a lobby.
type LeaveLobbyCommand struct {
	cardinal.BaseCommand
	PartyID string `json:"party_id"`
}

// Name returns the command name.
func (LeaveLobbyCommand) Name() string { return "lobby_leave" }

// SetReadyCommand sets a party's ready status.
type SetReadyCommand struct {
	cardinal.BaseCommand
	PartyID string `json:"party_id"`
	IsReady bool   `json:"is_ready"`
}

// Name returns the command name.
func (SetReadyCommand) Name() string { return "lobby_set_ready" }

// StartGameCommand starts the game (lobby host only).
type StartGameCommand struct {
	cardinal.BaseCommand
	PartyID string `json:"party_id"` // Must be host
	MatchID string `json:"match_id"`
}

// Name returns the command name.
func (StartGameCommand) Name() string { return "lobby_start_game" }

// EndGameCommand ends the game and transitions lobby to ended state.
type EndGameCommand struct {
	cardinal.BaseCommand
	MatchID string            `json:"match_id"`
	Results map[string]string `json:"results,omitempty"`
}

// Name returns the command name.
func (EndGameCommand) Name() string { return "lobby_end_game" }

// HeartbeatCommand keeps the lobby alive during gameplay.
type HeartbeatCommand struct {
	cardinal.BaseCommand
	MatchID string `json:"match_id"`
}

// Name returns the command name.
func (HeartbeatCommand) Name() string { return "lobby_heartbeat" }

// -----------------------------------------------------------------------------
// Lobby Events
// -----------------------------------------------------------------------------

// LobbyCreatedEvent is emitted when a lobby is created.
type LobbyCreatedEvent struct {
	cardinal.BaseEvent
	MatchID     string `json:"match_id"`
	HostPartyID string `json:"host_party_id"`
}

// Name returns the event name.
func (LobbyCreatedEvent) Name() string { return "lobby_created" }

// PartyJoinedLobbyEvent is emitted when a party joins a lobby.
type PartyJoinedLobbyEvent struct {
	cardinal.BaseEvent
	MatchID string `json:"match_id"`
	PartyID string `json:"party_id"`
}

// Name returns the event name.
func (PartyJoinedLobbyEvent) Name() string { return "lobby_party_joined" }

// PartyLeftLobbyEvent is emitted when a party leaves a lobby.
type PartyLeftLobbyEvent struct {
	cardinal.BaseEvent
	MatchID string `json:"match_id"`
	PartyID string `json:"party_id"`
}

// Name returns the event name.
func (PartyLeftLobbyEvent) Name() string { return "lobby_party_left" }

// LobbyReadyEvent is emitted when all parties are ready.
type LobbyReadyEvent struct {
	cardinal.BaseEvent
	MatchID string `json:"match_id"`
}

// Name returns the event name.
func (LobbyReadyEvent) Name() string { return "lobby_ready" }

// GameStartedEvent is emitted when the game starts.
type GameStartedEvent struct {
	cardinal.BaseEvent
	MatchID          string                `json:"match_id"`
	Teams            []component.LobbyTeam `json:"teams,omitempty"`
	MatchProfileName string                `json:"match_profile_name,omitempty"`
	Config           map[string]string     `json:"config,omitempty"`
}

// Name returns the event name.
func (GameStartedEvent) Name() string { return "lobby_game_started" }

// GameEndedEvent is emitted when the game ends.
type GameEndedEvent struct {
	cardinal.BaseEvent
	MatchID string            `json:"match_id"`
	Results map[string]string `json:"results,omitempty"`
}

// Name returns the event name.
func (GameEndedEvent) Name() string { return "lobby_game_ended" }

// LobbyDisbandedEvent is emitted when a lobby is disbanded.
type LobbyDisbandedEvent struct {
	cardinal.BaseEvent
	MatchID string `json:"match_id"`
	Reason  string `json:"reason"`
}

// Name returns the event name.
func (LobbyDisbandedEvent) Name() string { return "lobby_disbanded" }

// LobbyErrorEvent is emitted when a lobby operation fails.
type LobbyErrorEvent struct {
	cardinal.BaseEvent
	PartyID string `json:"party_id,omitempty"`
	MatchID string `json:"match_id,omitempty"`
	Error   string `json:"error"`
}

// Name returns the event name.
func (LobbyErrorEvent) Name() string { return "lobby_error" }

// -----------------------------------------------------------------------------
// Cross-Shard Communication Types
// -----------------------------------------------------------------------------

// LobbyTeamInfo represents a team for lobby creation (from matchmaking).
type LobbyTeamInfo struct {
	TeamName string   `json:"team_name"`
	PartyIDs []string `json:"party_ids"`
}

// CreateLobbyFromMatchEvent is received from matchmaking system (same shard).
type CreateLobbyFromMatchEvent struct {
	MatchID     string          `json:"match_id"`
	ProfileName string          `json:"profile_name"`
	Teams       []LobbyTeamInfo `json:"teams"`
}

// Name returns the system event name.
func (CreateLobbyFromMatchEvent) Name() string { return "matchmaking_create_lobby_from_match" }

// CreateLobbyFromMatchCommand is received from matchmaking shard (cross-shard).
type CreateLobbyFromMatchCommand struct {
	cardinal.BaseCommand
	MatchID     string          `json:"match_id"`
	ProfileName string          `json:"profile_name"`
	Teams       []LobbyTeamInfo `json:"teams"`
}

// Name returns the command name.
func (CreateLobbyFromMatchCommand) Name() string { return "matchmaking_create_lobby_from_match" }

// NotifyGameStartCommand is sent to game shard when game starts.
type NotifyGameStartCommand struct {
	cardinal.BaseCommand
	MatchID          string                `json:"match_id"`
	Teams            []component.LobbyTeam `json:"teams,omitempty"`
	MatchProfileName string                `json:"match_profile_name,omitempty"`
	Config           map[string]string     `json:"config,omitempty"`
}

// Name returns the command name.
func (NotifyGameStartCommand) Name() string { return "lobby_notify_game_start" }

// NotifyGameStartEvent is a system event sent to game system (same shard).
type NotifyGameStartEvent struct {
	MatchID          string                `json:"match_id"`
	Teams            []component.LobbyTeam `json:"teams,omitempty"`
	MatchProfileName string                `json:"match_profile_name,omitempty"`
	Config           map[string]string     `json:"config,omitempty"`
}

// Name returns the system event name.
func (NotifyGameStartEvent) Name() string { return "lobby_notify_game_start" }

// -----------------------------------------------------------------------------
// Lobby System State
// -----------------------------------------------------------------------------

// LobbySystemState is the state for the lobby system.
type LobbySystemState struct {
	cardinal.BaseSystemState

	// Commands
	CreateLobbyCmds cardinal.WithCommand[CreateLobbyCommand]
	JoinLobbyCmds   cardinal.WithCommand[JoinLobbyCommand]
	LeaveLobbyCmds  cardinal.WithCommand[LeaveLobbyCommand]
	SetReadyCmds    cardinal.WithCommand[SetReadyCommand]
	StartGameCmds   cardinal.WithCommand[StartGameCommand]
	EndGameCmds     cardinal.WithCommand[EndGameCommand]
	HeartbeatCmds   cardinal.WithCommand[HeartbeatCommand]

	// Entities
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}]

	Parties cardinal.Contains[struct {
		Party cardinal.Ref[component.PartyComponent]
	}]

	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	PartyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.PartyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events (client-facing)
	LobbyCreatedEvents     cardinal.WithEvent[LobbyCreatedEvent]
	PartyJoinedLobbyEvents cardinal.WithEvent[PartyJoinedLobbyEvent]
	PartyLeftLobbyEvents   cardinal.WithEvent[PartyLeftLobbyEvent]
	LobbyReadyEvents       cardinal.WithEvent[LobbyReadyEvent]
	GameStartedEvents      cardinal.WithEvent[GameStartedEvent]
	GameEndedEvents        cardinal.WithEvent[GameEndedEvent]
	LobbyDisbandedEvents   cardinal.WithEvent[LobbyDisbandedEvent]
	LobbyErrorEvents       cardinal.WithEvent[LobbyErrorEvent]

	// Cross-shard Commands (from matchmaking shard)
	CreateLobbyFromMatchCmds cardinal.WithCommand[CreateLobbyFromMatchCommand]

	// System Events (same-shard communication)
	CreateLobbyFromMatchEvents cardinal.WithSystemEventReceiver[CreateLobbyFromMatchEvent]
	NotifyGameStartEvents      cardinal.WithSystemEventEmitter[NotifyGameStartEvent]
}

// LobbySystem processes lobby commands.
func LobbySystem(state *LobbySystemState) error {
	now := state.Timestamp().Unix()

	// Get lobby index
	var lobbyIndex component.LobbyIndexComponent
	var lobbyIndexEntityID ecs.EntityID
	for eid, idx := range state.LobbyIndexes.Iter() {
		lobbyIndex = idx.Index.Get()
		lobbyIndexEntityID = eid
		break
	}

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

	// Process CreateLobbyFromMatch commands (cross-shard from matchmaking)
	for cmd := range state.CreateLobbyFromMatchCmds.Iter() {
		payload := cmd.Payload()
		createLobbyFromMatch(state, &lobbyIndex, &partyIndex, payload.MatchID, payload.ProfileName, payload.Teams, now)
	}

	// Process CreateLobbyFromMatch events (same-shard from matchmaking)
	for event := range state.CreateLobbyFromMatchEvents.Iter() {
		createLobbyFromMatch(state, &lobbyIndex, &partyIndex, event.MatchID, event.ProfileName, event.Teams, now)
	}

	// Process create lobby commands
	for cmd := range state.CreateLobbyCmds.Iter() {
		payload := cmd.Payload()

		matchID := payload.MatchID
		if matchID == "" {
			matchID = uuid.New().String()
		}

		// Check if lobby already exists
		if _, exists := lobbyIndex.GetEntityID(matchID); exists {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				MatchID: matchID,
				Error:   "lobby already exists",
			})
			continue
		}

		// Create lobby
		eid, lobbyEntity := state.Lobbies.Create()
		lobby := component.LobbyComponent{
			MatchID:          matchID,
			HostPartyID:      payload.HostPartyID,
			Parties:          []string{payload.HostPartyID},
			Teams:            payload.Teams,
			State:            component.LobbyStateWaiting,
			MatchProfileName: payload.MatchProfileName,
			GameShardID:      config.GameShardID,
			Config:           payload.Config,
			MinPlayers:       payload.MinPlayers,
			MaxPlayers:       payload.MaxPlayers,
			CreatedAt:        now,
		}

		// If teams are provided, add all parties from teams
		if len(payload.Teams) > 0 {
			lobby.Parties = []string{}
			for _, team := range payload.Teams {
				lobby.Parties = append(lobby.Parties, team.PartyIDs...)
			}
		}

		lobbyEntity.Lobby.Set(lobby)

		// Update lobby index
		lobbyIndex.AddLobby(matchID, uint32(eid), component.LobbyStateWaiting)

		// Update party index and party lobby references
		for _, partyID := range lobby.Parties {
			partyIndex.SetPartyLobby(partyID, matchID)

			// Update party's LobbyID
			if partyEntityID, exists := partyIndex.GetEntityID(partyID); exists {
				if partyEntity, ok := state.Parties.GetByID(ecs.EntityID(partyEntityID)); ok {
					party := partyEntity.Party.Get()
					party.LobbyID = matchID
					partyEntity.Party.Set(party)
				}
			}
		}

		// Emit event
		state.LobbyCreatedEvents.Emit(LobbyCreatedEvent{
			MatchID:     matchID,
			HostPartyID: payload.HostPartyID,
		})

		state.Logger().Info().
			Str("match_id", matchID).
			Str("host", payload.HostPartyID).
			Int("parties", len(lobby.Parties)).
			Msg("Created lobby")
	}

	// Process join lobby commands
	for cmd := range state.JoinLobbyCmds.Iter() {
		payload := cmd.Payload()

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.MatchID)
		if !exists {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: payload.MatchID,
				Error:   "lobby not found",
			})
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Check if lobby can accept joins
		if !lobby.CanJoin() {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: payload.MatchID,
				Error:   "lobby is not accepting joins",
			})
			continue
		}

		// Check if party already in lobby
		if lobby.HasParty(payload.PartyID) {
			continue
		}

		// Check if party is in another lobby
		if existingLobbyID, _ := partyIndex.GetPartyByPlayer(payload.PartyID); existingLobbyID != "" {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: payload.MatchID,
				Error:   "party already in another lobby",
			})
			continue
		}

		// Add party to lobby
		lobby.AddParty(payload.PartyID)
		lobbyEntity.Lobby.Set(lobby)

		// Update indexes
		partyIndex.SetPartyLobby(payload.PartyID, payload.MatchID)

		// Update party's LobbyID
		if partyEntityID, exists := partyIndex.GetEntityID(payload.PartyID); exists {
			if partyEntity, ok := state.Parties.GetByID(ecs.EntityID(partyEntityID)); ok {
				party := partyEntity.Party.Get()
				party.LobbyID = payload.MatchID
				partyEntity.Party.Set(party)
			}
		}

		// Emit event
		state.PartyJoinedLobbyEvents.Emit(PartyJoinedLobbyEvent{
			MatchID: payload.MatchID,
			PartyID: payload.PartyID,
		})

		state.Logger().Debug().
			Str("match_id", payload.MatchID).
			Str("party", payload.PartyID).
			Msg("Party joined lobby")
	}

	// Process leave lobby commands
	for cmd := range state.LeaveLobbyCmds.Iter() {
		payload := cmd.Payload()

		// Get party's current lobby
		partyEntityID, exists := partyIndex.GetEntityID(payload.PartyID)
		if !exists {
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(partyEntityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()
		if party.LobbyID == "" {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(party.LobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Check if can leave
		if !lobby.CanLeave() {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: lobby.MatchID,
				Error:   "cannot leave lobby during game",
			})
			continue
		}

		matchID := lobby.MatchID

		// Remove party from lobby
		lobby.RemoveParty(payload.PartyID)
		lobbyEntity.Lobby.Set(lobby)

		// Update indexes
		partyIndex.SetPartyLobby(payload.PartyID, "")
		party.LobbyID = ""
		party.IsReady = false
		partyEntity.Party.Set(party)

		// Emit event
		state.PartyLeftLobbyEvents.Emit(PartyLeftLobbyEvent{
			MatchID: matchID,
			PartyID: payload.PartyID,
		})

		state.Logger().Debug().
			Str("match_id", matchID).
			Str("party", payload.PartyID).
			Msg("Party left lobby")

		// If lobby is empty, disband it
		if lobby.PartyCount() == 0 {
			lobbyIndex.RemoveLobby(matchID)
			state.Lobbies.Destroy(ecs.EntityID(lobbyEntityID))
			state.LobbyDisbandedEvents.Emit(LobbyDisbandedEvent{
				MatchID: matchID,
				Reason:  "all parties left",
			})
		} else if lobby.HostPartyID == payload.PartyID && len(lobby.Parties) > 0 {
			// If host left, assign new host
			lobby.HostPartyID = lobby.Parties[0]
			lobbyEntity.Lobby.Set(lobby)
		}
	}

	// Process set ready commands
	for cmd := range state.SetReadyCmds.Iter() {
		payload := cmd.Payload()

		partyEntityID, exists := partyIndex.GetEntityID(payload.PartyID)
		if !exists {
			continue
		}

		partyEntity, ok := state.Parties.GetByID(ecs.EntityID(partyEntityID))
		if !ok {
			continue
		}

		party := partyEntity.Party.Get()
		if party.LobbyID == "" {
			continue
		}

		lobbyEntityID, exists := lobbyIndex.GetEntityID(party.LobbyID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Can only set ready in waiting state
		if lobby.State != component.LobbyStateWaiting {
			continue
		}

		party.IsReady = payload.IsReady
		partyEntity.Party.Set(party)

		state.Logger().Debug().
			Str("match_id", lobby.MatchID).
			Str("party", payload.PartyID).
			Bool("ready", payload.IsReady).
			Msg("Party ready status changed")

		// Check if all parties are ready
		if payload.IsReady {
			allReady := true
			for _, partyID := range lobby.Parties {
				if peid, exists := partyIndex.GetEntityID(partyID); exists {
					if pe, ok := state.Parties.GetByID(ecs.EntityID(peid)); ok {
						if !pe.Party.Get().IsReady {
							allReady = false
							break
						}
					}
				}
			}

			if allReady {
				lobby.State = component.LobbyStateReady
				lobbyEntity.Lobby.Set(lobby)
				lobbyIndex.UpdateLobbyState(lobby.MatchID, component.LobbyStateWaiting, component.LobbyStateReady)

				state.LobbyReadyEvents.Emit(LobbyReadyEvent{
					MatchID: lobby.MatchID,
				})

				state.Logger().Info().
					Str("match_id", lobby.MatchID).
					Msg("All parties ready")
			}
		}
	}

	// Process start game commands
	for cmd := range state.StartGameCmds.Iter() {
		payload := cmd.Payload()

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.MatchID)
		if !exists {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: payload.MatchID,
				Error:   "lobby not found",
			})
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Check if party is host
		if !lobby.IsHost(payload.PartyID) {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: payload.MatchID,
				Error:   "only host can start game",
			})
			continue
		}

		// Check if can start
		if !lobby.CanStart() {
			state.LobbyErrorEvents.Emit(LobbyErrorEvent{
				PartyID: payload.PartyID,
				MatchID: payload.MatchID,
				Error:   "lobby is not ready to start",
			})
			continue
		}

		// Transition to in_game
		oldState := lobby.State
		lobby.State = component.LobbyStateInGame
		lobby.StartedAt = now
		lobbyEntity.Lobby.Set(lobby)

		lobbyIndex.UpdateLobbyState(lobby.MatchID, oldState, component.LobbyStateInGame)

		// Emit client-facing event
		state.GameStartedEvents.Emit(GameStartedEvent{
			MatchID:          lobby.MatchID,
			Teams:            lobby.Teams,
			MatchProfileName: lobby.MatchProfileName,
			Config:           lobby.Config,
		})

		// Notify game shard (same-shard or cross-shard)
		if config.GameShardID != "" {
			// Cross-shard: send command to game shard
			gameWorld := cardinal.OtherWorld{
				ShardID: config.GameShardID,
			}
			gameWorld.Send(&state.BaseSystemState, NotifyGameStartCommand{
				MatchID:          lobby.MatchID,
				Teams:            lobby.Teams,
				MatchProfileName: lobby.MatchProfileName,
				Config:           lobby.Config,
			})
			state.Logger().Debug().
				Str("match_id", lobby.MatchID).
				Str("game_shard", config.GameShardID).
				Msg("Sent NotifyGameStartCommand to game shard")
		} else {
			// Same-shard: emit system event for game system to receive
			state.NotifyGameStartEvents.Emit(NotifyGameStartEvent{
				MatchID:          lobby.MatchID,
				Teams:            lobby.Teams,
				MatchProfileName: lobby.MatchProfileName,
				Config:           lobby.Config,
			})
			state.Logger().Debug().
				Str("match_id", lobby.MatchID).
				Msg("Emitted NotifyGameStartEvent (same-shard)")
		}

		state.Logger().Info().
			Str("match_id", lobby.MatchID).
			Int("parties", lobby.PartyCount()).
			Msg("Game started")
	}

	// Process end game commands
	for cmd := range state.EndGameCmds.Iter() {
		payload := cmd.Payload()

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.MatchID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only end if in game
		if lobby.State != component.LobbyStateInGame {
			continue
		}

		oldState := lobby.State
		lobby.State = component.LobbyStateEnded
		lobbyEntity.Lobby.Set(lobby)

		lobbyIndex.UpdateLobbyState(lobby.MatchID, oldState, component.LobbyStateEnded)

		// Clear party lobby references
		for _, partyID := range lobby.Parties {
			partyIndex.SetPartyLobby(partyID, "")
			if peid, exists := partyIndex.GetEntityID(partyID); exists {
				if pe, ok := state.Parties.GetByID(ecs.EntityID(peid)); ok {
					p := pe.Party.Get()
					p.LobbyID = ""
					p.IsReady = false
					pe.Party.Set(p)
				}
			}
		}

		// Emit event
		state.GameEndedEvents.Emit(GameEndedEvent{
			MatchID: lobby.MatchID,
			Results: payload.Results,
		})

		state.Logger().Info().
			Str("match_id", lobby.MatchID).
			Msg("Game ended")

		// Remove lobby after game ends
		lobbyIndex.RemoveLobby(lobby.MatchID)
		state.Lobbies.Destroy(ecs.EntityID(lobbyEntityID))
	}

	// Process heartbeat commands
	for cmd := range state.HeartbeatCmds.Iter() {
		payload := cmd.Payload()

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.MatchID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()
		lobby.LastHeartbeat = now
		lobbyEntity.Lobby.Set(lobby)
	}

	// Check for stale lobbies (in_game without heartbeat)
	if config.HeartbeatTimeoutSeconds > 0 {
		for _, matchID := range lobbyIndex.InGameLobbies {
			lobbyEntityID, exists := lobbyIndex.GetEntityID(matchID)
			if !exists {
				continue
			}

			lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
			if !ok {
				continue
			}

			lobby := lobbyEntity.Lobby.Get()
			if lobby.LastHeartbeat > 0 && now-lobby.LastHeartbeat > config.HeartbeatTimeoutSeconds {
				// Lobby is stale, end it
				state.Logger().Warn().
					Str("match_id", matchID).
					Int64("last_heartbeat", lobby.LastHeartbeat).
					Msg("Lobby heartbeat timeout")

				// Emit end game event
				state.GameEndedEvents.Emit(GameEndedEvent{
					MatchID: matchID,
					Results: map[string]string{"reason": "heartbeat_timeout"},
				})

				// Clean up
				for _, partyID := range lobby.Parties {
					partyIndex.SetPartyLobby(partyID, "")
					if peid, exists := partyIndex.GetEntityID(partyID); exists {
						if pe, ok := state.Parties.GetByID(ecs.EntityID(peid)); ok {
							p := pe.Party.Get()
							p.LobbyID = ""
							p.IsReady = false
							pe.Party.Set(p)
						}
					}
				}

				lobbyIndex.RemoveLobby(matchID)
				state.Lobbies.Destroy(ecs.EntityID(lobbyEntityID))
			}
		}
	}

	// Save indexes back
	if lobbyIndexEntity, ok := state.LobbyIndexes.GetByID(lobbyIndexEntityID); ok {
		lobbyIndexEntity.Index.Set(lobbyIndex)
	}

	if partyIndexEntity, ok := state.PartyIndexes.GetByID(partyIndexEntityID); ok {
		partyIndexEntity.Index.Set(partyIndex)
	}

	return nil
}

// createLobbyFromMatch creates a lobby from a matchmaking result.
// This is called for both same-shard (SystemEvent) and cross-shard (Command) scenarios.
func createLobbyFromMatch(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	partyIndex *component.PartyIndexComponent,
	matchID string,
	profileName string,
	teams []LobbyTeamInfo,
	now int64,
) {
	// Check if lobby already exists
	if _, exists := lobbyIndex.GetEntityID(matchID); exists {
		state.LobbyErrorEvents.Emit(LobbyErrorEvent{
			MatchID: matchID,
			Error:   "lobby already exists",
		})
		return
	}

	// Convert LobbyTeamInfo to component.LobbyTeam
	componentTeams := make([]component.LobbyTeam, len(teams))
	var allParties []string
	var hostPartyID string

	for i, team := range teams {
		componentTeams[i] = component.LobbyTeam{
			TeamName: team.TeamName,
			PartyIDs: team.PartyIDs,
		}
		allParties = append(allParties, team.PartyIDs...)
		// First party in first team is the host
		if hostPartyID == "" && len(team.PartyIDs) > 0 {
			hostPartyID = team.PartyIDs[0]
		}
	}

	// Create lobby
	eid, lobbyEntity := state.Lobbies.Create()
	lobby := component.LobbyComponent{
		MatchID:          matchID,
		HostPartyID:      hostPartyID,
		Parties:          allParties,
		Teams:            componentTeams,
		State:            component.LobbyStateWaiting,
		MatchProfileName: profileName,
		CreatedAt:        now,
	}
	lobbyEntity.Lobby.Set(lobby)

	// Update lobby index
	lobbyIndex.AddLobby(matchID, uint32(eid), component.LobbyStateWaiting)

	// Update party index and party lobby references
	for _, partyID := range allParties {
		partyIndex.SetPartyLobby(partyID, matchID)

		// Update party's LobbyID if party entity exists
		if partyEntityID, exists := partyIndex.GetEntityID(partyID); exists {
			if partyEntity, ok := state.Parties.GetByID(ecs.EntityID(partyEntityID)); ok {
				party := partyEntity.Party.Get()
				party.LobbyID = matchID
				partyEntity.Party.Set(party)
			}
		}
	}

	// Emit event
	state.LobbyCreatedEvents.Emit(LobbyCreatedEvent{
		MatchID:     matchID,
		HostPartyID: hostPartyID,
	})

	state.Logger().Info().
		Str("match_id", matchID).
		Str("profile", profileName).
		Int("parties", len(allParties)).
		Int("teams", len(teams)).
		Msg("Created lobby from match")
}
