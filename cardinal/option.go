package cardinal

import (
	"time"

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
		ecsOption: ecs.WithAdapter(adapter),
	}
}

// WithReceiptHistorySize specifies how many ticks worth of transaction receipts should be kept in memory. The default
// is 10. A smaller number uses less memory, but limits the
func WithReceiptHistorySize(size int) WorldOption {
	return WorldOption{
		ecsOption: ecs.WithReceiptHistorySize(size),
	}
}

// WithNamespace sets the World's namespace. The default is "world". The namespace is used in the transaction
// signing process.
func WithNamespace(namespace string) WorldOption {
	return WorldOption{
		ecsOption: ecs.WithNamespace(namespace),
	}
}

// WithPort specifies the port for the World's HTTP server. If omitted, the environment variable CARDINAL_PORT
// will be used, and if that is unset, port 4040 will be used.
func WithPort(port string) WorldOption {
	return WorldOption{
		serverOption: server.WithPort(port),
	}
}

// WithDisableSignatureVerification disables signature verification for the HTTP server. This should only be
// used for local development.
func WithDisableSignatureVerification() WorldOption {
	return WorldOption{
		serverOption: server.DisableSignatureVerification(),
	}
}

// WithLoopInterval sets the time between ticks. If left unset, 1 second is used as a default.
func WithLoopInterval(interval time.Duration) WorldOption {
	return WorldOption{
		cardinalOption: func(world *World) {
			world.loopInterval = interval
		},
	}
}

func WithPrettyLog() WorldOption {
	return WorldOption{
		ecsOption: ecs.WithPrettyLog(),
	}
}
