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
		cardinal.RegisterComponent[comp.Tree](world),
	)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}

	a := *testFlag[testKeys[0]]
	b := *testFlag[testKeys[1]]
	c := *testFlag[testKeys[2]]
	d := *testFlag[testKeys[3]]
	e := *testFlag[testKeys[4]]
	f := *testFlag[testKeys[5]]
	g := *testFlag[testKeys[6]]
	h := *testFlag[testKeys[7]]
	if a || b || c || d {
		initsystems = append(initsystems, sys.InitTenThousandEntities)
	}
	if d || e || f || g || h {
		initsystems = append(initsystems, sys.InitOneHundredEntities)
	}
	if h {
		initsystems = append(initsystems, sys.InitTreeEntities)
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

	err = cardinal.RegisterSystems(world, systems...)
	sys.ShutdownFunc = sys.CreateShutDownFunc(world)
	if err != nil {
		log.Fatal(err, eris.ToString(err, true))
	}
	world.Init(sumSystems(initsystems...))
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
