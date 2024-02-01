package main

import (
	"errors"
	"log"
	"os"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/shard/adapter"

	"github.com/argus-labs/world-engine/example/tester/game/comp"
	"github.com/argus-labs/world-engine/example/tester/game/msg"
	"github.com/argus-labs/world-engine/example/tester/game/query"
	"github.com/argus-labs/world-engine/example/tester/game/sys"
	"github.com/rotisserie/eris"
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

func setupAdapter() adapter.Adapter {
	baseShardAddr := os.Getenv("BASE_SHARD_ADDR")
	shardReceiverAddr := os.Getenv("SHARD_SEQUENCER_ADDR")
	cfg := adapter.Config{
		ShardSequencerAddr: shardReceiverAddr,
		EVMBaseShardAddr:   baseShardAddr,
	}

	adpter, err := adapter.New(cfg)
	if err != nil {
		panic(err)
	}
	return adpter
}
