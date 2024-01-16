package sys

import (
	"math/rand"

	"github.com/argus-labs/world-engine/example/tester_benchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// Jeremy's requested tests.
var TEN_THOUSAND_ENTITY_IDS = []entity.ID{}
var ONE_HUNDRED_ENTITY_IDS = []entity.ID{}
var TREE_ENTITY_IDS = []entity.ID{}

func CreateShutDownFunc(world *cardinal.World) func(cardinal.WorldContext) {
	return func(wCtx cardinal.WorldContext) {
		wCtx.Logger().Info().Msgf("SHUTTING DOWN BENCHMARK")
		world.ShutDown()
	}
}

var ShutdownFunc = func(wCtx cardinal.WorldContext) {}

func InitTenThousandEntities(wCtx cardinal.WorldContext) error {
	var err error
	TEN_THOUSAND_ENTITY_IDS, err = cardinal.CreateMany(wCtx, 10000, &comp.SingleNumber{Number: 1})
	if err != nil {
		return err
	}
	return nil
}

func InitOneHundredEntities(wCtx cardinal.WorldContext) error {
	var err error
	ONE_HUNDRED_ENTITY_IDS, err = cardinal.CreateMany(wCtx, 100, &comp.ArrayComp{Numbers: [10000]int{}})
	if err != nil {
		return err
	}
	return nil
}

func InitTreeEntities(wCtx cardinal.WorldContext) error {
	var err error
	wCtx.Logger().Ino().Msg("CREATING tree entity")
	tree := comp.CreateTree(10)
	TREE_ENTITY_IDS, err = cardinal.CreateMany(wCtx, 100, *tree)
	if err != nil {
		wCtx.Logger().Info().Msg("ERROR CREATING tree entity")
		wCtx.Logger().Info().Msg(err.Error())
		return err
	}
	return nil
}

func SystemA(wCtx cardinal.WorldContext) error {
	for _, id := range TEN_THOUSAND_ENTITY_IDS {

		gotcomp, err := cardinal.GetComponent[comp.SingleNumber](wCtx, id)
		if err != nil {
			return err
		}
		gotcomp.Number = rand.Int()
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return err
		}
	}
	return nil
}

func SystemB(wCtx cardinal.WorldContext) error {
	startIndex := rand.Int() % (1000 - 10)
	for _, id := range TEN_THOUSAND_ENTITY_IDS[startIndex : startIndex+10] {
		for i := 0; i < 1000; i++ {
			gotcomp, err := cardinal.GetComponent[comp.SingleNumber](wCtx, id)
			if err != nil {
				return err
			}
			gotcomp.Number = rand.Int()
			err = cardinal.SetComponent(wCtx, id, gotcomp)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SystemC(wCtx cardinal.WorldContext) error {
	err := wCtx.NewSearch(cardinal.Exact(comp.SingleNumber{})).Each(wCtx, func(id entity.ID) bool {
		return true
	})
	if err != nil {
		return err
	}
	return nil
}

func SystemD(wCtx cardinal.WorldContext) error {
	for _, id := range ONE_HUNDRED_ENTITY_IDS {
		gotcomp, err := cardinal.GetComponent[comp.ArrayComp](wCtx, id)
		if err != nil {
			return err
		}
		gotcomp.Numbers = [10000]int{1, 1, 1, 1, 1, 1}
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return nil
		}
	}
	return nil
}

func SystemE(wCtx cardinal.WorldContext) error {
	startIndex := rand.Int() % (100 - 10)
	for _, id := range ONE_HUNDRED_ENTITY_IDS[startIndex : startIndex+10] {
		for i := 0; i < 1000; i++ {
			gotcomp, err := cardinal.GetComponent[comp.ArrayComp](wCtx, id)
			if err != nil {
				return err
			}
			gotcomp.Numbers = [10000]int{startIndex, startIndex, startIndex, startIndex, startIndex}
			err = cardinal.SetComponent(wCtx, id, gotcomp)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SystemF(wCtx cardinal.WorldContext) error {
	err := wCtx.NewSearch(cardinal.Exact(comp.ArrayComp{})).Each(wCtx, func(id entity.ID) bool {
		return true
	})
	if err != nil {
		return err
	}
	return nil
}

func SystemG(wCtx cardinal.WorldContext) error {
	_, err := cardinal.CreateMany(wCtx, 100000, comp.SingleNumber{Number: 1}, comp.ArrayComp{Numbers: [10000]int{1, 1, 1, 1, 1, 1}})
	if err != nil {
		return err
	}
	return nil
}

func SystemH(wCtx cardinal.WorldContext) error {
	for _, id := range TREE_ENTITY_IDS {
		gotcomp, err := cardinal.GetComponent[comp.Tree](wCtx, id)
		if err != nil {
			return err
		}
		gotcomp.UpdateTree()
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return nil
		}
	}
	return nil
}
