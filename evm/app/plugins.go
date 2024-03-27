package app

import (
	"fmt"
	"os"

	"cosmossdk.io/log"

	"pkg.world.dev/world-engine/evm/router"
	"pkg.world.dev/world-engine/evm/sequencer"
	"pkg.world.dev/world-engine/rift/credentials"
)

func (app *App) setPlugins(logger log.Logger) {
	routerKey := os.Getenv("ROUTER_KEY")
	var sequencerOpts []sequencer.Option
	var routerOpts []router.Option
	if routerKey == "" {
		app.Logger().Debug("WARNING: starting the EVM base shard in insecure mode. No ROUTER_KEY provided")
	} else {
		if err := credentials.ValidateKey(routerKey); err != nil {
			panic(fmt.Errorf("invalid ROUTER_KEY: %w", err))
		}
		sequencerOpts = append(sequencerOpts, sequencer.WithRouterKey(routerKey))
		routerOpts = append(routerOpts, router.WithRouterKey(routerKey))
	}
	app.ShardSequencer = sequencer.NewShardSequencer(sequencerOpts...)
	app.ShardSequencer.Serve()

	app.Router = router.NewRouter(logger, app.CreateQueryContext, app.NamespaceKeeper.Address, routerOpts...)
}
