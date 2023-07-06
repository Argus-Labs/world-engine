package app

import (
	"github.com/argus-labs/world-engine/chain/router"
	"github.com/argus-labs/world-engine/chain/shard"
	"os"
)

func (app *App) setPlugins() {
	// setup the game shard listener.
	// TODO: clean this up. maybe a config?
	shardHandlerListener := os.Getenv("SHARD_HANDLER_LISTEN_ADDR")
	if shardHandlerListener != "" {
		app.ShardHandler = shard.NewShardServer()
		app.ShardHandler.Serve(shardHandlerListener)
	}

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		rtr := router.NewRouter()
		app.Router = rtr
	}
}
