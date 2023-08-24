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
		app.Logger().Info("shard handler served at %s", shardHandlerListener)
	} else {
		app.Logger().Info("SHARD_HANDLER_LISTEN_ADDR not specified, skipping shard handler setup")
	}

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR") // Cardinal's EVM support server address
	if cardinalShardAddr != "" {
		clientCert := os.Getenv("CLIENT_CERT_PATH")
		app.Router = router.NewRouter(cardinalShardAddr, router.WithCredentials(clientCert))
		app.Logger().Info("Router is ready to route messages from EVM base shard to game shards")
	} else {
		app.Logger().Info("CARDINAL_EVM_LISTENER_ADDR not specified, skipping router setup")
	}
}
