package sys

import (
	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/comp"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
)

var (
	TenThousandEntityIDs []types.EntityID
	OneHundredEntityIDs  []types.EntityID
	TreeEntityIDs        []types.EntityID
)

func InitTenThousandEntities(wCtx cardinal.WorldContext) error {
	var err error
	entityAmount := 10000
	TenThousandEntityIDs, err = cardinal.CreateMany(wCtx, entityAmount, &comp.SingleNumber{Number: 1})
	if err != nil {
		return err
	}
	return nil
}

func InitOneHundredEntities(wCtx cardinal.WorldContext) error {
	var err error
	entityAmount := 100
	OneHundredEntityIDs, err = cardinal.CreateMany(wCtx, entityAmount, &comp.ArrayComp{Numbers: [10000]int{}})
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
	TreeEntityIDs, err = cardinal.CreateMany(wCtx, entityAmount, *tree)
	if err != nil {
		wCtx.Logger().Info().Msg("ERROR CREATING tree entity")
		wCtx.Logger().Info().Msg(err.Error())
		return err
	}
	return nil
}

func SystemBenchmark(wCtx cardinal.WorldContext) error {
	for _, id := range TreeEntityIDs {
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
