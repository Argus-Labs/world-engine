package main

import (
	"errors"
	"log"
	"os"

	"github.com/argus-labs/world-engine/example/tester/msg"
	"github.com/argus-labs/world-engine/example/tester/query"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/example/tester/comp"
	"github.com/argus-labs/world-engine/example/tester/sys"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/shard"
)

func main() {
	options := []cardinal.WorldOption{
		cardinal.WithReceiptHistorySize(10), //nolint:gomnd // fine for testing.
	}

	options = append(options, cardinal.WithAdapter(setupAdapter()))

	world, err := cardinal.NewWorld(options...)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = errors.Join(
		cardinal.RegisterComponent[comp.Location](world),
		cardinal.RegisterComponent[comp.Player](world),
	)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = cardinal.RegisterMessages(world, msg.JoinMsg, msg.MoveMsg)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = query.RegisterLocationQuery(world)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	err = cardinal.RegisterSystems(world, sys.Join, sys.Move)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	err = world.StartGame()
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
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
