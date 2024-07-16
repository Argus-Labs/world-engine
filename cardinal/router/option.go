package router

import (
	"github.com/argus-labs/go-jobqueue"

	shard "pkg.world.dev/world-engine/rift/shard/v2"
)

type Option func(*router)

// WithMockJobQueue runs the router with an in-memory job queue instead of a persistent one that writes to disk.
func WithMockJobQueue() Option {
	return func(rtr *router) {
		sequencerJobQueue, err := jobqueue.New[*shard.SubmitTransactionsRequest](
			"",
			"submit-tx",
			20, //nolint:gomnd // Will do this later
			handleSubmitTx(rtr.ShardSequencer, rtr.tracer),
			jobqueue.WithInmemDB[*shard.SubmitTransactionsRequest](),
		)
		if err != nil {
			panic(err)
		}
		rtr.sequencerJobQueue = sequencerJobQueue
	}
}
