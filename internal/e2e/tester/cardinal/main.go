package main

import (
	"errors"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/query"
	"github.com/argus-labs/world-engine/example/tester/sys"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"log"
	"os"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/shard"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	namespace := os.Getenv("NAMESPACE")
	adapter := setupAdapter()
	world, err := cardinal.NewWorld(redisAddr, "",
		cardinal.WithNamespace(namespace),
		cardinal.WithReceiptHistorySize(10),
		cardinal.WithAdapter(adapter),
	)
	if err != nil {
		log.Fatal(err)
	}
	err = errors.Join(
		cardinal.RegisterComponent[comp.Location](world),
		cardinal.RegisterComponent[comp.Player](world),
	)
	if err != nil {
		log.Fatal(err)
	}
	err = cardinal.RegisterTransactions(world, tx.JoinTx, tx.MoveTx)
	if err != nil {
		log.Fatal(err)
	}
	err = cardinal.RegisterQueries(world, query.Location)
	if err != nil {
		log.Fatal(err)
	}
	cardinal.RegisterSystems(world, sys.Join, sys.Move)

	world.StartGame()
}

func setupAdapter() shard.Adapter {
	baseShardAddr := os.Getenv("BASE_SHARD_ADDR")
	shardReceiverAddr := os.Getenv("SHARD_SEQUENCER_ADDR")
	cfg := shard.AdapterConfig{
		ShardSequencerAddr: shardReceiverAddr,
		EVMBaseShardAddr:   baseShardAddr,
	}

	var opts []shard.Option
	clientCert := os.Getenv("CLIENT_CERT_PATH")
	if clientCert != "" {
		log.Print("running shard client with client certification")
		opts = append(opts, shard.WithCredentials(clientCert))
	} else {
		log.Print("WARNING: running shard client without client certification. this will cause issues if " +
			"the chain instance uses SSL credentials")
	}

	adapter, err := shard.NewAdapter(cfg, opts...)
	if err != nil {
		panic(err)
	}
	return adapter
}
