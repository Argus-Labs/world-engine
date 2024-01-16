package app

import (
	"os"
	"pkg.world.dev/world-engine/evm/sequencer"

	"cosmossdk.io/log"
	"pkg.world.dev/world-engine/evm/router"
)

func (app *App) setPlugins(logger log.Logger) {
	certPath := os.Getenv("SERVER_CERT_PATH")
	keyPath := os.Getenv("SERVER_KEY_PATH")
	if certPath == "" || keyPath == "" {
		logger.Info("running shard sequencer without SSL certs")
		app.ShardSequencer = sequencer.NewShardSequencer()
	} else {
		logger.Info("running shard sequencer with SSL certs")
		app.ShardSequencer = sequencer.NewShardSequencer(sequencer.WithCredentials(certPath, keyPath))
	}

	app.ShardSequencer.Serve()

	var opts []router.Option
	clientCert := os.Getenv("CLIENT_CERT_PATH")
	if clientCert != "" {
		logger.Info("running router with client certification")
		opts = append(opts, router.WithCredentials(clientCert))
	} else {
		logger.Error("WARNING: running router client without client certification. this will cause issues if " +
			"the game shard instance uses SSL credentials")
	}
	app.Router = router.NewRouter(logger, app.CreateQueryContext, app.NamespaceKeeper.Address, opts...)
}
