package sys

import (
	"github.com/argus-labs/world-engine/example/tester/gamebenchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
)

var TenThousandEntityIds = []types.EntityID{}
var OneHundredEntityIds = []types.EntityID{}
var TreeEntityIds = []types.EntityID{}

func InitTenThousandEntities(wCtx engine.Context) error {
	var err error
	entityAmount := 10000
	TenThousandEntityIds, err = cardinal.CreateMany(wCtx, entityAmount, &comp.SingleNumber{Number: 1})
	if err != nil {
		return err
	}
	return nil
}

func InitOneHundredEntities(wCtx engine.Context) error {
	var err error
	entityAmount := 100
	OneHundredEntityIds, err = cardinal.CreateMany(wCtx, entityAmount, &comp.ArrayComp{Numbers: [10000]int{}})
	if err != nil {
		return err
	}
	return nil
}

func InitTreeEntities(wCtx engine.Context) error {
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

func SystemBenchmark(wCtx cardinal.WorldContext) error {
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
