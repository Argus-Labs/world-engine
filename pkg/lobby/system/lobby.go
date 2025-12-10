package system

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/lobby/component"
)

// -----------------------------------------------------------------------------
// Lobby Commands
// -----------------------------------------------------------------------------

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

// LobbyCreatedEvent is emitted when a lobby is created from matchmaking.
type LobbyCreatedEvent struct {
	cardinal.BaseEvent
	MatchID          string `json:"match_id"`
	MatchProfileName string `json:"match_profile_name"`
}

// Name returns the event name.
func (LobbyCreatedEvent) Name() string { return "lobby_created" }

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

// LobbyErrorEvent is emitted when a lobby operation fails.
type LobbyErrorEvent struct {
	cardinal.BaseEvent
	MatchID string `json:"match_id,omitempty"`
	Error   string `json:"error"`
}

// Name returns the event name.
func (LobbyErrorEvent) Name() string { return "lobby_error" }

// PlayerDisconnectedEvent is emitted when a player is marked as disconnected.
type PlayerDisconnectedEvent struct {
	cardinal.BaseEvent
	MatchID  string `json:"match_id"`
	PartyID  string `json:"party_id"`
	TeamName string `json:"team_name"`
}

// Name returns the event name.
func (PlayerDisconnectedEvent) Name() string { return "lobby_player_disconnected" }

// -----------------------------------------------------------------------------
// Cross-Shard Communication Types
// -----------------------------------------------------------------------------

// LobbyTeamInfo represents a team in lobby info (used for cross-shard communication).
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

// NotifyGameEndCommand is received from game shard when game ends (cross-shard).
type NotifyGameEndCommand struct {
	cardinal.BaseCommand
	MatchID string            `json:"match_id"`
	Results map[string]string `json:"results,omitempty"`
}

// Name returns the command name.
func (NotifyGameEndCommand) Name() string { return "game_notify_lobby_end" }

// PlayerDisconnectedCommand is received from game shard when a player disconnects.
// This marks the party as disconnected in the lobby (state tracking only).
// Game shard is responsible for deciding whether to request backfill from matchmaking.
type PlayerDisconnectedCommand struct {
	cardinal.BaseCommand
	MatchID  string `json:"match_id"`
	PartyID  string `json:"party_id"`
	TeamName string `json:"team_name"`
}

// Name returns the command name.
func (PlayerDisconnectedCommand) Name() string { return "game_player_disconnected" }

// -----------------------------------------------------------------------------
// Lobby System State
// -----------------------------------------------------------------------------

// LobbySystemState is the state for the lobby system.
type LobbySystemState struct {
	cardinal.BaseSystemState

	// Commands
	EndGameCmds   cardinal.WithCommand[EndGameCommand]
	HeartbeatCmds cardinal.WithCommand[HeartbeatCommand]

	// Entities
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[component.LobbyComponent]
	}]

	LobbyIndexes cardinal.Contains[struct {
		Index cardinal.Ref[component.LobbyIndexComponent]
	}]

	Configs cardinal.Contains[struct {
		Config cardinal.Ref[component.ConfigComponent]
	}]

	// Events (client-facing)
	LobbyCreatedEvents cardinal.WithEvent[LobbyCreatedEvent]
	GameStartedEvents  cardinal.WithEvent[GameStartedEvent]
	GameEndedEvents    cardinal.WithEvent[GameEndedEvent]
	LobbyErrorEvents   cardinal.WithEvent[LobbyErrorEvent]

	// Cross-shard Commands (from matchmaking shard)
	CreateLobbyFromMatchCmds cardinal.WithCommand[CreateLobbyFromMatchCommand]

	// Cross-shard Commands (from game shard)
	NotifyGameEndCmds      cardinal.WithCommand[NotifyGameEndCommand]
	PlayerDisconnectedCmds cardinal.WithCommand[PlayerDisconnectedCommand]

	// Events (client-facing) - disconnect
	PlayerDisconnectedEvents cardinal.WithEvent[PlayerDisconnectedEvent]

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

	// Get config
	var config component.ConfigComponent
	for _, cfg := range state.Configs.Iter() {
		config = cfg.Config.Get()
		break
	}

	// Process CreateLobbyFromMatch commands (cross-shard from matchmaking)
	for cmd := range state.CreateLobbyFromMatchCmds.Iter() {
		payload := cmd.Payload()
		state.Logger().Info().
			Str("match_id", payload.MatchID).
			Str("profile", payload.ProfileName).
			Int("teams", len(payload.Teams)).
			Msg("[CROSS-SHARD] Received CreateLobbyFromMatch command from matchmaking shard")
		createLobbyFromMatch(state, &lobbyIndex, &config, payload.MatchID, payload.ProfileName, payload.Teams, now)
	}

	// Process CreateLobbyFromMatch events (same-shard from matchmaking)
	for event := range state.CreateLobbyFromMatchEvents.Iter() {
		createLobbyFromMatch(state, &lobbyIndex, &config, event.MatchID, event.ProfileName, event.Teams, now)
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

		lobby.State = component.LobbyStateEnded
		lobbyEntity.Lobby.Set(lobby)

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

	// Process NotifyGameEnd commands (cross-shard from game shard)
	for cmd := range state.NotifyGameEndCmds.Iter() {
		payload := cmd.Payload()
		state.Logger().Info().
			Str("match_id", payload.MatchID).
			Msg("[CROSS-SHARD] Received NotifyGameEnd command from game shard")

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.MatchID)
		if !exists {
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()
		if lobby.State != component.LobbyStateInGame {
			continue
		}

		lobby.State = component.LobbyStateEnded
		lobbyEntity.Lobby.Set(lobby)

		state.Logger().Info().
			Str("match_id", lobby.MatchID).
			Msg("Lobby state transition: game ended")

		state.GameEndedEvents.Emit(GameEndedEvent{
			MatchID: lobby.MatchID,
			Results: payload.Results,
		})

		lobbyIndex.RemoveLobby(lobby.MatchID)
		state.Lobbies.Destroy(ecs.EntityID(lobbyEntityID))
	}

	// Process PlayerDisconnected commands (cross-shard from game shard)
	for cmd := range state.PlayerDisconnectedCmds.Iter() {
		payload := cmd.Payload()
		state.Logger().Info().
			Str("match_id", payload.MatchID).
			Str("party_id", payload.PartyID).
			Str("team_name", payload.TeamName).
			Msg("[CROSS-SHARD] Received PlayerDisconnected command from game shard")

		lobbyEntityID, exists := lobbyIndex.GetEntityID(payload.MatchID)
		if !exists {
			state.Logger().Warn().
				Str("match_id", payload.MatchID).
				Msg("PlayerDisconnected: lobby not found")
			continue
		}

		lobbyEntity, ok := state.Lobbies.GetByID(ecs.EntityID(lobbyEntityID))
		if !ok {
			continue
		}

		lobby := lobbyEntity.Lobby.Get()

		// Only process if lobby is in_game
		if lobby.State != component.LobbyStateInGame {
			state.Logger().Warn().
				Str("match_id", payload.MatchID).
				Str("state", string(lobby.State)).
				Msg("PlayerDisconnected: lobby not in_game")
			continue
		}

		// Mark party as disconnected
		lobby.MarkDisconnected(payload.PartyID)
		lobbyEntity.Lobby.Set(lobby)

		// Emit event - game shard can listen for this and decide whether to request backfill
		state.PlayerDisconnectedEvents.Emit(PlayerDisconnectedEvent{
			MatchID:  payload.MatchID,
			PartyID:  payload.PartyID,
			TeamName: payload.TeamName,
		})

		state.Logger().Info().
			Str("match_id", payload.MatchID).
			Str("party_id", payload.PartyID).
			Msg("Party marked as disconnected - game shard should request backfill if needed")
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

				lobbyIndex.RemoveLobby(matchID)
				state.Lobbies.Destroy(ecs.EntityID(lobbyEntityID))
			}
		}
	}

	// Save lobby index
	if lobbyIndexEntity, ok := state.LobbyIndexes.GetByID(lobbyIndexEntityID); ok {
		lobbyIndexEntity.Index.Set(lobbyIndex)
	}

	return nil
}

// createLobbyFromMatch creates a lobby from a matchmaking result.
// This is called for both same-shard (SystemEvent) and cross-shard (Command) scenarios.
// Lobby immediately transitions to in_game state.
func createLobbyFromMatch(
	state *LobbySystemState,
	lobbyIndex *component.LobbyIndexComponent,
	config *component.ConfigComponent,
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

	for i, team := range teams {
		componentTeams[i] = component.LobbyTeam{
			TeamName: team.TeamName,
			PartyIDs: team.PartyIDs,
		}
		allParties = append(allParties, team.PartyIDs...)
	}

	// Create lobby - immediately in_game state
	eid, lobbyEntity := state.Lobbies.Create()
	lobby := component.LobbyComponent{
		MatchID:          matchID,
		Parties:          allParties,
		Teams:            componentTeams,
		State:            component.LobbyStateInGame,
		MatchProfileName: profileName,
		CreatedAt:        now,
		StartedAt:        now,
		LastHeartbeat:    now,
	}
	lobbyEntity.Lobby.Set(lobby)

	// Update lobby index
	lobbyIndex.AddLobby(matchID, uint32(eid))

	// Emit lobby created event
	state.LobbyCreatedEvents.Emit(LobbyCreatedEvent{
		MatchID:          matchID,
		MatchProfileName: profileName,
	})

	state.Logger().Info().
		Str("match_id", matchID).
		Str("profile", profileName).
		Int("parties", len(allParties)).
		Int("teams", len(teams)).
		Str("state", "in_game").
		Msg("Lobby created from match - game started")

	// Log team distribution for determinism verification
	for i, team := range componentTeams {
		state.Logger().Info().
			Str("match_id", matchID).
			Int("team_index", i).
			Str("team_name", team.TeamName).
			Strs("party_ids", team.PartyIDs).
			Msg("Team distribution")
	}

	// Emit game started event
	state.GameStartedEvents.Emit(GameStartedEvent{
		MatchID:          matchID,
		Teams:            componentTeams,
		MatchProfileName: profileName,
	})

	// Notify game shard (same-shard or cross-shard)
	if config.GameShardID != "" {
		// Cross-shard: send command to game shard
		gameWorld := cardinal.OtherWorld{
			Region:       config.GameRegion,
			Organization: config.GameOrganization,
			Project:      config.GameProject,
			ShardID:      config.GameShardID,
		}
		gameWorld.Send(&state.BaseSystemState, NotifyGameStartCommand{
			MatchID:          matchID,
			Teams:            componentTeams,
			MatchProfileName: profileName,
		})
		state.Logger().Info().
			Str("match_id", matchID).
			Str("game_shard", config.GameShardID).
			Msg("[CROSS-SHARD] Sent NotifyGameStartCommand to game shard")
	} else {
		// Same-shard: emit system event for game system to receive
		state.NotifyGameStartEvents.Emit(NotifyGameStartEvent{
			MatchID:          matchID,
			Teams:            componentTeams,
			MatchProfileName: profileName,
		})
		state.Logger().Info().
			Str("match_id", matchID).
			Msg("Emitted NotifyGameStartEvent (same-shard)")
	}
}
