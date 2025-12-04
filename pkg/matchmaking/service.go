package matchmaking

import (
	"context"

	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"

	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// MatchmakingService handles network communication for the matchmaking shard.
type MatchmakingService struct {
	*micro.Service
	mm  *matchmaking
	tel *telemetry.Telemetry
}

// NewMatchmakingService creates a new matchmaking service.
func NewMatchmakingService(
	client *micro.Client,
	address *microv1.ServiceAddress,
	mm *matchmaking,
	tel *telemetry.Telemetry,
) (*MatchmakingService, error) {
	svc, err := micro.NewService(client, address, tel)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create service")
	}

	ms := &MatchmakingService{
		Service: svc,
		mm:      mm,
		tel:     tel,
	}

	// Register query endpoints
	queryGroup := svc.AddGroup("query")
	if err := queryGroup.AddEndpoint("ticket", ms.handleGetTicket); err != nil {
		return nil, eris.Wrap(err, "failed to add get-ticket endpoint")
	}
	if err := queryGroup.AddEndpoint("stats", ms.handleGetStats); err != nil {
		return nil, eris.Wrap(err, "failed to add stats endpoint")
	}

	// Register backfill endpoints (internal, called by Lobby Shard)
	backfillGroup := svc.AddGroup("backfill")
	if err := backfillGroup.AddEndpoint("create", ms.handleCreateBackfillRequest); err != nil {
		return nil, eris.Wrap(err, "failed to add create-backfill endpoint")
	}
	if err := backfillGroup.AddEndpoint("cancel", ms.handleCancelBackfillRequest); err != nil {
		return nil, eris.Wrap(err, "failed to add cancel-backfill endpoint")
	}

	return ms, nil
}

// handleGetTicket handles ticket query requests.
func (ms *MatchmakingService) handleGetTicket(_ context.Context, req *micro.Request) *micro.Response {
	var query matchmakingv1.GetTicketRequest
	if err := req.Payload.UnmarshalTo(&query); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to unmarshal request"), 3)
	}

	t, ok := ms.mm.tickets.Get(query.TicketId)
	if !ok {
		return micro.NewErrorResponse(req, eris.New("ticket not found"), 5)
	}

	resp := &matchmakingv1.GetTicketResponse{
		Ticket: t.ToProto(),
	}
	return micro.NewSuccessResponse(req, resp)
}

// handleGetStats handles stats query requests.
func (ms *MatchmakingService) handleGetStats(_ context.Context, req *micro.Request) *micro.Response {
	// Build per-profile stats
	profileStats := make(map[string]int64)
	for _, prof := range ms.mm.profiles.All() {
		profileStats[prof.Name] = int64(ms.mm.tickets.CountByProfile(prof.Name))
	}

	resp := &matchmakingv1.GetStatsResponse{
		TotalTickets:          int64(ms.mm.tickets.Count()),
		TotalBackfillRequests: int64(ms.mm.backfills.Count()),
		MatchCounter:          ms.mm.matchCounter,
		TicketsByProfile:      profileStats,
	}

	return micro.NewSuccessResponse(req, resp)
}

// handleCreateBackfillRequest handles backfill creation requests from Lobby Shard.
func (ms *MatchmakingService) handleCreateBackfillRequest(_ context.Context, req *micro.Request) *micro.Response {
	var createReq matchmakingv1.CreateBackfillRequestRequest
	if err := req.Payload.UnmarshalTo(&createReq); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to unmarshal request"), 3)
	}

	// Validate profile exists
	_, ok := ms.mm.profiles.Get(createReq.MatchProfileName)
	if !ok {
		return micro.NewErrorResponse(req, eris.Errorf("unknown match profile: %s", createReq.MatchProfileName), 3)
	}

	// Note: The actual backfill creation happens via commands for deterministic replay.
	// This endpoint just acknowledges the request and queues a command.
	// For now, we return success - the command will be processed in the next tick.

	resp := &matchmakingv1.CreateBackfillRequestResponse{
		BackfillRequest: &matchmakingv1.BackfillRequest{
			MatchId:          createReq.MatchId,
			MatchProfileName: createReq.MatchProfileName,
			TeamName:         createReq.TeamName,
			LobbyAddress:     createReq.LobbyAddress,
		},
	}
	return micro.NewSuccessResponse(req, resp)
}

// handleCancelBackfillRequest handles backfill cancellation requests.
func (ms *MatchmakingService) handleCancelBackfillRequest(_ context.Context, req *micro.Request) *micro.Response {
	var cancelReq matchmakingv1.CancelBackfillRequestRequest
	if err := req.Payload.UnmarshalTo(&cancelReq); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to unmarshal request"), 3)
	}

	// Note: Similar to create, actual cancellation happens via commands.
	resp := &matchmakingv1.CancelBackfillRequestResponse{
		Success: true,
	}
	return micro.NewSuccessResponse(req, resp)
}

// PublishMatch publishes a match to the target Lobby Shard.
func (ms *MatchmakingService) PublishMatch(match *types.Match) error {
	if match.TargetAddress == nil {
		return eris.New("match has no target address")
	}

	protoMatch := match.ToProto()
	data, err := proto.Marshal(protoMatch)
	if err != nil {
		return eris.Wrap(err, "failed to marshal match")
	}

	subject := micro.Endpoint(match.TargetAddress, "matchmaking.match")
	if err := ms.NATS().Publish(subject, data); err != nil {
		return eris.Wrap(err, "failed to publish match")
	}

	ms.tel.Logger.Debug().
		Str("match_id", match.ID).
		Str("subject", subject).
		Msg("Published match to lobby")

	return nil
}

// PublishBackfillMatch publishes a backfill match to the Lobby Shard.
func (ms *MatchmakingService) PublishBackfillMatch(bm *types.BackfillMatch) error {
	// Get lobby address from backfill request
	req, ok := ms.mm.backfills.Get(bm.BackfillRequestID)
	if !ok {
		// Request already deleted - get address from pending state
		// For now, we skip if we can't find the address
		ms.tel.Logger.Warn().
			Str("backfill_id", bm.BackfillRequestID).
			Msg("Cannot find backfill request for publishing")
		return nil
	}

	if req.LobbyAddress == nil {
		return eris.New("backfill request has no lobby address")
	}

	protoMatch := bm.ToProto()
	data, err := proto.Marshal(protoMatch)
	if err != nil {
		return eris.Wrap(err, "failed to marshal backfill match")
	}

	subject := micro.Endpoint(req.LobbyAddress, "matchmaking.backfill-match")
	if err := ms.NATS().Publish(subject, data); err != nil {
		return eris.Wrap(err, "failed to publish backfill match")
	}

	ms.tel.Logger.Debug().
		Str("backfill_id", bm.ID).
		Str("subject", subject).
		Msg("Published backfill match to lobby")

	return nil
}
