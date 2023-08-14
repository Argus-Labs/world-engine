package app

import (
	"os"
	"pkg.world.dev/world-engine/chain/router"
	"pkg.world.dev/world-engine/chain/shard"
)

func (app *App) setPlugins() {
	// TODO: clean this up. maybe a config?
	shardHandlerListener := os.Getenv("SHARD_HANDLER_LISTEN_ADDR")
	if shardHandlerListener != "" {
		certPath := os.Getenv("SERVER_CERT_PATH")
		keyPath := os.Getenv("SERVER_KEY_PATH")
		app.ShardHandler = shard.NewShardServer(shard.WithCredentials(certPath, keyPath))
		app.ShardHandler.Serve(shardHandlerListener)
	}

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		clientCert := os.Getenv("CLIENT_CERT_PATH")
		app.Router = router.NewRouter(cardinalShardAddr, router.WithCredentials(clientCert))
	}
}
