package main

import (
	"errors"
	"log"
	"os"
	"runtime/pprof"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/sys"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/shard/adapter"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
)

const TicksUntilTermination = 180

func sumSystems(systems ...cardinal.System) cardinal.System {
	return func(wCtx cardinal.WorldContext) error {
		for _, system := range systems {
			err := system(wCtx)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func initializeSystems(
	initSystems []cardinal.System,
	systems []cardinal.System) ([]cardinal.System, []cardinal.System) {
	initSystems = append(initSystems, sys.InitOneHundredEntities)
	initSystems = append(initSystems, sys.InitTreeEntities)
	decoratedSystemH := sys.ProfilerTerminatorDecoratorForSystem(
		sys.SystemBenchmarkTest, TicksUntilTermination)
	systems = append(systems, decoratedSystemH)
	return initSystems, systems
}

func main() {
	// This code is a bit redundant will change.
	filename := "cpu.prof"
	fullFilename := "/profiles/" + filename
	profileFile, err := os.Create(fullFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer profileFile.Close()

	if err := pprof.StartCPUProfile(profileFile); err != nil {
		panic("could not start CPU profile: " + err.Error())
	}
	defer pprof.StopCPUProfile()
	pprof.StopCPUProfile()

	initsystems := []cardinal.System{}
	systems := []cardinal.System{}

	options := []cardinal.WorldOption{
		cardinal.WithReceiptHistorySize(10), //nolint:gomnd // fine for testing.
	}
	// if os.Getenv("ENABLE_ADAPTER") == "false" {
	if true { // uncomment above to enable adapter from env.
		log.Println("Skipping adapter")
	} else {
		options = append(options, cardinal.WithAdapter(setupAdapter()))
	}
	world, err := cardinal.NewWorld(options...)
	if err != nil {
		panic(eris.ToString(err, true))
	}
	err = errors.Join(
		cardinal.RegisterComponent[comp.ArrayComp](world),
		cardinal.RegisterComponent[comp.SingleNumber](world),
		cardinal.RegisterComponent[comp.Tree](world),
	)
	if err != nil {
		panic(eris.ToString(err, true))
	}

	initsystems, systems = initializeSystems(initsystems, systems)

	err = cardinal.RegisterSystems(world, systems...)
	if err != nil {
		panic(eris.ToString(err, true))
	}
	world.Init(sumSystems(initsystems...))
	err = world.StartGame()
	if err != nil {
		panic(eris.ToString(err, true))
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
