package micro

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/assert"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// -------------------------------------------------------------------------------------------------
// Follower mode
// -------------------------------------------------------------------------------------------------

// runFollower executes the follower mode main loop.
// It continuously consumes epochs from the stream and replays them in real-time.
func (s *Shard) runFollower(ctx context.Context) error {
	consumeCtx, err := s.consumer.Consume(func(msg jetstream.Msg) {
		if err := s.replayEpoch(msg.Data()); err != nil {
			logger := s.tel.GetLogger("shard")
			logger.Error().Err(err).Msg("failed to replay epoch")
			return
		}

		if err := msg.Ack(); err != nil {
			logger := s.tel.GetLogger("shard")
			logger.Error().Err(err).Msg("failed to acknowledge message")
		}
	})
	if err != nil {
		return eris.Wrap(err, "failed to start consuming epochs")
	}

	// Consume is non-blocking, so when we return from this function, e.g. context is cancelled, the
	// consumeCts.Stop() method is called to stop the consumer.
	defer consumeCtx.Stop()

	<-ctx.Done()
	return ctx.Err()
}

// -------------------------------------------------------------------------------------------------
// Initialization and Sync
// -------------------------------------------------------------------------------------------------

// init initializes the shard. It restores from a snapshot if available and in leader mode,
// otherwise runs the shard engine's init method.
func (s *Shard) init() error {
	logger := s.tel.GetLogger("shard")
	if s.mode == ModeLeader && s.restoreSnapshot() {
		logger.Info().Uint64("epoch", s.epochHeight).Msg("shard restored from snapshot")
		return nil
	}

	_ = s.beginTick(TickData{})

	err := s.base.Init()
	if err != nil {
		return eris.Wrap(err, "failed to initialize shard")
	}

	logger.Info().Msg("shard initialized from scratch")
	return s.endTick()
}

// restoreSnapshot attempts to restore shard state from a snapshot. Returns true if restoration was
// successful, false if fallback to normal init is needed.
func (s *Shard) restoreSnapshot() bool {
	if !s.snapshotStorage.Exists() {
		return false
	}

	logger := s.tel.GetLogger("shard")

	snapshot, err := s.snapshotStorage.Load()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load snapshot, falling back to normal init")
		return false
	}

	// Attempt to restore from snapshot.
	if err := s.base.Restore(snapshot.Data); err != nil {
		logger.Warn().Err(err).Msg("failed to restore engine from snapshot, resetting and falling back to normal init")
		s.base.Reset()
		return false
	}

	// Validate restored state hash matches snapshot.
	currentHash, err := s.base.StateHash()
	if err != nil {
		logger.Error().Err(err).Msg("failed to get current state hash, resetting and falling back to normal init")
		s.base.Reset()
		return false
	}
	if !bytes.Equal(currentHash, snapshot.StateHash) {
		logger.Error().
			Str("expected_hash", string(snapshot.StateHash)).
			Str("actual_hash", string(currentHash)).
			Msg("snapshot state hash mismatch, resetting and falling back to normal init")
		s.base.Reset()
		return false
	}

	// Only update shard state after successful restoration and validation.
	s.epochHeight = snapshot.EpochHeight + 1
	s.tickHeight = snapshot.TickHeight + 1

	logger.Info().
		Uint64("epoch", snapshot.EpochHeight).
		Uint64("tick", snapshot.TickHeight).
		Msg("successfully restored and validated snapshot")

	return true
}

// sync replays epochs from the stream starting from the current epoch height to bring the shard up
// to the latest state.
func (s *Shard) sync() error { //nolint:gocognit // its fine
	logger := s.tel.GetLogger("shard")

	// Set mode to follower for the duration of the sync.
	originalMode := s.mode
	s.mode = ModeFollower
	defer func() { s.mode = originalMode }()

	// Get consumer info to determine how many messages are pending
	cInfo, err := s.consumer.Info(context.Background())
	if err != nil {
		return eris.Wrap(err, "failed to fetch consumer info")
	}

	pending := cInfo.NumPending
	startEpoch := s.epochHeight
	logger.Info().Uint64("from_epoch", startEpoch).Uint64("pending_messages", pending).Msg("starting sync")

	const batchSize = 256
	for pending > 0 {
		batch, err := s.consumer.FetchNoWait(batchSize)
		if err != nil {
			return eris.Wrap(err, "failed to fetch message batch")
		}

		processed := uint64(0)
		for message := range batch.Messages() {
			processed++

			msgID := message.Headers().Get("Nats-Msg-Id")
			epochHeight := epochHeightFromMsgID(msgID, s.subject)

			// Skip epochs that are below current epoch height
			if epochHeight < s.epochHeight {
				if err := message.Ack(); err != nil {
					return eris.Wrap(err, "failed to acknowledge skipped message")
				}
				continue
			}

			if err := s.replayEpoch(message.Data()); err != nil {
				return eris.Wrap(err, "failed to replay epoch")
			}

			if err := message.Ack(); err != nil {
				return eris.Wrap(err, "failed to acknowledge message")
			}
		}

		// Check for errors that occurred during message delivery.
		if err := batch.Error(); err != nil {
			return eris.Wrap(err, "error occurred during message batch delivery")
		}

		pending -= processed
	}

	endEpoch := s.epochHeight
	logger.Info().Uint64("from_epoch", startEpoch).Uint64("to_epoch", endEpoch).Msg("sync completed")
	return nil
}

// replayEpoch deserializes and replays an epoch to reconstruct state.
// It validates the epoch and replays each tick to ensure deterministic state reconstruction.
func (s *Shard) replayEpoch(epochBytes []byte) error {
	epoch := iscv1.Epoch{}
	if err := proto.Unmarshal(epochBytes, &epoch); err != nil {
		return eris.Wrap(err, "failed to unmarshal epoch")
	}
	if err := protovalidate.Validate(&epoch); err != nil {
		return eris.Wrap(err, "failed to validate epoch")
	}

	if epoch.GetEpochHeight() != s.epochHeight {
		return eris.Errorf("mismatched epoch, expected: %d, actual: %d", s.epochHeight, epoch.GetEpochHeight())
	}

	for _, tick := range epoch.GetTicks() {
		if tick.GetHeader().GetTickHeight() == 0 {
			continue // This is done in init()
		}

		if err := s.replayTick(tick); err != nil {
			return eris.Wrap(err, "failed to replay tick")
		}
	}

	currentHash, err := s.base.StateHash()
	if err != nil {
		return eris.Wrap(err, "failed to get current state hash")
	}
	if !bytes.Equal(epoch.GetHash(), currentHash) {
		return eris.New("mismatched state hash")
	}

	return nil
}

// replayTick replays a single tick during epoch reconstruction.
// It deserializes the input and calls the replay function.
func (s *Shard) replayTick(tick *iscv1.Tick) error {
	if tick.GetHeader().GetTickHeight() != s.tickHeight {
		return eris.Errorf("mismatched tick, expected: %d, actual: %d", s.tickHeight, tick.GetHeader().GetTickHeight())
	}

	// Enqueue to command manager so the command payloads get marshalled to their corresponding concrete type.
	for _, command := range tick.GetData().GetCommands() {
		if err := s.commands.auth.VerifyCommand(command); err != nil {
			return eris.Wrap(err, "failed to verify command")
		}

		if err := s.commands.Enqueue(command); err != nil {
			return eris.Wrap(err, "failed to enqueue command")
		}
	}

	tickData := s.commands.GetTickData()

	replayTick := s.beginTick(tickData)

	err := s.base.Replay(replayTick)
	if err != nil {
		return eris.Wrap(err, "replay function failed")
	}

	if err := s.endTick(); err != nil {
		return eris.Wrap(err, "failed to end tick replay")
	}

	return nil
}

// epochHeightFromMsgID extracts the epoch height from a JetStream message ID.
// Message IDs are in the format "<subject>-<epochHeight>".
func epochHeightFromMsgID(msgID, subject string) uint64 {
	epochStr := strings.TrimPrefix(msgID, subject+"-")
	epochHeight, err := strconv.ParseUint(epochStr, 10, 64)
	assert.That(err == nil, "epoch isn't published in the expected format")
	return epochHeight
}
