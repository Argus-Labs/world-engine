package cardinal

import (
	"bytes"
	"context"
	"crypto/sha256"

	// "github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
)

func (w *World) runFollower(ctx context.Context) error {
	for {
		err := w.epochLog.Consume(ctx, func(epoch *iscv1.Epoch) error {
			return w.replayEpoch(epoch)
		})
		if err != nil {
			return err
		}
	}
}

func (w *World) sync() error {
	logger := w.tel.GetLogger("sync")
	logger.Debug().Str("mode", string(w.options.Mode)).Msg("initializing shard")

	// Fetch from snapshot if exists and leader
	if w.options.Mode == ModeLeader {
		if ok := w.restoreSnapshot(); !ok {
			// TODO: only reset if error, this code will reset even if snapshot doesn't exist.
			// Something went wrong when restoring from snapshot, reinitialize world (since it may be in
			// a corrupted state), and fallback to replaying epochs.
			// w.world = ecs.NewWorld() // Restart to a fresh world
			// w.world.Init()           // Reinitialize schedulers
		}
	}

	// Set mode to follower for the duration of the sync.
	originalMode := w.options.Mode
	w.options.Mode = ModeFollower
	defer func() { w.options.Mode = originalMode }()

	// Fetch from epoch log
	epochCount, err := w.epochLog.EpochCount()
	if err != nil {
		return eris.Wrap(err, "failed to get number of epochs to sync")
	}
	startEpoch := w.epochHeight
	logger.Info().Uint64("from_epoch", startEpoch).Uint64("pending_messages", epochCount).Msg("starting sync")

	// Fetch pending epochs in batches.
	const batchSize = 256
	for epochCount > 0 {
		processed, err := w.epochLog.ConsumeBatch(context.Background(), batchSize, func(epoch *iscv1.Epoch) error {
			// Skip epochs already restored from snapshot.
			if epoch.GetEpochHeight() < startEpoch {
				return nil
			}
			return w.replayEpoch(epoch)
		})
		if err != nil {
			return eris.Wrap(err, "failed to replay epoch batch")
		}
		epochCount -= uint64(processed)
	}

	return nil
}

func (w *World) replayEpoch(epoch *iscv1.Epoch) error {
	if epoch.GetEpochHeight() != w.epochHeight {
		return eris.Errorf("mismatched epoch, expected: %d, actual: %d", w.epochHeight, epoch.GetEpochHeight())
	}

	for _, tick := range epoch.GetTicks() {
		if err := w.replayTick(tick); err != nil {
			return eris.Wrap(err, "failed to replay tick")
		}
	}

	data, err := w.world.Serialize()
	if err != nil {
		return eris.Wrap(err, "failed to serialize world")
	}

	currentHash := sha256.Sum256(data)
	if !bytes.Equal(epoch.GetHash(), currentHash[:]) {
		return eris.New("mismatched state hash")
	}

	return nil
}

func (w *World) replayTick(tick *iscv1.Tick) error {
	if tick.GetHeader().GetTickHeight() != w.tickHeight {
		return eris.Errorf("mismatched tick, expected: %d, actual: %d", w.tickHeight, tick.GetHeader().GetTickHeight())
	}

	// Enqueue to command manager so the command payloads get marshalled to their corresponding concrete type.
	for _, command := range tick.GetData().GetCommands() {
		if err := w.commands.Enqueue(command); err != nil {
			return eris.Wrap(err, "failed to enqueue command")
		}
	}

	err := w.Tick(tick.GetHeader().GetTimestamp().AsTime())
	if err != nil {
		return eris.Wrap(err, "failed to replay tick")
	}

	return nil
}

func (w *World) restoreSnapshot() bool {
	logger := w.tel.GetLogger("snapshot")

	if !w.snapshotStorage.Exists() {
		logger.Debug().Msg("no snapshot found")
		return false
	}

	logger.Debug().Msg("restoring from snapshot")
	snapshot, err := w.snapshotStorage.Load()
	if err != nil {
		logger.Warn().Err(err).Msg("failed to load snapshot, falling back to normal init")
		return false
	}

	// Attempt to restore ECS world from snapshot.
	if err := w.world.Deserialize(snapshot.Data); err != nil {
		logger.Warn().Err(err).Msg("failed to restore world from snapshot, resetting and falling back to normal init")
		return false
	}

	// Validate restored state hash matches snapshot.
	data, err := w.world.Serialize()
	if err != nil {
		logger.Error().Err(err).Msg("failed to serialize restored world, resetting and falling back to normal init")
		return false
	}

	currentHash := sha256.Sum256(data)
	if !bytes.Equal(currentHash[:], snapshot.StateHash) {
		logger.Error().
			Str("expected_hash", string(snapshot.StateHash)).
			Str("actual_hash", string(currentHash[:])).
			Msg("snapshot state hash mismatch, resetting and falling back to normal init")
		return false
	}

	// Only update shard state after successful restoration and validation.
	w.epochHeight = snapshot.EpochHeight + 1
	w.tickHeight = snapshot.TickHeight + 1

	logger.Info().
		Uint64("epoch", snapshot.EpochHeight).
		Uint64("tick", snapshot.TickHeight).
		Msg("successfully restored and validated snapshot")
	return true
}
