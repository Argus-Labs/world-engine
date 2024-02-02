package main //nolint: cyclop // for tests.

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
		systems = append(systems, sys.SystemTestGetSmallComponentA)
	}
	if b {
		systems = append(systems, sys.SystemTestGetSmallComponentB)
	}
	if c {
		systems = append(systems, sys.SystemTestSearchC)
	}
	if d {
		systems = append(systems, sys.SystemTestGetComponentWithArrayD)
	}
	if e {
		systems = append(systems, sys.SystemTestGetAndSetComponentWithArrayE)
	}
	if f {
		systems = append(systems, sys.SystemTestSearchingForCompWithArrayF)
	}
	if g {
		systems = append(systems, sys.SystemTestEntityCreationG)
	}
	if h {
		systems = append(systems, sys.SystemTestGettingHighlyNestedComponentsH)
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
