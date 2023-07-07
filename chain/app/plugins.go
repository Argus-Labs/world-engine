package app

import (
	"fmt"
	"github.com/argus-labs/world-engine/chain/router"
	"github.com/argus-labs/world-engine/chain/shard"
	"os"
)

func (app *App) setPlugins() {
	// TODO: clean this up. maybe a config?
	shardHandlerListener := os.Getenv("SHARD_HANDLER_LISTEN_ADDR")
	shardHandlerListener = "localhost:9329"
	if shardHandlerListener != "" {
		fmt.Println("starting shard listener...")
		app.ShardHandler = shard.NewShardServer()
		app.ShardHandler.Serve(shardHandlerListener)
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
