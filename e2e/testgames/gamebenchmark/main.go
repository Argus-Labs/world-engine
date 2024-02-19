package main

import (
	"errors"
	"log"
	"os"
	"pkg.world.dev/world-engine/cardinal/system"
	"runtime/pprof"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/sys"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
)

func initializeSystems(
	initSystems []cardinal.System,
	systems []cardinal.System) ([]cardinal.System, []cardinal.System) {
	initSystems = append(initSystems, sys.InitTenThousandEntities)
	initSystems = append(initSystems, sys.InitOneHundredEntities)
	initSystems = append(initSystems, sys.InitTreeEntities)
	systems = append(systems, sys.SystemBenchmark)
	return initSystems, systems
}

func main() {
	// This code is a bit redundant will change.
	filename := "cpu.prof"
	folder := "/profiles/"
	fullFilename := folder + filename
	profileFile, err := os.Create(fullFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer profileFile.Close()

	if err := pprof.StartCPUProfile(profileFile); err != nil {
		panic("could not start CPU profile: " + err.Error())
	}
	defer pprof.StopCPUProfile()

	initsystems := []system.System{}
	systems := []system.System{}

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

	initsystems, systems = initializeSystems(initsystems, systems)

	err = cardinal.RegisterSystems(world, systems...)
	if err != nil {
		panic(eris.ToString(err, true))
	}
	err = cardinal.RegisterInitSystems(world, initsystems...)
	if err != nil {
		panic(eris.ToString(err, true))
	}
	err = world.StartGame()
	if err != nil {
		panic(eris.ToString(err, true))
	}
}
