package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/server"
)

// separateOptions separates the given options into ecs options, server options, and cardinal (this package) options.
// The different options are all grouped together to simplify the end user's experience, but under the hood different
// options are meant for different sub-systems.
func separateOptions(opts []WorldOption) (
	serverOptions []server.Option,
	cardinalOptions []Option,
) {
	for _, opt := range opts {
		if opt.serverOption != nil {
			serverOptions = append(serverOptions, opt.serverOption)
		}
		if opt.cardinalOption != nil {
			cardinalOptions = append(cardinalOptions, opt.cardinalOption)
		}
	}
	return serverOptions, cardinalOptions
}
