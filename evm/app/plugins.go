package app

import (
	"os"

	"pkg.world.dev/world-engine/evm/router"
	"pkg.world.dev/world-engine/evm/shard"

	"cosmossdk.io/log"
)

func (app *App) setPlugins(logger log.Logger) {
	// TODO: clean this up. maybe a config?
	certPath := os.Getenv("SERVER_CERT_PATH")
	keyPath := os.Getenv("SERVER_KEY_PATH")
	if certPath == "" || keyPath == "" {
		logger.Info("running shard sequencer without SSL certs")
		app.ShardSequencer = shard.NewShardSequencer()
	} else {
		logger.Info("running shard sequencer with SSL certs")
		app.ShardSequencer = shard.NewShardSequencer(shard.WithCredentials(certPath, keyPath))
	}

	app.ShardSequencer.Serve()

	// TODO: we dont need cardinal addr anymore. we're gonna get it from state machine.
	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		var opts []router.Option
		clientCert := os.Getenv("CLIENT_CERT_PATH")
		if clientCert != "" {
			logger.Info("running router client with client certification")
			opts = append(opts, router.WithCredentials(clientCert))
		} else {
			logger.Info("WARNING: running router client without client certification. this will cause issues if " +
				"the cardinal instance uses SSL credentials")
		}
		app.Router = router.NewRouter(logger, app.CreateQueryContext, app.NamespaceKeeper.Address, opts...)
	} else {
		logger.Info("router is not running")
	}
}
