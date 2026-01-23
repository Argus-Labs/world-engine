package cardinal

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/cardinal/epoch"
	"github.com/rotisserie/eris"
)

// runLeader executes the leader mode main loop.
// It processes ticks at the configured tick rate and publishes epochs to followers.
func (w *World) runLeader(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(float64(time.Second) / w.options.TickRate))
	defer ticker.Stop()

	// TODO: select from debug channel to pause/play ticks.
	for {
		select {
		case <-ticker.C:
			if err := w.Tick(time.Now()); err != nil {
				return eris.Wrap(err, "failed to run tick")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (w *World) Tick(timestamp time.Time) error {
	assert.That(len(w.ticks) < int(w.options.EpochFrequency), "last epoch is not submitted")

	commands := w.commands.Drain()

	// Append to ticks slice.
	tick := epoch.Tick{
		Header: epoch.TickHeader{
			TickHeight: w.tickHeight,
			Timestamp:  timestamp,
		},
		Data: epoch.TickData{Commands: commands},
	}
	w.ticks = append(w.ticks, tick)

	// Tick ECS world.
	events, err := w.world.Tick(tick.Data.Commands)
	if err != nil {
		return eris.Wrap(err, "one or more systems failed")
	}

	// Increment tick height.
	w.tickHeight++

	// Emit events.
	w.service.Publish(events)

	data, _ := w.world.Serialize()
	hash := sha256.Sum256(data)

	// Publish epoch.
	if len(w.ticks) == int(w.options.EpochFrequency) {
		if w.options.Mode == ModeLeader {
			epoch := epoch.Epoch{
				EpochHeight: w.epochHeight,
				Hash:        hash[:],
			}
			if err := w.epochLog.Publish(context.Background(), epoch); err != nil {
				return eris.Wrap(err, "failed to published epoch")
			}

			// Publish snapshot.
			if w.epochHeight%uint64(w.options.SnapshotFrequency) == 0 {
				// snapshot := &micro.Snapshot{
				// 	EpochHeight: w.epochHeight,
				// 	TickHeight:  w.tickHeight - 1,
				// 	Timestamp:   timestamppb.New(timestamp),
				// 	StateHash:   hash[:],
				// 	Data:        nil, // Will be filled in the goroutine
				// }
				// if err := w.snapshots.Store(snapshot); err != nil {
				// 	return eris.Wrap(err, "failed to published snapshot")
				// }
			}
		}

		// Increment epoch count after publishing the epoch.
		w.epochHeight++

		// Clear ticks array to prepare for the next epoch.
		w.ticks = w.ticks[:0]
	}

	return nil
}
