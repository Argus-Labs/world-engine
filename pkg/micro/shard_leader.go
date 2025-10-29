package micro

import (
	"context"
	"slices"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// -------------------------------------------------------------------------------------------------
// Leader mode
// -------------------------------------------------------------------------------------------------

// runLeader executes the leader mode main loop.
// It processes ticks at the configured tick rate and publishes epochs to followers.
func (s *Shard) runLeader(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(float64(time.Second) / s.tickRate))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tickData := s.commands.GetTickData()

			tick := s.beginTick(tickData)

			err := s.base.Tick(tick)
			if err != nil {
				return eris.Wrap(err, "failed to run base tick function")
			}

			// Only end the tick when there is no error.
			err = s.endTick()
			if err != nil {
				return eris.Wrap(err, "failed to end tick")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// beginTick begins a new tick by recording the input and timestamp.
func (s *Shard) beginTick(data TickData) Tick {
	assert.That(len(s.ticks) < int(s.frequency), "last epoch is not submitted")

	// Copy commands slice to avoid aliasing issues with reused buffer.
	commands := slices.Clone(data.Commands)

	tick := Tick{
		Header: TickHeader{
			TickHeight: s.tickHeight,
			Timestamp:  time.Now(),
		},
		Data: TickData{Commands: commands},
	}

	s.ticks = append(s.ticks, tick)
	return tick
}

// endTick completes a tick and potentially publishes an epoch.
func (s *Shard) endTick() error {
	assert.That(len(s.ticks) > 0, "start tick wasn't called for this end tick")

	// Increment tick count at the end of the tick.
	logger := s.tel.GetLogger("shard")
	logger.Debug().Uint64("tick", s.tickHeight).Msg("tick completed")
	s.tickHeight++

	if len(s.ticks) == int(s.frequency) { //nolint:nestif // its ok
		if s.mode == ModeLeader {
			if err := s.publishEpoch(context.Background()); err != nil {
				return eris.Wrap(err, "failed to published epoch")
			}

			// Create snapshot asynchronously after successful epoch publishing, but only every snapshotFrequency epochs.
			if s.epochHeight%uint64(s.snapshotFrequency) == 0 {
				// Create snapshot metadata synchronously to avoid race conditions.
				stateHash, err := s.base.StateHash()
				if err != nil {
					return eris.Wrap(err, "failed to get state hash for snapshot")
				}
				snapshot := &Snapshot{
					EpochHeight: s.epochHeight,
					TickHeight:  s.tickHeight - 1,
					Timestamp:   timestamppb.Now(),
					StateHash:   stateHash,
					Data:        nil, // Will be filled in the goroutine
				}
				go s.createAndStoreSnapshot(snapshot)
			}
		}

		// Increment epoch count after publishing the epoch.
		logger.Debug().Uint64("epoch", s.epochHeight).Msg("epoch completed")
		s.epochHeight++

		// Clear ticks array to prepare for the next epoch.
		s.ticks = s.ticks[:0]
	}

	return nil
}

// publishEpoch serializes the current epoch and publishes it to the stream.
func (s *Shard) publishEpoch(ctx context.Context) error {
	stateHash, err := s.base.StateHash()
	if err != nil {
		return eris.Wrap(err, "failed to get state hash for epoch")
	}
	epoch := iscv1.Epoch{
		EpochHeight: s.epochHeight,
		Hash:        stateHash,
	}

	for _, tick := range s.ticks {
		tickPb := iscv1.Tick{
			Header: &iscv1.TickHeader{
				TickHeight: tick.Header.TickHeight,
				Timestamp:  timestamppb.New(tick.Header.Timestamp),
			},
			Data: &iscv1.TickData{},
		}

		for _, command := range tick.Data.Commands {
			commandPb := iscv1.Command{
				Signature: command.Signature,
				AuthInfo: &iscv1.AuthInfo{
					Mode:          command.AuthInfo.Mode,
					SignerAddress: command.AuthInfo.SignerAddress,
				},
				CommandBytes: command.CommandBytes,
			}
			tickPb.Data.Commands = append(tickPb.Data.Commands, &commandPb)
		}

		epoch.Ticks = append(epoch.Ticks, &tickPb)
	}

	payload, err := proto.Marshal(&epoch)
	if err != nil {
		return eris.Wrap(err, "failed to marshal epoch")
	}
	_, err = s.js.Publish(ctx, s.subject, payload, epochPublishOptions(s.subject, s.epochHeight)...)
	if err != nil {
		return eris.Wrap(err, "failed to publish epoch")
	}

	return nil
}

// createAndStoreSnapshot creates and stores a snapshot in a background goroutine.
// This is called after successful epoch publishing to avoid blocking the critical path.
// The snapshot metadata is already populated; this function fills in the data and stores it.
func (s *Shard) createAndStoreSnapshot(snapshot *Snapshot) {
	logger := s.tel.GetLogger("snapshot")

	engineData, err := s.base.Snapshot()
	if err != nil {
		logger.Error().Err(err).Msg("failed to create snapshot")
		return
	}
	snapshot.Data = engineData

	if err := s.snapshotStorage.Store(snapshot); err != nil {
		logger.Error().Err(err).Msg("failed to store snapshot")
		return
	}

	logger.Debug().
		Uint64("epoch", snapshot.EpochHeight).
		Uint64("tick", snapshot.TickHeight).
		Msg("snapshot created successfully")
}
