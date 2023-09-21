package app

import (
	"github.com/rs/zerolog/log"
	"os"
	"pkg.world.dev/world-engine/chain/router"
	"pkg.world.dev/world-engine/chain/shard"
)

func (app *App) setPlugins() {
	// TODO: clean this up. maybe a config?
	certPath := os.Getenv("SERVER_CERT_PATH")
	keyPath := os.Getenv("SERVER_KEY_PATH")
	var opt shard.Option
	if certPath == "" && keyPath == "" {
		log.Warn().Msg("running shard sequencer without SSL certs")
	} else {
		opt = shard.WithCredentials(certPath, keyPath)
	}
	app.ShardSequencer = shard.NewShardSequencer(opt)
	app.ShardSequencer.Serve()

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		clientCert := os.Getenv("CLIENT_CERT_PATH")
		app.Router = router.NewRouter(cardinalShardAddr, router.WithCredentials(clientCert))
	} else {
		log.Warn().Msg("router is not running")
	}
}
