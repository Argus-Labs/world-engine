package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"runtime/pprof"

	"github.com/argus-labs/world-engine/example/tester_benchmark/sys"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/example/tester_benchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/shard/adapter"
)

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
	testFlag map[string]*bool,
	testKeys []string,
	initSystems []cardinal.System,
	systems []cardinal.System) ([]cardinal.System, []cardinal.System) {
	a := *testFlag[testKeys[0]]
	b := *testFlag[testKeys[1]]
	c := *testFlag[testKeys[2]]
	d := *testFlag[testKeys[3]]
	e := *testFlag[testKeys[4]]
	f := *testFlag[testKeys[5]]
	g := *testFlag[testKeys[6]]
	h := *testFlag[testKeys[7]]
	if a || b || c || d {
		initSystems = append(initSystems, sys.InitTenThousandEntities)
	}
	if d || e || f || g || h {
		initSystems = append(initSystems, sys.InitOneHundredEntities)
	}
	if h {
		initSystems = append(initSystems, sys.InitTreeEntities)
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
	if h {
		systems = append(systems, sys.SystemH)
	}
	return initSystems, systems
}

func main() {
	// This code is a bit redundant will change.
	var testFlag = make(map[string]*bool)
	var testKeys = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
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

	initsystems, systems = initializeSystems(testFlag, testKeys, initsystems, systems)

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

	var opts []adapter.Option
	clientCert := os.Getenv("CLIENT_CERT_PATH")
	if clientCert != "" {
		log.Print("running shard client with client certification")
		opts = append(opts, adapter.WithCredentials(clientCert))
	} else {
		log.Print("WARNING: running shard client without client certification. this will cause issues if " +
			"the chain instance uses SSL credentials")
	}

	adapter, err := adapter.New(cfg, opts...)
	if err != nil {
		panic(err)
	}
	return adapter
}
