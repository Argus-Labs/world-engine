package sys

import (
	cryptorand "crypto/rand"
	"math/big"

	"github.com/argus-labs/world-engine/example/tester_benchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

var TenThousandEntityIds = []entity.ID{}
var OneHundredEntityIds = []entity.ID{}
var TreeEntityIds = []entity.ID{}

func InitTenThousandEntities(wCtx cardinal.WorldContext) error {
	var err error
	entityAmount := 10000
	TenThousandEntityIds, err = cardinal.CreateMany(wCtx, entityAmount, &comp.SingleNumber{Number: 1})
	if err != nil {
		return err
	}
	return nil
}

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

func SystemTestGetSmallComponentA(wCtx cardinal.WorldContext) error {
	for _, id := range TenThousandEntityIds {
		gotcomp, err := cardinal.GetComponent[comp.SingleNumber](wCtx, id)
		if err != nil {
			return err
		}
		var maxRand int64 = 100
		num, err := cryptorand.Int(cryptorand.Reader, big.NewInt(maxRand))
		if err != nil {
			return err
		}
		gotcomp.Number = int(num.Int64())
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return err
		}
	}
	return nil
}

func SystemTestGetSmallComponentB(wCtx cardinal.WorldContext) error {
	var maxRand int64 = 1000 - 10
	num, err := cryptorand.Int(cryptorand.Reader, big.NewInt(maxRand))
	if err != nil {
		return err
	}
	startIndex := int(num.Int64())
	for _, id := range TenThousandEntityIds[startIndex : startIndex+10] {
		for i := 0; i < 1000; i++ {
			gotcomp, err := cardinal.GetComponent[comp.SingleNumber](wCtx, id)
			if err != nil {
				return err
			}
			var maxRand int64 = 100
			num, err := cryptorand.Int(cryptorand.Reader, big.NewInt(maxRand))
			if err != nil {
				return err
			}
			gotcomp.Number = int(num.Int64())
			err = cardinal.SetComponent(wCtx, id, gotcomp)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func SystemTestSearchC(wCtx cardinal.WorldContext) error {
	err := wCtx.NewSearch(cardinal.Exact(comp.SingleNumber{})).Each(wCtx, func(id entity.ID) bool {
		return true
	})
	if err != nil {
		return err
	}
	return nil
}

func SystemTestGetComponentWithArrayD(wCtx cardinal.WorldContext) error {
	for _, id := range OneHundredEntityIds {
		gotcomp, err := cardinal.GetComponent[comp.ArrayComp](wCtx, id)
		if err != nil {
			return err
		}
		gotcomp.Numbers = [10000]int{1, 1, 1, 1, 1, 1}
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return err
		}
	}
	return nil
}

func SystemTestGetAndSetComponentWithArrayE(wCtx cardinal.WorldContext) error {
	var maxRand int64 = 100 - 10
	num, err := cryptorand.Int(cryptorand.Reader, big.NewInt(maxRand))
	if err != nil {
		return err
	}
	startIndex := int(num.Int64())
	for _, id := range OneHundredEntityIds[startIndex : startIndex+10] {
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

func SystemTestSearchingForCompWithArrayF(wCtx cardinal.WorldContext) error {
	err := wCtx.NewSearch(cardinal.Exact(comp.ArrayComp{})).Each(wCtx, func(id entity.ID) bool {
		return true
	})
	if err != nil {
		return err
	}
	return nil
}

func SystemTestEntityCreationG(wCtx cardinal.WorldContext) error {
	entityAmount := 100000
	_, err := cardinal.CreateMany(wCtx,
		entityAmount,
		comp.SingleNumber{Number: 1},
		comp.ArrayComp{Numbers: [10000]int{1, 1, 1, 1, 1, 1}})
	if err != nil {
		return err
	}
	return nil
}

func SystemTestGettingHighlyNestedComponentsH(wCtx cardinal.WorldContext) error {
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
