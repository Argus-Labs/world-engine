/*
This sample game server shard built using Cardinal exposes 4 endpoints:

list_players:

	returns a list contains each player's id, health, and position

create_player:

	create a player. A body of `{"X": 10, "Y": 20}` will create a player at position 10, 20

create_fire

	create a fire. A body of `{"X": 99, "Y": 200}` will create a fire at position 99, 200

move_player

	move a player. A body of `{"ID": 5, "XDelta": 10, "YDelta": 20}` will move player 5 10 units
	in the X direction and 20 units in the Y direction.

For every tick that a player is standing on a fire, they will lose 10 health.
*/
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/argus-labs/world-engine/cardinal/server"
	"github.com/argus-labs/world-engine/game/sample_game/server/component"
	"github.com/argus-labs/world-engine/game/sample_game/server/system"
	"github.com/argus-labs/world-engine/game/sample_game/server/transaction"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

func mustSetupWorld(world *ecs.World) {
	must(world.RegisterComponents(
		component.Health,
		component.Position,
	))
	must(world.RegisterTransactions(
		transaction.Move,
		transaction.CreateFire,
		transaction.CreatePlayer,
	))
	world.AddSystem(system.PlayerSpawnerSystem)
	world.AddSystem(system.FireSpawnerSystem)
	world.AddSystem(system.MoveSystem)
	world.AddSystem(system.BurnSystem)

	must(world.LoadGameState())
}

const (
	EnvCardinalPort = "CARDINAL_PORT"
)

func main() {
	world := inmem.NewECSWorld()
	mustSetupWorld(world)

	port := os.Getenv(EnvCardinalPort)
	if port == "" {
		log.Fatalf("Must specify a port via %s", EnvCardinalPort)
	}

	go gameLoop(world)

	th, err := server.NewHandler(world) //, server.DisableSignatureVerification())
	if err != nil {
		log.Fatal(err)
	}
	th.Serve("", port)
}

func gameLoop(world *ecs.World) {
	for range time.Tick(time.Second) {
		if err := world.Tick(context.Background()); err != nil {
			panic(err)
		}
	}
}

type playerInfo struct {
	ID     storage.EntityID
	Health int
	XPos   int
	YPos   int
}

type httpHandler struct {
	world *ecs.World
}

func queryPlayers(world *ecs.World) *ecs.Query {
	return ecs.NewQuery(filter.Exact(component.Health, component.Position))
}

func getPlayerInfoFromWorld(world *ecs.World) ([]playerInfo, error) {
	var players []playerInfo
	var errs []error

	queryPlayers(world).Each(world, func(id storage.EntityID) bool {
		currPos, err := component.Position.Get(world, id)
		if err != nil {
			errs = append(errs, err)
			return true
		}
		currHealth, err := component.Health.Get(world, id)
		if err != nil {
			errs = append(errs, err)
			return true
		}
		players = append(players, playerInfo{
			ID:     id,
			Health: currHealth.Val,
			XPos:   currPos.X,
			YPos:   currPos.Y,
		})
		return true
	})
	return players, errors.Join(errs...)
}

func decode[T any](r *http.Request) (T, error) {
	var val T
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&val); err != nil {
		return val, err
	}
	return val, nil
}

func writeError(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(500)
	fmt.Fprintf(w, "%s: %v", msg, err)
}

func writeResult(w http.ResponseWriter, v any) {
	if s, ok := v.(string); ok {
		v = struct{ Msg string }{Msg: s}
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(v); err != nil {
		writeError(w, "can't encode", err)
		return
	}
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
