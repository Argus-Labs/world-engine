package system

import (
	"log"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	comp "github.com/argus-labs/world-engine/game/sample_game_server/server/component"
	tx "github.com/argus-labs/world-engine/game/sample_game_server/server/transaction"
)

func MustInitialize(world *ecs.World) {
	world.AddSystem(CreatePlayer)
	world.AddSystem(CreateFire)
	world.AddSystem(Move)
	world.AddSystem(BurnPlayers)
}

func CreatePlayer(world *ecs.World, tq *ecs.TransactionQueue) error {
	createTxs := tx.CreatePlayer.In(tq)
	newPlayerIDs, err := world.CreateMany(len(createTxs), comp.Health, comp.Position)
	if err != nil {
		return err
	}
	for i := range createTxs {
		id := newPlayerIDs[i]
		createTx := createTxs[i]
		if err := comp.Health.Set(world, id, comp.HealthComponent{Val: 100}); err != nil {
			return err
		}
		if err := comp.Position.Set(world, id, comp.PositionComponent{
			X: createTx.X,
			Y: createTx.Y,
		}); err != nil {
			return err
		}
	}
	return nil
}

func CreateFire(world *ecs.World, tq *ecs.TransactionQueue) error {
	for _, createFire := range tx.CreateFire.In(tq) {
		id, err := world.Create(comp.Position)
		if err != nil {
			return err
		}
		if err = comp.Position.Set(world, id, comp.PositionComponent{X: createFire.X, Y: createFire.Y}); err != nil {
			return err
		}
	}
	return nil
}

func Move(world *ecs.World, tq *ecs.TransactionQueue) error {
	for _, move := range tx.Move.In(tq) {
		pos, err := comp.Position.Get(world, move.ID)
		if err != nil {
			return err
		}
		pos.X += move.XDelta
		pos.Y += move.YDelta
		if err = comp.Position.Set(world, move.ID, pos); err != nil {
			return err
		}
	}
	return nil
}

func BurnPlayers(world *ecs.World, tq *ecs.TransactionQueue) error {
	fires := map[comp.PositionComponent]bool{}
	ecs.NewQuery(filter.Exact(comp.Position)).Each(world, func(id storage.EntityID) {
		pos, err := comp.Position.Get(world, id)
		if err != nil {
			log.Print(err)
			return
		}
		fires[pos] = true
	})
	ecs.NewQuery(filter.Exact(comp.Health, comp.Position)).Each(world, func(id storage.EntityID) {
		pos, err := comp.Position.Get(world, id)
		if err != nil {
			log.Print(err)
			return
		}
		if !fires[pos] {
			return
		}

		health, err := comp.Health.Get(world, id)
		if err != nil {
			log.Print(err)
			return
		}
		health.Val -= 10
		if err := comp.Health.Set(world, id, health); err != nil {
			log.Print(err)
			return
		}
	})
	return nil
}
