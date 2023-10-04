package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/itransaction"
	"pkg.world.dev/world-engine/cardinal/server"
)

func toITransactionType(ins []AnyTransaction) []itransaction.ITransaction {
	out := make([]itransaction.ITransaction, 0, len(ins))
	for _, t := range ins {
		out = append(out, t.Convert())
	}
	return out
}

func toIReadType(ins []AnyReadType) []ecs.IRead {
	out := make([]ecs.IRead, 0, len(ins))
	for _, r := range ins {
		out = append(out, r.Convert())
	}
	return out
}

// separateOptions separates the given options into ecs options, server options, and cardinal (this package) options.
// The different options are all grouped together to simplify the end user's experience, but under the hood different
// options are meant for different sub-systems.
func separateOptions(opts []WorldOption) (ecsOptions []ecs.Option, serverOptions []server.Option, cardinalOptions []func(*World)) {
	for _, opt := range opts {
		if opt.ecsOption != nil {
			ecsOptions = append(ecsOptions, opt.ecsOption)
		} else if opt.serverOption != nil {
			serverOptions = append(serverOptions, opt.serverOption)
		} else if opt.cardinalOption != nil {
			cardinalOptions = append(cardinalOptions, opt.cardinalOption)
		}
	}
	return ecsOptions, serverOptions, cardinalOptions
}
