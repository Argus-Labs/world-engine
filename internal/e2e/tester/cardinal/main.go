package main

import (
	"errors"
	"log"
	"os"

	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/query"
	"github.com/argus-labs/world-engine/example/tester/sys"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/shard"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	namespace := os.Getenv("NAMESPACE")
	options := []cardinal.WorldOption{
		cardinal.WithNamespace(namespace),
		cardinal.WithReceiptHistorySize(10), //nolint:gomnd // fine for testing.
	}
	if os.Getenv("ENABLE_ADAPTER") == "false" {
		log.Println("Skipping adapter")
	} else {
		options = append(options, cardinal.WithAdapter(setupAdapter()))
	}

	world, err := cardinal.NewWorld(redisAddr, "", options...)
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

	err = world.StartGame()
	if err != nil {
		log.Fatal(err)
	}
}

func setupAdapter() shard.Adapter {
	baseShardAddr := os.Getenv("BASE_SHARD_ADDR")
	shardReceiverAddr := os.Getenv("SHARD_SEQUENCER_ADDR")
	cfg := shard.AdapterConfig{
		ShardSequencerAddr: shardReceiverAddr,
		EVMBaseShardAddr:   baseShardAddr,
	}
	adapter, err := shard.NewAdapter(cfg)
	if err != nil {
		panic(err)
	}
	return adapter
}
