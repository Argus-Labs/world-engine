package server

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	v1 "github.com/argus-labs/world-engine/cardinal/net/proto/gen/go/ecs/v1"
)

var _ v1.GameServer = gameServer{}

func NewGameServer(backend *redis.Client) v1.GameServer {
	return gameServer{}
}

type gameServer struct {
	world   ecs.World
	backend *redis.Client
}

func (i gameServer) StartGameLoop(ctx context.Context, loop *v1.MsgStartGameLoop) (*v1.MsgStartGameLoopResponse, error) {
	worldID := 0 // TODO: figure this out
	store := storage.NewRedisStorage(i.backend, worldID)
	world := ecs.NewWorld(store, worldID)
	i.world = world
	// from here.. we need to initialize the world in the ECS system. loading components, making entities, etc etc..
	// how will that look?
	//
	// comps := gamedirectory.Components() <-- these are components defined by the developer, i.e. whoever is implementing dark forest in the ECS
	//
	// i.world.RegisterComponents(comps...)
	//
	// i.world.Create(whatever components) <-- creating the actual entities.
	// i.world.CreateMany(amount, whatever components) <--- creating multiple of some entity
	//
	// i assume here we would sent entity ID's back to client, so they can assign them to planets??
	return &v1.MsgStartGameLoopResponse{}, nil
}
