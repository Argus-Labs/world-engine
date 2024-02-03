package sys

import (
	"log"

	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

var OneHundredEntityIds = []entity.ID{}
var TreeEntityIds = []entity.ID{}

func InitOneHundredEntities(wCtx cardinal.WorldContext) error {
	var err error
	entityAmount := 100
	OneHundredEntityIds, err = cardinal.CreateMany(wCtx, entityAmount, &comp.ArrayComp{Numbers: [10000]int{}})
	if err != nil {
		return err
	}
	return nil
}

func InitTreeEntities(wCtx cardinal.WorldContext) error {
	var err error
	var entityAmount = 100
	var treeDepth = 10
	wCtx.Logger().Info().Msg("CREATING tree entity")
	tree := comp.CreateTree(treeDepth)
	TreeEntityIds, err = cardinal.CreateMany(wCtx, entityAmount, *tree)
	if err != nil {
		wCtx.Logger().Info().Msg("ERROR CREATING tree entity")
		wCtx.Logger().Info().Msg(err.Error())
		return err
	}
	return nil
}

func ProfilerTerminatorDecoratorForSystem(system cardinal.System, tickToStopProfiling uint) cardinal.System {
	tickCounter := tickToStopProfiling
	return func(wCtx cardinal.WorldContext) error {
		result := system(wCtx)
		if tickCounter == 0 {
			log.Fatalf("Exiting.") // Just die, all we need is the profile.
		}
		tickCounter--

		return result
	}
}

func SystemBenchmarkTest(wCtx cardinal.WorldContext) error {
	for _, id := range TreeEntityIds {
		gotcomp, err := cardinal.GetComponent[comp.Tree](wCtx, id)
		if err != nil {
			return err
		}
		gotcomp.UpdateTree()
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return err
		}
	}
	return nil
}
