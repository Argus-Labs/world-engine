package app

import (
	"cosmossdk.io/log"

	"pkg.world.dev/world-engine/evm/router"
	"pkg.world.dev/world-engine/evm/sequencer"
)

func (app *App) setPlugins(logger log.Logger) {

	app.ShardSequencer = sequencer.NewShardSequencer()
	app.ShardSequencer.Serve()
	app.Router = router.NewRouter(logger, app.CreateQueryContext, app.NamespaceKeeper.Address)
}
