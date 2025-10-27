// Package micro provides distributed shard management functionality for the ECS framework.
// It implements a leader-follower model with deterministic replay for maintaining consistency
// across distributed game instances.
package micro

import (
	"context"
	"fmt"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
)

// ShardEngine defines the interface that must be implemented by specific shard types.
// It provides the core functionality for initializing, processing inputs, and maintaining state.
type ShardEngine interface {
	// Init initializes the shard's initial state.
	Init() error

	// Tick processes the given input and advances the shard state.
	Tick(Tick) error

	// Replay processes the given input during state reconstruction.
	// This should produce the same result as Tick for deterministic replay.
	Replay(Tick) error

	// StateHash returns a hash of the current shard state for consistency verification.
	StateHash() ([]byte, error)

	// Snapshot captures the current shard state and returns it as serialized bytes.
	Snapshot() ([]byte, error)

	// Restore restores the shard state from the given serialized bytes.
	Restore(data []byte) error

	// Reset resets the engine to its clean initial state.
	Reset()
}

// Shard manages the shard lifecycle depending on the operation mode.
type Shard struct {
	// Specific shard implementations, e.g. Cardinal, Registry, etc.
	base ShardEngine

	// Networking and epoch JetStream management.
	client   *Client             // NATS client
	js       jetstream.JetStream // JetStream client
	stream   jetstream.Stream    // The epoch stream
	consumer jetstream.Consumer  // Reusable JetStream consumer
	subject  string              // Epoch subject
	commands commandManager      // Receives commands

	// Epoch and tick book-keeping.
	mode        ShardMode // Shard mode
	epochHeight uint64    // Epoch height
	tickHeight  uint64    // Tick height
	frequency   uint32    // Epoch frequency (number of ticks per epoch)
	tickRate    float64   // Tick rate (number of ticks per second)
	ticks       []Tick    // List of ticks in the current epoch

	// Snapshots.
	snapshotStorage   SnapshotStorage // Snapshot storage
	snapshotFrequency uint32          // Snapshot every N epochs

	// Utilities.
	tel            *telemetry.Telemetry
	disablePersona bool
}

// NewShard creates a new shard instance with the given base implementation and options.
func NewShard(base ShardEngine, opts ShardOptions) (*Shard, error) {
	config, err := loadShardConfig()
	if err != nil {
		return nil, eris.Wrap(err, "failed to load shard config")
	}

	options := newDefaultShardOptions()
	config.applyToOptions(&options)
	options.apply(opts)
	if err := options.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid shard options")
	}

	subject := Endpoint(options.Address, "epoch")
	streamName := formatStreamName(options.Address)

	js, err := jetstream.New(options.Client.Conn)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create jetstream client")
	}

	streamConfig := jetstream.StreamConfig{
		Name:      streamName,
		Subjects:  []string{subject},
		Retention: jetstream.LimitsPolicy,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
		MaxBytes:  int64(options.EpochStreamMaxBytes),
	}

	// Try to get existing stream first, if it exists we'll update it
	stream, err := js.Stream(context.Background(), streamName) //nolint: staticcheck, wastedassign // this is ok
	if err != nil {
		// Stream doesn't exist, try to create it
		stream, err = js.CreateStream(context.Background(), streamConfig)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to create epoch stream (name=%s, subjects=%v, maxBytes=%d)",
				streamConfig.Name, streamConfig.Subjects, streamConfig.MaxBytes)
		}
	} else {
		// Stream exists, update it with new config
		stream, err = js.UpdateStream(context.Background(), streamConfig)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to update existing epoch stream (name=%s, subjects=%v, maxBytes=%d)",
				streamConfig.Name, streamConfig.Subjects, streamConfig.MaxBytes)
		}
	}

	consumer, err := stream.CreateConsumer(context.Background(), jetstream.ConsumerConfig{
		FilterSubject: subject,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create stream consumer")
	}

	storage, err := createSnapshotStorage(options)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create snapshot storage")
	}

	s := &Shard{
		base: base,

		client:   options.Client,
		js:       js,
		stream:   stream,
		consumer: consumer,
		subject:  subject,

		mode:              options.Mode,
		epochHeight:       0,
		tickHeight:        0,
		frequency:         options.EpochFrequency,
		tickRate:          options.TickRate,
		ticks:             make([]Tick, 0, int(options.EpochFrequency)),
		snapshotStorage:   storage,
		snapshotFrequency: options.SnapshotFrequency,

		tel:            options.Telemetry,
		disablePersona: options.DisablePersona,
	}

	commands, err := newCommandManager(s, options)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create command manager")
	}
	s.commands = commands

	return s, nil
}

// Run starts the shard's main execution loop.
func (s *Shard) Run(ctx context.Context) error {
	if err := s.init(); err != nil {
		return eris.Wrap(err, "failed to initialize shard")
	}

	if err := s.sync(); err != nil {
		return eris.Wrap(err, "failed to sync shard state")
	}

	// Core shard loop based on the mode.
	logger := s.tel.GetLogger("shard")
	logger.Info().Str("mode", s.mode.String()).Msg("starting core shard loop")
	switch s.mode {
	case ModeLeader:
		return s.runLeader(ctx)
	case ModeFollower:
		return s.runFollower(ctx)
	case ModeUndefined:
		assert.That(true, "unreachable")
	}

	return nil
}

// Mode returns the current operating mode of the shard (leader, follower, or undefined).
func (s *Shard) Mode() ShardMode {
	return s.mode
}

// Base returns the underlying shard engine implementation.
func (s *Shard) Base() ShardEngine {
	return s.base
}

func (s *Shard) IsDisablePersona() bool {
	return s.disablePersona
}

// RegisterCommand registers a command type T with the shard's command manager.
// This allows the shard to receive and process commands of the specified type.
func RegisterCommand[T ShardCommand](s *Shard) error {
	return registerCommand[T](&s.commands)
}

// CurrentTick returns the current tick information.
// Returns an error if called during the inter-tick period, whic happens after an epoch is published
// but before the next tick is started.
func (s *Shard) CurrentTick() (Tick, error) {
	if len(s.ticks) == 0 {
		return Tick{}, eris.New("cannot get current tick during inter-tick period")
	}

	return s.ticks[len(s.ticks)-1], nil
}

func createSnapshotStorage(opts ShardOptions) (SnapshotStorage, error) {
	switch opts.SnapshotStorageType {
	case SnapshotStorageNop:
		return NewNopSnapshotStorage(), nil

	case SnapshotStorageJetStream:
		jsOpts, err := newJetstreamSnapshotStorageOptions()
		if err != nil {
			return nil, eris.Wrap(err, "failed to initialize JetStream storage options")
		}
		jsOpts.apply(opts)
		if err := jsOpts.validate(); err != nil {
			return nil, eris.Wrap(err, "failed to validate JetStream storage options")
		}
		return NewJetStreamSnapshotStorage(jsOpts)

	case SnapshotStorageUndefined:
	default:
	}
	return nil, eris.New("invalid snapshot storage type")
}

// -------------------------------------------------------------------------------------------------
// Utilities
// -------------------------------------------------------------------------------------------------

// formatStreamName creates a unique stream name based on the service address.
func formatStreamName(address *ServiceAddress) string {
	return fmt.Sprintf("%s_%s_%s_epoch",
		address.GetOrganization(), address.GetProject(), address.GetServiceId())
}

// epochPublishOptions creates publish options for epoch messages with deduplication and ordering.
func epochPublishOptions(subject string, epochCount uint64) []jetstream.PublishOpt {
	opts := []jetstream.PublishOpt{jetstream.WithMsgID(fmt.Sprintf("%s-%d", subject, epochCount))}
	if epochCount > 0 {
		opts = append(opts, jetstream.WithExpectLastMsgID(fmt.Sprintf("%s-%d", subject, epochCount-1)))
	}
	return opts
}
