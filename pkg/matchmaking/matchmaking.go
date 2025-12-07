// Package matchmaking provides a distributed matchmaking shard for multi-tenant game matchmaking.
// It implements the micro.ShardEngine interface for deterministic replay and state management.
package matchmaking

import (
	"context"
	"crypto/sha256"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/matchmaking/store"
	"github.com/argus-labs/world-engine/pkg/matchmaking/types"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	matchmakingv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/matchmaking/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// World represents the matchmaking shard and serves as the main entry point.
type World struct {
	*micro.Shard
	tel telemetry.Telemetry
}

// NewWorld creates a new matchmaking world with the specified configuration.
func NewWorld(opts WorldOptions) (*World, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, eris.Wrap(err, "failed to load config")
	}

	options := newDefaultWorldOptions()
	if err := options.applyConfig(cfg); err != nil {
		return nil, eris.Wrap(err, "failed to apply config")
	}
	options.apply(opts)
	if err := options.loadMatchProfiles(opts, cfg); err != nil {
		return nil, eris.Wrap(err, "failed to load match profiles")
	}
	if err := options.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid world options")
	}

	tel, err := telemetry.New(telemetry.Options{
		ServiceName: "matchmaking",
		SentryOptions: sentry.Options{
			Tags: map[string]string{
				"region":       options.Region,
				"organization": options.Organization,
				"project":      options.Project,
				"shard_id":     options.ShardID,
			},
		},
		PosthogOptions: posthog.Options{
			DistinctID: options.Organization,
			BaseProperties: map[string]any{
				"region":       options.Region,
				"organization": options.Organization,
				"project":      options.Project,
				"shard_id":     options.ShardID,
			},
		},
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize telemetry")
	}
	defer tel.RecoverAndFlush(true)

	client, err := micro.NewClient(micro.WithLogger(tel.GetLogger("client")))
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize micro client")
	}

	address := micro.GetAddress(options.Region, micro.RealmWorld, options.Organization, options.Project, options.ShardID)

	mm := newMatchmaking(options, &tel)

	shard, err := micro.NewShard(mm, micro.ShardOptions{
		Client:                 client,
		Address:                address,
		EpochFrequency:         options.EpochFrequency,
		TickRate:               options.TickRate,
		Telemetry:              &tel,
		SnapshotStorageType:    options.SnapshotStorageType,
		SnapshotStorageOptions: options.SnapshotStorageOptions,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize shard")
	}

	// Register commands
	if err := micro.RegisterCommand[CreateTicketCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register create-ticket command")
	}
	if err := micro.RegisterCommand[CancelTicketCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register cancel-ticket command")
	}
	if err := micro.RegisterCommand[CreateBackfillCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register create-backfill command")
	}
	if err := micro.RegisterCommand[CancelBackfillCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register cancel-backfill command")
	}

	// Initialize service only in leader mode
	if shard.Mode() == micro.ModeLeader {
		if err := mm.initService(client, address, &tel); err != nil {
			return nil, eris.Wrap(err, "failed to initialize matchmaking service")
		}
	}

	return &World{
		Shard: shard,
		tel:   tel,
	}, nil
}

// StartMatchmaking launches the matchmaking shard and runs until stopped.
func (w *World) StartMatchmaking() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	w.tel.CaptureEvent(ctx, "Start Matchmaking", nil)

	if err := w.Run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running matchmaking")
	}
}

func (w *World) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down matchmaking")

	mm := w.Base().(*matchmaking)
	if err := mm.shutdown(); err != nil {
		w.tel.Logger.Error().Err(err).Msg("matchmaking shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("Matchmaking shutdown complete")
}

// matchmaking implements micro.ShardEngine.
type matchmaking struct {
	mu sync.RWMutex

	// Configuration
	profiles    *store.ProfileStore
	backfillTTL time.Duration

	// State
	tickets   *store.TicketStore
	backfills *store.BackfillStore

	// Counters for generating unique IDs
	matchCounter uint64

	// Service for network communication (leader mode only)
	service *MatchmakingService
	address *microv1.ServiceAddress

	// Telemetry
	tel *telemetry.Telemetry

	// Snapshot caching
	cachedSnapshot []byte
	isDirty        bool

	// Pending matches to publish after tick
	pendingMatches         []*types.Match
	pendingBackfillMatches []*types.BackfillMatch
}

var _ micro.ShardEngine = &matchmaking{}

// newMatchmaking creates a new matchmaking instance.
func newMatchmaking(opts worldOptionsInternal, tel *telemetry.Telemetry) *matchmaking {
	return &matchmaking{
		profiles:    opts.MatchProfiles,
		backfillTTL: opts.BackfillTTL,
		tickets:     store.NewTicketStore(),
		backfills:   store.NewBackfillStore(),
		tel:         tel,
		isDirty:     true,
	}
}

// Init initializes the matchmaking shard.
func (m *matchmaking) Init() error {
	m.tel.Logger.Info().Msg("Initializing matchmaking shard")
	return nil
}

// Tick processes commands and runs matchmaking for the current tick.
func (m *matchmaking) Tick(tick micro.Tick) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := tick.Header.Timestamp

	// Clear pending matches from previous tick
	m.pendingMatches = nil
	m.pendingBackfillMatches = nil

	// 1. Process commands
	if err := m.processCommands(tick.Data.Commands, now); err != nil {
		return eris.Wrap(err, "failed to process commands")
	}

	// 2. Expire old tickets
	expired := m.tickets.ExpireTickets(now)
	if expired > 0 {
		m.tel.Logger.Debug().Int("count", expired).Msg("Expired tickets")
	}

	// 3. Expire old backfill requests
	expiredBackfills := m.backfills.ExpireRequests(now)
	if expiredBackfills > 0 {
		m.tel.Logger.Debug().Int("count", expiredBackfills).Msg("Expired backfill requests")
	}

	// 4. Process backfill requests (priority)
	m.processBackfillRequests(now)

	// 5. Process regular matchmaking
	m.processMatchProfiles(now)

	// 6. Publish matches (after tick completes)
	if m.service != nil {
		m.publishPendingMatches()
	}

	m.isDirty = true
	return nil
}

// Replay processes tick during state reconstruction.
func (m *matchmaking) Replay(tick micro.Tick) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := tick.Header.Timestamp

	// Clear pending (we don't publish during replay)
	m.pendingMatches = nil
	m.pendingBackfillMatches = nil

	// Process same as Tick but without publishing
	if err := m.processCommands(tick.Data.Commands, now); err != nil {
		return eris.Wrap(err, "failed to process commands during replay")
	}

	m.tickets.ExpireTickets(now)
	m.backfills.ExpireRequests(now)
	m.processBackfillRequests(now)
	m.processMatchProfiles(now)

	m.isDirty = true
	return nil
}

// StateHash returns a hash of the current state.
func (m *matchmaking) StateHash() ([]byte, error) {
	snapshot, err := m.Snapshot()
	if err != nil {
		return nil, eris.Wrap(err, "failed to create snapshot for state hash")
	}
	hash := sha256.Sum256(snapshot)
	return hash[:], nil
}

// Snapshot serializes the current state.
func (m *matchmaking) Snapshot() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isDirty && m.cachedSnapshot != nil {
		return m.cachedSnapshot, nil
	}

	data, err := m.serialize()
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize state")
	}

	m.cachedSnapshot = data
	m.isDirty = false
	return data, nil
}

// Restore restores state from serialized data.
func (m *matchmaking) Restore(data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.deserialize(data); err != nil {
		return eris.Wrap(err, "failed to deserialize state")
	}

	m.isDirty = true
	return nil
}

// Reset resets to clean initial state.
func (m *matchmaking) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tickets.Clear()
	m.backfills.Clear()
	m.matchCounter = 0
	m.pendingMatches = nil
	m.pendingBackfillMatches = nil
	m.cachedSnapshot = nil
	m.isDirty = true
}

// shutdown performs graceful cleanup.
func (m *matchmaking) shutdown() error {
	if m.service != nil {
		if err := m.service.Close(); err != nil {
			return eris.Wrap(err, "failed to close service")
		}
	}
	return nil
}

// processCommands handles all commands in the tick.
func (m *matchmaking) processCommands(commands []micro.Command, now time.Time) error {
	for _, cmd := range commands {
		cmdName := cmd.Command.Body.Name
		switch cmdName {
		case "create-ticket":
			if err := m.handleCreateTicket(cmd, now); err != nil {
				m.tel.Logger.Error().Err(err).Str("command", cmdName).Msg("Failed to handle command")
			}
		case "cancel-ticket":
			if err := m.handleCancelTicket(cmd); err != nil {
				m.tel.Logger.Error().Err(err).Str("command", cmdName).Msg("Failed to handle command")
			}
		case "create-backfill":
			if err := m.handleCreateBackfill(cmd, now); err != nil {
				m.tel.Logger.Error().Err(err).Str("command", cmdName).Msg("Failed to handle command")
			}
		case "cancel-backfill":
			if err := m.handleCancelBackfill(cmd); err != nil {
				m.tel.Logger.Error().Err(err).Str("command", cmdName).Msg("Failed to handle command")
			}
		}
	}
	return nil
}

// handleCreateTicket processes a create ticket command.
// Note: Basic validation (required fields, format) is done at enqueue time via Validate().
// This handler focuses on business logic that requires state access.
func (m *matchmaking) handleCreateTicket(cmd micro.Command, now time.Time) error {
	// Get typed payload
	payload, ok := cmd.Command.Body.Payload.(CreateTicketCommand)
	if !ok {
		return eris.New("invalid payload type for create-ticket command")
	}

	// Parse callback address (already validated in Validate(), safe to parse)
	callbackAddr, _ := micro.ParseAddress(payload.CallbackAddress)

	// Validate profile exists (requires state access, can't be done in Validate())
	prof, ok := m.profiles.Get(payload.MatchProfileName)
	if !ok {
		// Send error callback
		m.publishTicketError(callbackAddr, payload.PartyID, "unknown match profile: "+payload.MatchProfileName)
		return nil // Not a fatal error, we've notified the caller
	}

	// Compute pool counts
	tempTicket := &types.Ticket{Players: payload.Players}
	poolCounts := DerivePoolCounts(tempTicket, prof)

	// TTL already validated in Validate()
	ttl := time.Duration(payload.TTLSeconds) * time.Second

	// Create ticket
	ticket, err := m.tickets.Create(payload.PartyID, payload.MatchProfileName, payload.AllowBackfill, payload.Players, now, ttl, poolCounts, callbackAddr)
	if err != nil {
		// Send error callback
		m.publishTicketError(callbackAddr, payload.PartyID, err.Error())
		return nil // Not a fatal error, we've notified the caller
	}

	// Send success callback with ticket_id
	m.publishTicketCreated(callbackAddr, payload.PartyID, ticket.ID)

	m.tel.Logger.Debug().Str("ticket_id", ticket.ID).Str("party_id", payload.PartyID).Str("profile", payload.MatchProfileName).Msg("Created ticket")
	return nil
}

// handleCancelTicket processes a cancel ticket command.
func (m *matchmaking) handleCancelTicket(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(CancelTicketCommand)
	if !ok {
		return eris.New("invalid payload type for cancel-ticket command")
	}

	_, ok = m.tickets.Get(payload.TicketID)
	if !ok {
		return eris.Errorf("ticket not found: %s", payload.TicketID)
	}

	m.tickets.Delete(payload.TicketID)
	m.tel.Logger.Debug().Str("ticket_id", payload.TicketID).Msg("Cancelled ticket")
	return nil
}

// handleCreateBackfill processes a create backfill command.
func (m *matchmaking) handleCreateBackfill(cmd micro.Command, now time.Time) error {
	payload, ok := cmd.Command.Body.Payload.(CreateBackfillCommand)
	if !ok {
		return eris.New("invalid payload type for create-backfill command")
	}

	// Parse lobby address
	var lobbyAddr *microv1.ServiceAddress
	if payload.LobbyAddress != "" {
		var err error
		lobbyAddr, err = micro.ParseAddress(payload.LobbyAddress)
		if err != nil {
			return eris.Wrapf(err, "invalid lobby address: %s", payload.LobbyAddress)
		}
	}

	m.backfills.Create(payload.MatchID, payload.MatchProfileName, payload.TeamName, payload.SlotsNeeded, lobbyAddr, now, m.backfillTTL)
	m.tel.Logger.Debug().Str("match_id", payload.MatchID).Str("team", payload.TeamName).Msg("Created backfill request")
	return nil
}

// handleCancelBackfill processes a cancel backfill command.
func (m *matchmaking) handleCancelBackfill(cmd micro.Command) error {
	payload, ok := cmd.Command.Body.Payload.(CancelBackfillCommand)
	if !ok {
		return eris.New("invalid payload type for cancel-backfill command")
	}

	if !m.backfills.Delete(payload.BackfillRequestID) {
		return eris.Errorf("backfill request not found: %s", payload.BackfillRequestID)
	}

	m.tel.Logger.Debug().Str("backfill_id", payload.BackfillRequestID).Msg("Cancelled backfill request")
	return nil
}

// processBackfillRequests attempts to match backfill requests (priority).
func (m *matchmaking) processBackfillRequests(now time.Time) {
	for _, req := range m.backfills.All() {
		prof, ok := m.profiles.Get(req.MatchProfileName)
		if !ok {
			continue
		}

		// Get backfill-eligible tickets
		tickets := m.tickets.GetBackfillEligible(req.MatchProfileName)
		if len(tickets) == 0 {
			continue
		}

		// Record pool size and start time for benchmarking
		poolSize := len(tickets)
		startTime := time.Now()

		// Filter candidates
		candidates := FilterBackfillCandidates(tickets, prof, req.SlotsNeeded)
		if len(candidates) == 0 {
			continue
		}

		// Run backfill matching
		result := RunBackfillMatchmaking(candidates, req.SlotsNeeded, now)
		if !result.Success {
			continue
		}

		// Calculate duration
		durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0

		// Create backfill match
		m.matchCounter++
		backfillMatch := &types.BackfillMatch{
			ID:                uuid.New().String(),
			BackfillRequestID: req.ID,
			MatchID:           req.MatchID,
			TeamName:          req.TeamName,
			Tickets:           make([]*types.Ticket, len(result.Assignments)),
			CreatedAt:         now,
		}
		for i, a := range result.Assignments {
			backfillMatch.Tickets[i] = a.Ticket
		}

		// Remove matched tickets
		ticketIDs := make([]string, len(result.Assignments))
		for i, a := range result.Assignments {
			ticketIDs[i] = a.Ticket.ID
		}
		m.tickets.DeleteMultiple(ticketIDs)

		// Remove backfill request
		m.backfills.Delete(req.ID)

		// Queue for publishing
		m.pendingBackfillMatches = append(m.pendingBackfillMatches, backfillMatch)

		m.tel.Logger.Info().
			Str("backfill_id", req.ID).
			Str("match_id", req.MatchID).
			Int("players", len(result.Assignments)).
			Int("pool_size", poolSize).
			Float64("duration_ms", durationMs).
			Msg("Backfill match created")
	}
}

// processMatchProfiles runs matchmaking for each profile.
func (m *matchmaking) processMatchProfiles(now time.Time) {
	for _, prof := range m.profiles.All() {
		tickets := m.tickets.GetByProfile(prof.Name)
		if len(tickets) == 0 {
			continue
		}

		// Record pool size and start time for benchmarking
		poolSize := len(tickets)
		startTime := time.Now()

		// Filter candidates
		candidates := FilterCandidates(tickets, prof)
		if len(candidates) == 0 {
			continue
		}

		// Run matchmaking
		result := RunMatchmaking(candidates, prof, now)
		if !result.Success {
			continue
		}

		// Calculate duration
		durationMs := float64(time.Since(startTime).Microseconds()) / 1000.0

		// Create match
		m.matchCounter++
		match := result.ToMatch(
			uuid.New().String(),
			prof,
			m.address,
			now,
		)

		// Remove matched tickets
		ticketIDs := make([]string, len(result.Assignments))
		for i, a := range result.Assignments {
			ticketIDs[i] = a.Ticket.ID
		}
		m.tickets.DeleteMultiple(ticketIDs)

		// Queue for publishing
		m.pendingMatches = append(m.pendingMatches, match)

		m.tel.Logger.Info().
			Str("match_id", match.ID).
			Str("profile", prof.Name).
			Int("players", match.TotalPlayers()).
			Int("pool_size", poolSize).
			Float64("duration_ms", durationMs).
			Msg("Match created")
	}
}

// publishPendingMatches publishes matches to lobby shards.
func (m *matchmaking) publishPendingMatches() {
	for _, match := range m.pendingMatches {
		if err := m.service.PublishMatch(match); err != nil {
			m.tel.Logger.Error().Err(err).Str("match_id", match.ID).Msg("Failed to publish match")
		}
	}

	for _, bm := range m.pendingBackfillMatches {
		if err := m.service.PublishBackfillMatch(bm); err != nil {
			m.tel.Logger.Error().Err(err).Str("backfill_id", bm.ID).Msg("Failed to publish backfill match")
		}
	}
}

// initService initializes the NATS service for leader mode.
func (m *matchmaking) initService(client *micro.Client, address *microv1.ServiceAddress, tel *telemetry.Telemetry) error {
	m.address = address
	service, err := NewMatchmakingService(client, address, m, tel)
	if err != nil {
		return eris.Wrap(err, "failed to create matchmaking service")
	}
	m.service = service
	return nil
}

// publishTicketCreated publishes a ticket-created callback to the Game Shard.
// Endpoint: <callback_address>.matchmaking.ticket-created
func (m *matchmaking) publishTicketCreated(callbackAddr *microv1.ServiceAddress, partyID, ticketID string) {
	if m.service == nil || callbackAddr == nil {
		return
	}

	resp := &matchmakingv1.TicketCreatedCallback{
		PartyId:  partyID,
		TicketId: ticketID,
	}

	if err := m.service.NATS().Publish(callbackAddr, "matchmaking.ticket-created", resp); err != nil {
		m.tel.Logger.Error().Err(err).
			Str("party_id", partyID).
			Str("ticket_id", ticketID).
			Msg("Failed to publish ticket-created callback")
	} else {
		m.tel.Logger.Debug().
			Str("party_id", partyID).
			Str("ticket_id", ticketID).
			Msg("Published ticket-created callback")
	}
}

// publishTicketError publishes a ticket-error callback to the Game Shard.
// Endpoint: <callback_address>.matchmaking.ticket-error
func (m *matchmaking) publishTicketError(callbackAddr *microv1.ServiceAddress, partyID, errorMsg string) {
	if m.service == nil || callbackAddr == nil {
		return
	}

	resp := &matchmakingv1.TicketErrorCallback{
		PartyId: partyID,
		Error:   errorMsg,
	}

	if err := m.service.NATS().Publish(callbackAddr, "matchmaking.ticket-error", resp); err != nil {
		m.tel.Logger.Error().Err(err).
			Str("party_id", partyID).
			Str("error", errorMsg).
			Msg("Failed to publish ticket-error callback")
	} else {
		m.tel.Logger.Debug().
			Str("party_id", partyID).
			Str("error", errorMsg).
			Msg("Published ticket-error callback")
	}
}
