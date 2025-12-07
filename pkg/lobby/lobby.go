// Package lobby provides a distributed lobby shard for multi-tenant game lobby management.
// It implements the micro.ShardEngine interface for deterministic replay and state management.
package lobby

import (
	"context"
	"crypto/sha256"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/pkg/lobby/store"
	"github.com/argus-labs/world-engine/pkg/lobby/types"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/pkg/telemetry/posthog"
	"github.com/argus-labs/world-engine/pkg/telemetry/sentry"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
)

// World represents the lobby shard and serves as the main entry point.
type World struct {
	*micro.Shard
	tel telemetry.Telemetry
}

// NewWorld creates a new lobby world with the specified configuration.
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
	if err := options.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid world options")
	}
	// Set defaults if not provided
	if options.Region == "" {
		options.Region = "local"
	}

	tel, err := telemetry.New(telemetry.Options{
		ServiceName: "lobby",
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

	lb := newLobby(options, &tel)

	shard, err := micro.NewShard(lb, micro.ShardOptions{
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

	// Register party commands
	if err := micro.RegisterCommand[CreatePartyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register create-party command")
	}
	if err := micro.RegisterCommand[JoinPartyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register join-party command")
	}
	if err := micro.RegisterCommand[LeavePartyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register leave-party command")
	}
	if err := micro.RegisterCommand[KickFromPartyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register kick-from-party command")
	}
	if err := micro.RegisterCommand[DisbandPartyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register disband-party command")
	}
	if err := micro.RegisterCommand[SetPartyLeaderCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register set-party-leader command")
	}
	if err := micro.RegisterCommand[SetPartyOpenCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register set-party-open command")
	}

	// Register lobby commands
	if err := micro.RegisterCommand[CreateLobbyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register create-lobby command")
	}
	if err := micro.RegisterCommand[JoinLobbyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register join-lobby command")
	}
	if err := micro.RegisterCommand[LeaveLobbyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register leave-lobby command")
	}
	if err := micro.RegisterCommand[KickFromLobbyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register kick-from-lobby command")
	}
	if err := micro.RegisterCommand[CloseLobbyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register close-lobby command")
	}

	// Register ready/match lifecycle commands
	if err := micro.RegisterCommand[SetReadyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register set-ready command")
	}
	if err := micro.RegisterCommand[UnsetReadyCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register unset-ready command")
	}
	if err := micro.RegisterCommand[StartMatchCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register start-match command")
	}
	if err := micro.RegisterCommand[EndMatchCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register end-match command")
	}

	// Register internal commands (from Game Shard)
	if err := micro.RegisterCommand[HeartbeatCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register heartbeat command")
	}
	if err := micro.RegisterCommand[SetPlayerStatusCommand](shard); err != nil {
		return nil, eris.Wrap(err, "failed to register set-player-status command")
	}
	// Note: RequestBackfill and CancelBackfill are handled via service endpoints, not commands

	// Initialize service only in leader mode
	if shard.Mode() == micro.ModeLeader {
		if err := lb.initService(client, address, &tel); err != nil {
			return nil, eris.Wrap(err, "failed to initialize lobby service")
		}
	}

	return &World{
		Shard: shard,
		tel:   tel,
	}, nil
}

// StartLobby launches the lobby shard and runs until stopped.
func (w *World) StartLobby() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	defer w.shutdown()
	defer w.tel.RecoverAndFlush(true)

	w.tel.CaptureEvent(ctx, "Start Lobby", nil)

	if err := w.Run(ctx); err != nil {
		w.tel.CaptureException(ctx, err)
		w.tel.Logger.Error().Err(err).Msg("failed running lobby")
	}
}

func (w *World) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.tel.Logger.Info().Msg("Shutting down lobby")

	lb := w.Base().(*lobby)
	if err := lb.shutdown(); err != nil {
		w.tel.Logger.Error().Err(err).Msg("lobby shutdown error")
		w.tel.CaptureException(ctx, err)
	}

	if err := w.tel.Shutdown(ctx); err != nil {
		w.tel.Logger.Error().Err(err).Msg("telemetry shutdown error")
	}

	w.tel.Logger.Info().Msg("Lobby shutdown complete")
}

// lobby implements micro.ShardEngine.
type lobby struct {
	mu sync.RWMutex

	// Configuration
	heartbeatTimeout time.Duration

	// State
	parties *store.PartyStore
	lobbies *store.LobbyStore

	// Service for network communication (leader mode only)
	service *LobbyService
	address *microv1.ServiceAddress

	// Telemetry
	tel *telemetry.Telemetry

	// Snapshot caching
	cachedSnapshot []byte
	isDirty        bool

	// Internal command queue for deterministic state changes from service handlers.
	// These commands are queued by service handlers and processed in Tick().
	internalQueue []InternalCommand
}

var _ micro.ShardEngine = &lobby{}

// newLobby creates a new lobby instance.
func newLobby(opts WorldOptions, tel *telemetry.Telemetry) *lobby {
	return &lobby{
		heartbeatTimeout: opts.HeartbeatTimeout,
		parties:          store.NewPartyStore(),
		lobbies:          store.NewLobbyStore(),
		tel:              tel,
		isDirty:          true,
	}
}

// Init initializes the lobby shard.
func (l *lobby) Init() error {
	l.tel.Logger.Info().Msg("Initializing lobby shard")
	return nil
}

// Tick processes commands and performs cleanup for the current tick.
func (l *lobby) Tick(tick micro.Tick) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := tick.Header.Timestamp

	// 1. Process external commands (from NATS)
	if err := l.processCommands(tick.Data.Commands, now); err != nil {
		return eris.Wrap(err, "failed to process commands")
	}

	// 2. Process internal commands (from service handlers)
	if err := l.processInternalCommands(now); err != nil {
		return eris.Wrap(err, "failed to process internal commands")
	}

	// 3. Cleanup zombie lobbies (in_game with no heartbeat)
	l.cleanupZombieLobbies(now)

	l.isDirty = true
	return nil
}

// EnqueueInternalCommand queues an internal command for processing in the next tick.
// This method is thread-safe and used by service handlers to ensure deterministic state changes.
func (l *lobby) EnqueueInternalCommand(cmd InternalCommand) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.internalQueue = append(l.internalQueue, cmd)
}

// Replay processes tick during state reconstruction.
func (l *lobby) Replay(tick micro.Tick) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := tick.Header.Timestamp

	// Process same as Tick
	if err := l.processCommands(tick.Data.Commands, now); err != nil {
		return eris.Wrap(err, "failed to process commands during replay")
	}

	l.cleanupZombieLobbies(now)

	l.isDirty = true
	return nil
}

// StateHash returns a hash of the current state.
func (l *lobby) StateHash() ([]byte, error) {
	snapshot, err := l.Snapshot()
	if err != nil {
		return nil, eris.Wrap(err, "failed to create snapshot for state hash")
	}
	hash := sha256.Sum256(snapshot)
	return hash[:], nil
}

// Snapshot serializes the current state.
func (l *lobby) Snapshot() ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if !l.isDirty && l.cachedSnapshot != nil {
		return l.cachedSnapshot, nil
	}

	data, err := l.serialize()
	if err != nil {
		return nil, eris.Wrap(err, "failed to serialize state")
	}

	l.cachedSnapshot = data
	l.isDirty = false
	return data, nil
}

// Restore restores state from serialized data.
func (l *lobby) Restore(data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.deserialize(data); err != nil {
		return eris.Wrap(err, "failed to deserialize state")
	}

	l.isDirty = true
	return nil
}

// Reset resets to clean initial state.
func (l *lobby) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.parties.Clear()
	l.lobbies.Clear()
	l.internalQueue = nil
	l.cachedSnapshot = nil
	l.isDirty = true
}

// shutdown performs graceful cleanup.
func (l *lobby) shutdown() error {
	if l.service != nil {
		if err := l.service.Close(); err != nil {
			return eris.Wrap(err, "failed to close service")
		}
	}
	return nil
}

// cleanupZombieLobbies marks lobbies without heartbeat as ended.
// From ADR-030: Lobbies in `in_game` state that haven't received a heartbeat
// (3 consecutive misses = 15 minutes) are marked as `ended`.
func (l *lobby) cleanupZombieLobbies(now time.Time) {
	zombies := l.lobbies.GetZombieLobbies(now, l.heartbeatTimeout)
	for _, zombie := range zombies {
		l.tel.Logger.Warn().
			Str("match_id", zombie.MatchID).
			Msg("Marking zombie lobby as ended")

		if err := l.lobbies.SetState(zombie.MatchID, types.LobbyStateEnded); err != nil {
			l.tel.Logger.Error().Err(err).Str("match_id", zombie.MatchID).Msg("Failed to mark zombie lobby as ended")
		}
	}
}


// checkAllPartiesReady checks if all parties in a lobby are ready and updates state.
func (l *lobby) checkAllPartiesReady(lobbyID string) {
	lb, ok := l.lobbies.Get(lobbyID)
	if !ok || lb.State != types.LobbyStateWaiting {
		return
	}

	allReady := true
	for _, partyID := range lb.Parties {
		party, ok := l.parties.Get(partyID)
		if !ok || !party.IsReady {
			allReady = false
			break
		}
	}

	if allReady && len(lb.Parties) >= lb.MinPlayers {
		if err := l.lobbies.SetState(lobbyID, types.LobbyStateReady); err != nil {
			l.tel.Logger.Error().Err(err).Str("lobby_id", lobbyID).Msg("Failed to set lobby ready")
		}
	}
}

// checkAnyPartyUnready checks if any party is unready and reverts lobby to waiting.
func (l *lobby) checkAnyPartyUnready(lobbyID string) {
	lb, ok := l.lobbies.Get(lobbyID)
	if !ok || lb.State != types.LobbyStateReady {
		return
	}

	for _, partyID := range lb.Parties {
		party, ok := l.parties.Get(partyID)
		if !ok || !party.IsReady {
			if err := l.lobbies.SetState(lobbyID, types.LobbyStateWaiting); err != nil {
				l.tel.Logger.Error().Err(err).Str("lobby_id", lobbyID).Msg("Failed to revert lobby to waiting")
			}
			return
		}
	}
}
