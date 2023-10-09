package main

import (
	"context"
	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/read"
	"github.com/argus-labs/world-engine/example/tester/sys"
	"github.com/argus-labs/world-engine/example/tester/tx"
	"log"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/evm"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/shard"
	"time"
)

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	namespace := os.Getenv("NAMESPACE")
	rs := storage.NewRedisStorage(storage.Options{Addr: redisAddr}, namespace)
	store := storage.NewWorldStorage(&rs)
	adapter := setupAdapter()
	world, err := ecs.NewWorld(
		store,
		ecs.WithNamespace(namespace),
		ecs.WithReceiptHistorySize(10),
		ecs.WithAdapter(adapter),
	)
	if err != nil {
		log.Fatal(err)
	}
	err = world.RegisterComponents(comp.LocationComponent, comp.PlayerComponent)
	if err != nil {
		log.Fatal(err)
	}
	err = world.RegisterTransactions(tx.MoveTx, tx.JoinTx)
	if err != nil {
		log.Fatal(err)
	}
	err = world.RegisterReads(read.Location)
	if err != nil {
		log.Fatal(err)
	}
	world.AddSystems(sys.Join, sys.Move)
	err = world.LoadGameState()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	srvr, err := server.NewHandler(world, server.WithAdapter(adapter))
	if err != nil {
		log.Fatal(err)
	}
	world.StartGameLoop(ctx, time.Second*1)
	go srvr.Serve()
	evmServer, err := evm.NewServer(world)
	if err != nil {
		log.Fatal(err)
	}
	err = evmServer.Serve()
	if err != nil {
		panic(err)
	}
	select {}
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
