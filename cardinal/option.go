package cardinal

import (
	"os"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/router"
	"pkg.world.dev/world-engine/cardinal/server"
)

// WorldOption represents an option that can be used to augment how the cardinal.World will be run.
type WorldOption struct {
	serverOption   server.Option
	routerOption   router.Option
	cardinalOption Option
}

type Option func(*World)

// WithPort sets the port that the HTTP server will run on.
func WithPort(port string) WorldOption {
	return WorldOption{
		serverOption: server.WithPort(port),
	}
}

// WithReceiptHistorySize specifies how many ticks worth of transaction receipts should be kept in memory. The default
// is 10. A smaller number uses less memory, but limits the amount of historical receipts available.
func WithReceiptHistorySize(size int) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.receiptHistory = receipt.NewHistory(world.CurrentTick(), size)
		},
	}
}

// WithDisableSignatureVerification disables signature verification for the HTTP server. This should only be
// used for local development.
func WithDisableSignatureVerification() WorldOption {
	return WorldOption{
		serverOption: server.DisableSignatureVerification(),
	}
}

// WithMessageExpiration How long messages will live past their creation
// time on the sender before they are considered to be expired and will
// not be processed. Default is 10 seconds.
// For longer expiration times you may also need to set a larger hash cache
// size using the WithHashCacheSize option
// This setting is ignored if the DisableSignatureVerification option is used
// NOTE: this means that the real time clock for the sender and receiver
// must be synchronized
func WithMessageExpiration(seconds int) WorldOption {
	return WorldOption{
		serverOption: server.WithMessageExpiration(seconds),
	}
}

// WithTickChannel sets the channel that will be used to decide when world.doTick is executed. If unset, a loop interval
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

func WithStoreManager(s gamestate.Manager) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.entityStore = s
		},
	}
}

// WithMockRedis runs the World with an embedded miniredis instance on port 6379.
func WithMockRedis() WorldOption {
	// Start a miniredis instance on port 6379.
	mr := miniredis.NewMiniRedis()
	err := mr.StartAddr(":6379")
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start miniredis")
	}
	log.Debug().Msgf("miniredis started at %s", mr.Addr())

	// Set the REDIS_ADDRESS environment variable to the miniredis address.
	err = os.Setenv("REDIS_ADDRESS", mr.Addr())
	if err != nil {
		log.Fatal().Err(err).Msg("unable to set REDIS_ADDRESS")
	}

	return WorldOption{}
}

// WithMockJobQueue runs the router with an in-memory job queue instead of a persistent one that writes to disk.
func WithMockJobQueue() WorldOption {
	return WorldOption{
		routerOption: router.WithMockJobQueue(),
	}
}

func WithCustomLogger(logger zerolog.Logger) WorldOption {
	return WorldOption{
		cardinalOption: func(_ *World) {
			log.Logger = logger
		},
	}
}

func WithCustomRouter(rtr router.Router) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.router = rtr
		},
	}
}

func WithPrettyLog() WorldOption {
	return WorldOption{
		cardinalOption: func(_ *World) {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		},
	}
}
