package system

import (
	comp "github.com/argus-labs/world-engine/example/tester/agarbenchmark/component"
	"github.com/argus-labs/world-engine/example/tester/agarbenchmark/world"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/search/filter"
)

func CoinSpawnerSystem(wCtx cardinal.WorldContext) error {
	maxConcurrentCoins := world.Settings.MaxConcurrentCoins()
	concurrentCoins, err := cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Coin](), filter.Component[comp.Pickup]())).
		Count(wCtx)
	if err != nil {
		DebugLogError(wCtx, "CoinSpawnerSystem: Error counting coins", err)
		return err
	}
	if concurrentCoins != maxConcurrentCoins {
		wCtx.Logger().Debug().Msgf("Spawning %d coins.", maxConcurrentCoins-concurrentCoins)
	}
	for i := concurrentCoins; i < maxConcurrentCoins; i++ {
		p := getRandomGridPosition()
		_ = SpawnCoin(wCtx, p)
	}
	return nil
}

func MedpackSpawnerSystem(wCtx cardinal.WorldContext) error {
	maxConcurrentMedpacks := world.Settings.MaxConcurrentMedpacks()
	concurrentMedpacks, err := cardinal.NewSearch().
		Entity(filter.Contains(filter.Component[comp.Medpack](), filter.Component[comp.Pickup]())).
		Count(wCtx)
	if err != nil {
		DebugLogError(wCtx, "MedpackSpawnerSystem: Error counting medpacks", err)
		return err
	}
	if concurrentMedpacks != maxConcurrentMedpacks {
		wCtx.Logger().Debug().Msgf("Spawning %d medpacks.", maxConcurrentMedpacks-concurrentMedpacks)
	}
	for i := concurrentMedpacks; i < maxConcurrentMedpacks; i++ {
		p := getRandomGridPosition()
		_ = SpawnMedpack(wCtx, p)
	}
	return nil
}
