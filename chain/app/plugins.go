package app

import (
	"os"
	"pkg.world.dev/world-engine/chain/router"
	"pkg.world.dev/world-engine/chain/shard"

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
		app.ShardSequencer = shard.NewShardSequencer(shard.WithCredentials(certPath, keyPath))
	}

	app.ShardSequencer.Serve()

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		clientCert := os.Getenv("CLIENT_CERT_PATH")
		app.Router = router.NewRouter(cardinalShardAddr, logger, router.WithCredentials(clientCert))
	} else {
		logger.Info("router is not running")
	}
}
