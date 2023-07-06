package app

import (
	"os"
	"strconv"

	"github.com/argus-labs/world-engine/chain/router"
	"github.com/argus-labs/world-engine/chain/shard"
)

func (app *App) setPlugins() {
	// setup the game shard listener.
	// TODO: clean this up
	useShardListenerStr := os.Getenv("USE_SHARD_LISTENER")
	useShardListener, err := strconv.ParseBool(useShardListenerStr)
	if err != nil {
		panic(err)
	}
	if useShardListener {
		app.ShardHandler = shard.NewShardServer()
		app.ShardHandler.Serve(os.Getenv("SHARD_HANDLER_LISTEN_ADDR"))
	}

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		rtr, err := router.NewRouter(cardinalShardAddr)
		if err != nil {
			panic(err)
		}
		app.Router = rtr
	}
}
