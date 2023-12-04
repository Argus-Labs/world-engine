package cardinal

import (
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog/log"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/events"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/shard"
)

// WorldOption represents an option that can be used to augment how the cardinal.World will be run.
type WorldOption struct {
	ecsOption      ecs.Option
	serverOption   server.Option
	cardinalOption func(*World)
}

// WithAdapter provides the world with communicate channels to the EVM base shard, enabling transaction storage and
// transaction retrieval for state rebuilding purposes.
func WithAdapter(adapter shard.Adapter) WorldOption {
	return WorldOption{
		ecsOption:    ecs.WithAdapter(adapter),
		serverOption: server.WithAdapter(adapter),
	}
}

// WithReceiptHistorySize specifies how many ticks worth of transaction receipts should be kept in memory. The default
// is 10. A smaller number uses less memory, but limits the amount of historical receipts available.
func WithReceiptHistorySize(size int) WorldOption {
	return WorldOption{
		ecsOption: ecs.WithReceiptHistorySize(size),
	}
}

// WithDisableSignatureVerification disables signature verification for the HTTP server. This should only be
// used for local development.
func WithDisableSignatureVerification() WorldOption {
	return WorldOption{
		serverOption: server.DisableSignatureVerification(),
	}
}

// WithTickChannel sets the channel that will be used to decide when world.Tick is executed. If unset, a loop interval
// of 1 second will be set. To set some other time, use: WithTickChannel(time.Tick(<some-duration>)). Tests can pass
// in a channel controlled by the test for fine-grained control over when ticks are executed.
func WithTickChannel(ch <-chan time.Time) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.tickChannel = ch
		},
	}
}

// WithTickDoneChannel sets a channel that will be notified each time a tick completes. The completed tick will be
// pushed to the channel. This option is useful in tests when assertions need to be performed at the end of a tick.
func WithTickDoneChannel(ch chan<- uint64) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.tickDoneChannel = ch
		},
	}
}

func WithStoreManager(s store.IManager) WorldOption {
	return WorldOption{
		ecsOption: ecs.WithStoreManager(s),
	}
}

func WithEventHub(eventHub events.EventHub) WorldOption {
	return WorldOption{
		ecsOption: ecs.WithEventHub(eventHub),
	}
}

func WithLoggingEventHub(logger *ecslog.Logger) WorldOption {
	return WorldOption{
		ecsOption: ecs.WithLoggingEventHub(logger),
	}
}

func withMockRedis() WorldOption {
	// We manually set the start address to make the port deterministic
	s := miniredis.NewMiniRedis()
	err := s.StartAddr(":6379")
	if err != nil {
		panic(
			"Unable to start miniredis. Make sure there is no other redis instance running on port 6379",
		)
	}
	log.Logger.Debug().Msgf("miniredis started at %s", s.Addr())

	return WithCustomMockRedis(s)
}

func WithCustomMockRedis(miniRedis *miniredis.Miniredis) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.cleanup = func() {
				log.Logger.Debug().Msg("miniredis shutting down")
				miniRedis.Close()
				log.Logger.Debug().Msg("miniredis shutdown successful")
			}
		},
	}
}
