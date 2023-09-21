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
	if certPath == "" || keyPath == "" {
		log.Warn().Msg("running shard sequencer without SSL certs")
		app.ShardSequencer = shard.NewShardSequencer()
	} else {
		app.ShardSequencer = shard.NewShardSequencer(shard.WithCredentials(certPath, keyPath))
	}

	app.ShardSequencer.Serve()

	cardinalShardAddr := os.Getenv("CARDINAL_EVM_LISTENER_ADDR")
	if cardinalShardAddr != "" {
		clientCert := os.Getenv("CLIENT_CERT_PATH")
		app.Router = router.NewRouter(cardinalShardAddr, router.WithCredentials(clientCert))
	} else {
		log.Warn().Msg("router is not running")
	}
}
