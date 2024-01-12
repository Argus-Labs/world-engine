package sys

import (
	"log"
	"math/rand"

	"github.com/argus-labs/world-engine/example/tester_benchmark/comp"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// Jeremy's requested tests.
var TEN_THOUSAND_ENTITY_IDS = []entity.ID{}
var ONE_HUNDRED_ENTITY_IDS = []entity.ID{}

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
	ONE_HUNDRED_ENTITY_IDS, err = cardinal.CreateMany(wCtx, 100, &comp.ArrayComp{Numbers: [100]int{}})
	if err != nil {
		return err
	}
	return nil
}

var systemACounter = 100

func SystemA(wCtx cardinal.WorldContext) error {
	//wCtx.Logger().Info().Msgf("%d SYSTEMA!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!", len(TEN_THOUSAND_ENTITY_IDS))
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
	if systemACounter == 0 {
		log.Fatalf("Force exit.")
	} else {
		systemACounter--
		wCtx.Logger().Info().Msgf("System A counter at: %d", systemACounter)
	}
	return nil
}

var systemBCounter = systemACounter

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
	if systemBCounter == 0 {
		log.Fatalf("Force exit.")
	} else {
		systemBCounter--
		wCtx.Logger().Info().Msgf("System B counter at: %d", systemBCounter)
	}
	return nil
}

var systemCCounter = systemACounter

func SystemC(wCtx cardinal.WorldContext) error {
	err := wCtx.NewSearch(cardinal.Exact(comp.SingleNumber{})).Each(wCtx, func(id entity.ID) bool {
		return true
	})
	if err != nil {
		return err
	}
	if systemCCounter == 0 {
		log.Fatalf("Force exit.")
	} else {
		systemCCounter--
		wCtx.Logger().Info().Msgf("System C counter at: %d", systemCCounter)
	}
	return nil
}

var systemDCounter = systemACounter

func SystemD(wCtx cardinal.WorldContext) error {
	for _, id := range ONE_HUNDRED_ENTITY_IDS {
		gotcomp, err := cardinal.GetComponent[comp.ArrayComp](wCtx, id)
		if err != nil {
			return err
		}
		gotcomp.Numbers = [100]int{1, 1, 1, 1, 1, 1}
		err = cardinal.SetComponent(wCtx, id, gotcomp)
		if err != nil {
			return nil
		}
	}
	if systemDCounter == 0 {
		log.Fatalf("Force exit.")
	} else {
		systemDCounter--
		wCtx.Logger().Info().Msgf("System D counter at: %d", systemDCounter)
	}
	return nil
}

var systemECounter = systemACounter

func SystemE(wCtx cardinal.WorldContext) error {
	startIndex := rand.Int() % (100 - 10)
	for _, id := range ONE_HUNDRED_ENTITY_IDS[startIndex : startIndex+10] {
		for i := 0; i < 1000; i++ {
			gotcomp, err := cardinal.GetComponent[comp.ArrayComp](wCtx, id)
			if err != nil {
				return err
			}
			gotcomp.Numbers = [100]int{startIndex, startIndex, startIndex, startIndex, startIndex}
			err = cardinal.SetComponent(wCtx, id, gotcomp)
			if err != nil {
				return err
			}
		}
	}
	if systemECounter == 0 {
		log.Fatalf("Force exit.")
	} else {
		systemECounter--
		wCtx.Logger().Info().Msgf("System E counter at: %d", systemECounter)
	}
	return nil
}

var systemFCounter = systemACounter

func SystemF(wCtx cardinal.WorldContext) error {
	err := wCtx.NewSearch(cardinal.Exact(comp.ArrayComp{})).Each(wCtx, func(id entity.ID) bool {
		return true
	})
	if err != nil {
		return err
	}
	if systemFCounter == 0 {
		log.Fatalf("Force exit.")
	} else {
		systemFCounter--
		wCtx.Logger().Info().Msgf("System F counter at: %d", systemFCounter)
	}
	return nil
}

var systemGCounter = systemACounter

func SystemG(wCtx cardinal.WorldContext) error {
	_, err := cardinal.CreateMany(wCtx, 1000, comp.SingleNumber{Number: 1}, comp.ArrayComp{Numbers: [100]int{1, 1, 1, 1, 1, 1}})
	if err != nil {
		return err
	}
	if systemGCounter == 0 {
		log.Fatalf("Force exit.")

	} else {
		systemGCounter--
		wCtx.Logger().Info().Msgf("System G counter at: %d", systemGCounter)
	}
	return nil
}
