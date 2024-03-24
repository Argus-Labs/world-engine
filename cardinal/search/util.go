package search

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

var NonFatalError = []error{
	gamestate.ErrEntityDoesNotExist,
	gamestate.ErrComponentNotOnEntity,
	gamestate.ErrComponentAlreadyOnEntity,
	gamestate.ErrEntityMustHaveAtLeastOneComponent,
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
