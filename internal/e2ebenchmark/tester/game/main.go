package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"runtime/pprof"

	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/example/tester_benchmark/comp"
	"github.com/argus-labs/world-engine/example/tester_benchmark/sys"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/shard"
)

func main() {
	//This code is a bit redundant will change.
	var testFlag = make(map[string]*bool)
	var testKeys = []string{"a", "b", "c", "d", "e", "f", "g"}
	for _, key := range testKeys {
		testFlag[key] = flag.Bool(key, false, "flag "+key)
	}
	flag.Parse()
	filename := "cpu.prof"
	prefix := ""
	for _, key := range testKeys {
		if *testFlag[key] {
			prefix += key + "_"
		}
	}
	filename = prefix + filename
	profileFile, err := os.Create("/profiles/" + filename)
	if err != nil {
		log.Fatal(err)
	}
	defer profileFile.Close()

	if err := pprof.StartCPUProfile(profileFile); err != nil {
		log.Fatal("could not start CPU profile: ", err)
	}
	defer pprof.StopCPUProfile()

	sumSystems := func(systems ...cardinal.System) cardinal.System {
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
		log.Fatal(err, eris.ToString(err, true))
	}
	err = errors.Join(
		cardinal.RegisterComponent[comp.ArrayComp](world),
		cardinal.RegisterComponent[comp.SingleNumber](world),
	)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	//err = cardinal.RegisterMessages(world, msg.JoinMsg, msg.MoveMsg)
	//if err != nil {
	//	log.Fatal(err, eris.ToString(err, true))
	//}
	//err = query.RegisterLocationQuery(world)
	//if err != nil {
	//	log.Fatal(err, eris.ToString(err, true))
	//}

	a := *testFlag[testKeys[0]]
	b := *testFlag[testKeys[1]]
	c := *testFlag[testKeys[2]]
	d := *testFlag[testKeys[3]]
	e := *testFlag[testKeys[4]]
	f := *testFlag[testKeys[5]]
	g := *testFlag[testKeys[6]]
	if a || b || c || d {
		//panic("INIT TEN THOUSAND ENTITIES")
		initsystems = append(initsystems, sys.InitTenThousandEntities)
	}
	if d || e || f || g {
		initsystems = append(initsystems, sys.InitOneHundredEntities)
	}
	if a {
		systems = append(systems, sys.SystemA)
	}
	if b {
		systems = append(systems, sys.SystemB)
	}
	if c {
		systems = append(systems, sys.SystemC)
	}
	if d {
		systems = append(systems, sys.SystemD)
	}
	if e {
		systems = append(systems, sys.SystemE)
	}
	if f {
		systems = append(systems, sys.SystemF)
	}
	if g {
		systems = append(systems, sys.SystemG)
	}

	err = cardinal.RegisterSystems(world, systems...)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	world.Init(sumSystems(initsystems...))
	log.Print("STARRRRTING GAME!!!!!!!!")
	err = world.StartGame()
	log.Print("ENDING GAME!!!!!!!!!!!!!!!!!!!")
	log.Fatal(eris.Errorf("blah"), "ERRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRR!!!!!!!!!!!!")
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
