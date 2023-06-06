package server

import (
	"context"

	v1 "buf.build/gen/go/argus-labs/cardinal/grpc/go/ecs/ecsv1grpc"
	ecsv1 "buf.build/gen/go/argus-labs/cardinal/protocolbuffers/go/ecs"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

var _ v1.GameServer = &gameServer{}

func NewGameServer(backend storage.WorldStorage) v1.GameServer {
	return &gameServer{}
}

type gameServer struct {
	world   *ecs.World
	backend storage.WorldStorage
}

func (i *gameServer) StartGameLoop(ctx context.Context, loop *ecsv1.MsgStartGameLoop) (*ecsv1.MsgStartGameLoopResponse, error) {
	world, err := ecs.NewWorld(i.backend)
	if err != nil {
		return nil, err
	}
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
	return &ecsv1.MsgStartGameLoopResponse{}, nil
}
