package main

import (
	"errors"
	"log"
	"os"
	"runtime/pprof"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/sys"
	"github.com/rotisserie/eris"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/comp"
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

	// TODO: Unused decorator somehow this breaks profiling (it ends up with missing pieces of the stack).
	// TODO: Figure out how to get this to work.
	// timedShutdownDecorator := func(system cardinal.System, tickAmount int) cardinal.System {
	//	counter := 0
	//	return func(context cardinal.WorldContext) error {
	//		context.Engine().GetEngine().Logger.Info().Msg("system")
	//		if counter >= tickAmount {
	//			pid := os.Getpid()                        // Get the current process's PID
	//			err := syscall.Kill(pid, syscall.SIGTERM) // Send SIGTERM to itself
	//			if err != nil {
	//				panic(err) // Handle error
	//			}
	//			return nil
	//		} else {
	//			counter++
	//			return system(context)
	//		}
	//	}
	//}

	initsystems, systems = initializeSystems(initsystems, systems)

	// for i, system := range systems {
	//	systems[i] = timedShutdownDecorator(system, 100)
	// }

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
