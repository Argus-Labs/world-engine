package lobby

import (
	"context"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/argus-labs/world-engine/pkg/lobby/types"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	lobbyv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/lobby/v1"
	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// LobbyService handles NATS service communication for the lobby shard.
type LobbyService struct {
	*micro.Service
	lb  *lobby
	tel *telemetry.Telemetry
}

// initService initializes the service endpoints for leader mode.
func (l *lobby) initService(client *micro.Client, address *microv1.ServiceAddress, tel *telemetry.Telemetry) error {
	service, err := micro.NewService(client, address, tel)
	if err != nil {
		return eris.Wrap(err, "failed to create service")
	}

	l.service = &LobbyService{
		Service: service,
		lb:      l,
		tel:     tel,
	}
	l.address = address

	// Register matchmaking endpoints (for receiving matches from Matchmaking Shard)
	matchmakingGroup := service.AddGroup("matchmaking")
	if err := matchmakingGroup.AddEndpoint("match", l.service.handleReceiveMatch); err != nil {
		return eris.Wrap(err, "failed to add matchmaking.match endpoint")
	}
	if err := matchmakingGroup.AddEndpoint("backfill-match", l.service.handleReceiveBackfillMatch); err != nil {
		return eris.Wrap(err, "failed to add matchmaking.backfill-match endpoint")
	}

	// Register command endpoints (for Game Shard - uses command.* pattern for consistency)
	commandGroup := service.AddGroup("command")
	if err := commandGroup.AddEndpoint("request-backfill", l.service.handleRequestBackfill); err != nil {
		return eris.Wrap(err, "failed to add command.request-backfill endpoint")
	}
	if err := commandGroup.AddEndpoint("cancel-backfill", l.service.handleCancelBackfill); err != nil {
		return eris.Wrap(err, "failed to add command.cancel-backfill endpoint")
	}

	// Register game shard endpoints (for Game Shard → Lobby communication)
	gameGroup := service.AddGroup("game")
	if err := gameGroup.AddEndpoint("heartbeat", l.service.handleGameHeartbeat); err != nil {
		return eris.Wrap(err, "failed to add game.heartbeat endpoint")
	}
	if err := gameGroup.AddEndpoint("player-status", l.service.handleGamePlayerStatus); err != nil {
		return eris.Wrap(err, "failed to add game.player-status endpoint")
	}
	if err := gameGroup.AddEndpoint("end-match", l.service.handleGameEndMatch); err != nil {
		return eris.Wrap(err, "failed to add game.end-match endpoint")
	}

	// Register query endpoints (for clients and other services)
	queryGroup := service.AddGroup("query")
	if err := queryGroup.AddEndpoint("party", l.service.handleGetParty); err != nil {
		return eris.Wrap(err, "failed to add query.party endpoint")
	}
	if err := queryGroup.AddEndpoint("party-by-player", l.service.handleGetPartyByPlayer); err != nil {
		return eris.Wrap(err, "failed to add query.party-by-player endpoint")
	}
	if err := queryGroup.AddEndpoint("lobby", l.service.handleGetLobby); err != nil {
		return eris.Wrap(err, "failed to add query.lobby endpoint")
	}
	if err := queryGroup.AddEndpoint("lobby-by-player", l.service.handleGetLobbyByPlayer); err != nil {
		return eris.Wrap(err, "failed to add query.lobby-by-player endpoint")
	}
	if err := queryGroup.AddEndpoint("list-lobbies", l.service.handleListLobbies); err != nil {
		return eris.Wrap(err, "failed to add query.list-lobbies endpoint")
	}
	if err := queryGroup.AddEndpoint("stats", l.service.handleStats); err != nil {
		return eris.Wrap(err, "failed to add query.stats endpoint")
	}

	return nil
}

// Close closes the service.
func (s *LobbyService) Close() error {
	return s.Service.Close()
}

// SendBackfillRequest sends a backfill request to the Matchmaking Shard via command queue.
// This is fire-and-forget. Response comes via callback to <lobby_address>.matchmaking.backfill-match.
func (s *LobbyService) SendBackfillRequest(
	ctx context.Context,
	matchmakingAddr *microv1.ServiceAddress,
	matchID string,
	matchProfileName string,
	teamName string,
	slotsNeeded []*matchmakingv1.SlotNeeded,
) error {
	// Send as command via Request (returns after enqueue, actual processing in tick)
	// Matchmaking will send backfill-match callback when found
	req := &matchmakingv1.CreateBackfillRequest{
		MatchId:          matchID,
		MatchProfileName: matchProfileName,
		TeamName:         teamName,
		SlotsNeeded:      slotsNeeded,
		LobbyAddress:     s.lb.address,
	}

	// Call backfill endpoint (inter-shard communication, not signed command queue)
	_, err := s.NATS().Request(ctx, matchmakingAddr, "backfill.create", req)
	if err != nil {
		return eris.Wrap(err, "failed to send backfill command")
	}

	s.tel.Logger.Debug().
		Str("match_id", matchID).
		Str("team_name", teamName).
		Msg("Backfill command sent to matchmaking")

	return nil
}

// CancelBackfillRequest cancels a pending backfill request via command queue.
// This is fire-and-forget - no callback expected.
func (s *LobbyService) CancelBackfillRequest(
	ctx context.Context,
	matchmakingAddr *microv1.ServiceAddress,
	backfillRequestID string,
) error {
	// Send as command via Request (returns after enqueue)
	req := &matchmakingv1.CancelBackfillRequest{
		BackfillRequestId: backfillRequestID,
		LobbyAddress:      s.lb.address,
	}

	// Call backfill endpoint (inter-shard communication, not signed command queue)
	_, err := s.NATS().Request(ctx, matchmakingAddr, "backfill.cancel", req)
	if err != nil {
		return eris.Wrap(err, "failed to send cancel backfill command")
	}

	s.tel.Logger.Debug().
		Str("backfill_request_id", backfillRequestID).
		Msg("Cancel backfill command sent to matchmaking")

	return nil
}

// handleReceiveMatch handles incoming Match from Matchmaking Shard.
// This handler queues an internal command for deterministic processing in Tick().
func (s *LobbyService) handleReceiveMatch(_ context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Received match from matchmaking")

	// Parse the match protobuf from the payload
	var matchPb matchmakingv1.Match
	if err := req.Payload.UnmarshalTo(&matchPb); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal match")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	// Convert to internal command format
	teams := make([]MatchTeam, len(matchPb.Teams))
	for i, teamPb := range matchPb.Teams {
		tickets := make([]MatchTicket, len(teamPb.Tickets))
		for j, ticket := range teamPb.Tickets {
			tickets[j] = MatchTicket{
				ID:        ticket.Id,
				PlayerIDs: ticket.PlayerIds,
			}
		}
		teams[i] = MatchTeam{
			Name:    teamPb.Name,
			Tickets: tickets,
		}
	}

	// Convert config
	var config map[string]any
	if matchPb.Config != nil {
		config = matchPb.Config.AsMap()
	}

	// Convert addresses
	var matchmakingAddr, targetAddr *ServiceAddressJSON
	if matchPb.MatchmakingAddress != nil {
		realm := "world"
		if matchPb.MatchmakingAddress.Realm == microv1.ServiceAddress_REALM_INTERNAL {
			realm = "internal"
		}
		matchmakingAddr = &ServiceAddressJSON{
			Region:       matchPb.MatchmakingAddress.Region,
			Realm:        realm,
			Organization: matchPb.MatchmakingAddress.Organization,
			Project:      matchPb.MatchmakingAddress.Project,
			ServiceID:    matchPb.MatchmakingAddress.ServiceId,
		}
	}
	if matchPb.TargetAddress != nil {
		realm := "world"
		if matchPb.TargetAddress.Realm == microv1.ServiceAddress_REALM_INTERNAL {
			realm = "internal"
		}
		targetAddr = &ServiceAddressJSON{
			Region:       matchPb.TargetAddress.Region,
			Realm:        realm,
			Organization: matchPb.TargetAddress.Organization,
			Project:      matchPb.TargetAddress.Project,
			ServiceID:    matchPb.TargetAddress.ServiceId,
		}
	}

	// Queue internal command for processing in Tick()
	cmd := ReceiveMatchInternalCommand{
		MatchID:            matchPb.Id,
		MatchProfileName:   matchPb.MatchProfileName,
		Teams:              teams,
		Config:             config,
		MatchmakingAddress: matchmakingAddr,
		TargetAddress:      targetAddr,
	}
	s.lb.EnqueueInternalCommand(cmd)

	s.tel.Logger.Debug().
		Str("match_id", matchPb.Id).
		Str("profile", matchPb.MatchProfileName).
		Msg("Match queued for processing")

	return micro.NewSuccessResponse(req, nil)
}

// handleReceiveBackfillMatch handles incoming BackfillMatch from Matchmaking Shard.
// This handler queues an internal command for deterministic processing in Tick().
func (s *LobbyService) handleReceiveBackfillMatch(_ context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Received backfill match from matchmaking")

	// Parse the backfill match protobuf from the payload
	var backfillPb matchmakingv1.BackfillMatch
	if err := req.Payload.UnmarshalTo(&backfillPb); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal backfill match")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	// Convert to internal command format
	tickets := make([]MatchTicket, len(backfillPb.Tickets))
	for i, ticket := range backfillPb.Tickets {
		tickets[i] = MatchTicket{
			ID:        ticket.Id,
			PlayerIDs: ticket.PlayerIds,
		}
	}

	// Queue internal command for processing in Tick()
	cmd := ReceiveBackfillMatchInternalCommand{
		BackfillRequestID: backfillPb.BackfillRequestId,
		MatchID:           backfillPb.MatchId,
		TeamName:          backfillPb.TeamName,
		Tickets:           tickets,
	}
	s.lb.EnqueueInternalCommand(cmd)

	s.tel.Logger.Debug().
		Str("match_id", backfillPb.MatchId).
		Str("team_name", backfillPb.TeamName).
		Int("ticket_count", len(tickets)).
		Msg("Backfill match queued for processing")

	// Note: Game Shard notification will happen after Tick() processes the command
	// This is a trade-off for determinism - we acknowledge the request but actual
	// processing happens in the next tick.

	return micro.NewSuccessResponse(req, nil)
}

// Backfill handlers - for Game Shard to request/cancel backfill

// handleRequestBackfill handles backfill requests from Game Shard.
// This forwards the request to Matchmaking as a command (fire-and-forget).
// Result comes back via matchmaking.backfill-match callback.
func (s *LobbyService) handleRequestBackfill(ctx context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Received backfill request from game shard")

	var backfillReq lobbyv1.RequestBackfillRequest
	if err := req.Payload.UnmarshalTo(&backfillReq); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal backfill request")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	s.lb.mu.RLock()
	lobby, ok := s.lb.lobbies.Get(backfillReq.MatchId)
	s.lb.mu.RUnlock()

	if !ok {
		err := eris.Errorf("lobby for match %s not found", backfillReq.MatchId)
		s.tel.Logger.Error().Err(err).Msg("Backfill request for unknown lobby")
		return micro.NewErrorResponse(req, err, 5) // NOT_FOUND
	}

	if lobby.State != types.LobbyStateInGame {
		err := eris.Errorf("lobby %s is not in game (state: %s)", lobby.MatchID, lobby.State)
		s.tel.Logger.Error().Err(err).Msg("Backfill request for non-active lobby")
		return micro.NewErrorResponse(req, err, 9) // FAILED_PRECONDITION
	}

	if lobby.MatchmakingAddress == nil {
		err := eris.New("lobby has no matchmaking address (not a matchmade lobby)")
		s.tel.Logger.Error().Err(err).Msg("Backfill request for non-matchmade lobby")
		return micro.NewErrorResponse(req, err, 9) // FAILED_PRECONDITION
	}

	// Convert lobbyv1.BackfillSlotNeeded to matchmakingv1.SlotNeeded
	slotsNeeded := make([]*matchmakingv1.SlotNeeded, len(backfillReq.SlotsNeeded))
	for i, slot := range backfillReq.SlotsNeeded {
		slotsNeeded[i] = &matchmakingv1.SlotNeeded{
			PoolName: slot.PoolName,
			Count:    slot.Count,
		}
	}

	// Forward to Matchmaking Shard as command
	// Result will come back via matchmaking.backfill-match callback
	if err := s.SendBackfillRequest(
		ctx,
		lobby.MatchmakingAddress,
		backfillReq.MatchId,
		backfillReq.ProfileName,
		backfillReq.TeamName,
		slotsNeeded,
	); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to forward backfill request to matchmaking")
		return micro.NewErrorResponse(req, err, 13) // INTERNAL
	}

	s.tel.Logger.Info().
		Str("match_id", backfillReq.MatchId).
		Str("team_name", backfillReq.TeamName).
		Msg("Backfill request sent to matchmaking (async)")

	// Return acknowledgment - actual backfill result comes via callback
	return micro.NewSuccessResponse(req, nil)
}

// handleCancelBackfill handles backfill cancellation from Game Shard.
// This forwards the cancellation to Matchmaking as a command (fire-and-forget).
func (s *LobbyService) handleCancelBackfill(ctx context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Received cancel backfill request from game shard")

	var cancelReq lobbyv1.CancelBackfillRequest
	if err := req.Payload.UnmarshalTo(&cancelReq); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal cancel backfill request")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	// We need to know which matchmaking shard to forward to.
	// For now, we'll look up by iterating lobbies (could be optimized with an index).

	s.lb.mu.RLock()
	var matchmakingAddr *microv1.ServiceAddress
	lobbies := s.lb.lobbies.All()
	for _, lobby := range lobbies {
		if lobby.MatchmakingAddress != nil {
			matchmakingAddr = lobby.MatchmakingAddress
			break
		}
	}
	s.lb.mu.RUnlock()

	if matchmakingAddr == nil {
		err := eris.New("no matchmaking address found")
		s.tel.Logger.Error().Err(err).Msg("Cannot cancel backfill - no matchmaking address")
		return micro.NewErrorResponse(req, err, 9) // FAILED_PRECONDITION
	}

	// Forward to Matchmaking Shard as command
	if err := s.CancelBackfillRequest(ctx, matchmakingAddr, cancelReq.BackfillRequestId); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to forward cancel backfill to matchmaking")
		return micro.NewErrorResponse(req, err, 13) // INTERNAL
	}

	s.tel.Logger.Info().
		Str("backfill_request_id", cancelReq.BackfillRequestId).
		Msg("Backfill cancellation sent to matchmaking (async)")

	// Return acknowledgment
	return micro.NewSuccessResponse(req, nil)
}

// Query handlers - these return nil payload since we don't have lobby-specific protobuf definitions yet
// In production, we would define proper protobuf messages for these responses

// handleGetParty returns a party by ID.
func (s *LobbyService) handleGetParty(_ context.Context, req *micro.Request) *micro.Response {
	// For now, these query endpoints just return success with nil payload
	// In production, we would parse the request and return proper protobuf responses
	s.tel.Logger.Debug().Msg("Query: get party")
	return micro.NewSuccessResponse(req, nil)
}

// handleGetPartyByPlayer returns the party for a given player.
func (s *LobbyService) handleGetPartyByPlayer(_ context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Query: get party by player")
	return micro.NewSuccessResponse(req, nil)
}

// handleGetLobby returns a lobby by ID.
func (s *LobbyService) handleGetLobby(_ context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Query: get lobby")
	return micro.NewSuccessResponse(req, nil)
}

// handleGetLobbyByPlayer returns the lobby for a given player.
func (s *LobbyService) handleGetLobbyByPlayer(_ context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Query: get lobby by player")
	return micro.NewSuccessResponse(req, nil)
}

// handleListLobbies returns lobbies in waiting state (for browsing).
func (s *LobbyService) handleListLobbies(_ context.Context, req *micro.Request) *micro.Response {
	s.tel.Logger.Debug().Msg("Query: list lobbies")
	return micro.NewSuccessResponse(req, nil)
}

// handleStats returns shard statistics.
func (s *LobbyService) handleStats(_ context.Context, req *micro.Request) *micro.Response {
	s.lb.mu.RLock()
	defer s.lb.mu.RUnlock()

	s.tel.Logger.Debug().
		Int("party_count", s.lb.parties.Count()).
		Int("lobby_count", s.lb.lobbies.Count()).
		Msg("Query: stats")

	return micro.NewSuccessResponse(req, nil)
}

// =============================================================================
// Lobby -> Game Shard Communication
// =============================================================================

// SendStartGame sends the start-game message to the Game Shard (Q1 - Gameplay shard handoff).
func (s *LobbyService) SendStartGame(ctx context.Context, lobby *types.Lobby) error {
	if lobby.TargetAddress == nil {
		return eris.New("lobby has no target address (Game Shard)")
	}

	// Build team info with player IDs
	teams := make([]*lobbyv1.TeamInfo, len(lobby.Teams))
	for i, team := range lobby.Teams {
		// Collect all player IDs from parties in this team
		var playerIDs []string
		for _, partyID := range team.PartyIDs {
			party, ok := s.lb.parties.Get(partyID)
			if ok {
				playerIDs = append(playerIDs, party.Members...)
			}
		}
		teams[i] = &lobbyv1.TeamInfo{
			Name:      team.Name,
			PlayerIds: playerIDs,
		}
	}

	// Convert config to protobuf Struct
	var configPb *structpb.Struct
	if lobby.Config != nil {
		var err error
		configPb, err = structpb.NewStruct(lobby.Config)
		if err != nil {
			s.tel.Logger.Warn().Err(err).Msg("Failed to convert lobby config to protobuf")
		}
	}

	req := &lobbyv1.StartGameRequest{
		MatchId:      lobby.MatchID,
		ProfileName:  lobby.MatchProfileName,
		Teams:        teams,
		Config:       configPb,
		LobbyAddress: s.lb.address,
	}

	_, err := s.NATS().Request(ctx, lobby.TargetAddress, "lobby.game-start", req)
	if err != nil {
		return eris.Wrap(err, "failed to send start-game to Game Shard")
	}

	s.tel.Logger.Info().
		Str("match_id", lobby.MatchID).
		Msg("Start-game sent to Game Shard")

	return nil
}

// SendBackfillNotification sends backfill notification to the Game Shard (Q3).
func (s *LobbyService) SendBackfillNotification(
	ctx context.Context,
	lobby *types.Lobby,
	backfillRequestID string,
	teamName string,
	playerIDs []string,
) error {
	if lobby.TargetAddress == nil {
		return eris.New("lobby has no target address (Game Shard)")
	}

	req := &lobbyv1.BackfillNotification{
		BackfillRequestId: backfillRequestID,
		MatchId:           lobby.MatchID,
		TeamName:          teamName,
		PlayerIds:         playerIDs,
		LobbyAddress:      s.lb.address,
	}

	_, err := s.NATS().Request(ctx, lobby.TargetAddress, "lobby.backfill-match", req)
	if err != nil {
		return eris.Wrap(err, "failed to send backfill notification to Game Shard")
	}

	s.tel.Logger.Info().
		Str("match_id", lobby.MatchID).
		Str("backfill_request_id", backfillRequestID).
		Str("team_name", teamName).
		Int("player_count", len(playerIDs)).
		Msg("Backfill notification sent to Game Shard")

	return nil
}

// =============================================================================
// Game Shard -> Lobby Handlers (Q4 & Q5)
// =============================================================================

// handleGameHeartbeat handles heartbeat from Game Shard.
// This handler queues an internal command for deterministic processing in Tick().
func (s *LobbyService) handleGameHeartbeat(_ context.Context, req *micro.Request) *micro.Response {
	var heartbeatReq lobbyv1.HeartbeatRequest
	if err := req.Payload.UnmarshalTo(&heartbeatReq); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal heartbeat request")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	// Queue internal command for processing in Tick()
	cmd := GameHeartbeatInternalCommand{
		MatchID: heartbeatReq.MatchId,
	}
	s.lb.EnqueueInternalCommand(cmd)

	s.tel.Logger.Debug().
		Str("match_id", heartbeatReq.MatchId).
		Msg("Heartbeat queued for processing")

	return micro.NewSuccessResponse(req, &lobbyv1.HeartbeatResponse{Success: true})
}

// handleGamePlayerStatus handles player status updates from Game Shard (Q5).
// This handler queues an internal command for deterministic processing in Tick().
func (s *LobbyService) handleGamePlayerStatus(_ context.Context, req *micro.Request) *micro.Response {
	var statusReq lobbyv1.PlayerStatusRequest
	if err := req.Payload.UnmarshalTo(&statusReq); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal player status request")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	// Determine connected status from player status enum
	connected := statusReq.Status == lobbyv1.PlayerStatus_PLAYER_STATUS_CONNECTED ||
		statusReq.Status == lobbyv1.PlayerStatus_PLAYER_STATUS_RECONNECTED

	// Queue internal command for processing in Tick()
	cmd := GamePlayerStatusInternalCommand{
		MatchID:   statusReq.MatchId,
		PlayerID:  statusReq.PlayerId,
		Connected: connected,
	}
	s.lb.EnqueueInternalCommand(cmd)

	s.tel.Logger.Debug().
		Str("match_id", statusReq.MatchId).
		Str("player_id", statusReq.PlayerId).
		Str("status", statusReq.Status.String()).
		Msg("Player status queued for processing")

	return micro.NewSuccessResponse(req, &lobbyv1.PlayerStatusResponse{Success: true})
}

// handleGameEndMatch handles end-match from Game Shard.
// This handler queues an internal command for deterministic processing in Tick().
func (s *LobbyService) handleGameEndMatch(_ context.Context, req *micro.Request) *micro.Response {
	var endReq lobbyv1.EndMatchRequest
	if err := req.Payload.UnmarshalTo(&endReq); err != nil {
		s.tel.Logger.Error().Err(err).Msg("Failed to unmarshal end-match request")
		return micro.NewErrorResponse(req, err, 3) // INVALID_ARGUMENT
	}

	// Queue internal command for processing in Tick()
	cmd := GameEndMatchInternalCommand{
		MatchID: endReq.MatchId,
		Result:  endReq.Result.String(),
	}
	s.lb.EnqueueInternalCommand(cmd)

	s.tel.Logger.Debug().
		Str("match_id", endReq.MatchId).
		Str("result", endReq.Result.String()).
		Msg("End match queued for processing")

	return micro.NewSuccessResponse(req, &lobbyv1.EndMatchResponse{Success: true})
}
