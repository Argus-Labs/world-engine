package cardinal

import (
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

var NonFatalError = []error{
	ErrEntityDoesNotExist,
	ErrComponentNotOnEntity,
	ErrComponentAlreadyOnEntity,
	ErrEntityMustHaveAtLeastOneComponent,
}

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

// panicOnFatalError is a helper function to panic on non-deterministic errors (i.e. Redis error).
func panicOnFatalError(wCtx engine.Context, err error) {
	if err != nil && !wCtx.IsReadOnly() && isFatalError(err) {
		wCtx.Logger().Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
		panic(err)
	}
}

func isFatalError(err error) bool {
	for _, e := range NonFatalError {
		if eris.Is(err, e) {
			return false
		}
	}
	return true
}
