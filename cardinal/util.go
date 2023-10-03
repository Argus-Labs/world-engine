package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs/options"
	"pkg.world.dev/world-engine/cardinal/public"
	"pkg.world.dev/world-engine/cardinal/server"
)

func toITransactionType(ins []AnyTransaction) []public.ITransaction {
	out := make([]public.ITransaction, 0, len(ins))
	for _, t := range ins {
		out = append(out, t.Convert())
	}
	return out
}

func toIReadType(ins []AnyReadType) []public.IRead {
	out := make([]public.IRead, 0, len(ins))
	for _, r := range ins {
		out = append(out, r.Convert())
	}
	return out
}

// separateOptions separates the given options into ecs options, server options, and cardinal (this package) options.
// The different options are all grouped together to simplify the end user's experience, but under the hood different
// options are meant for different sub-systems.
func separateOptions(opts []WorldOption) (ecsOptions []options.Option, serverOptions []server.Option, cardinalOptions []func(*World)) {
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
