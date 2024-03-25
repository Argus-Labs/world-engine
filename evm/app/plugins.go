package app

import (
	"os"

	"cosmossdk.io/log"

	"pkg.world.dev/world-engine/evm/router"
	"pkg.world.dev/world-engine/evm/sequencer"
)

func (app *App) setPlugins(logger log.Logger) {
	key := os.Getenv("SECRET_KEY")
	var sequencerOpts []sequencer.Option
	var routerOpts []router.Option
	if key == "" {
		app.Logger().Debug("WARNING: starting the EVM base shard in insecure mode. No SECRET_KEY provided")
	} else {
		sequencerOpts = append(sequencerOpts, sequencer.WithRouterKey(key))
		routerOpts = append(routerOpts, router.WithRouterKey(key))
	}
	app.ShardSequencer = sequencer.NewShardSequencer(sequencerOpts...)
	app.ShardSequencer.Serve()

	app.Router = router.NewRouter(logger, app.CreateQueryContext, app.NamespaceKeeper.Address, routerOpts...)
}
