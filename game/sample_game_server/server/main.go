/*
This sample game server exposes 4 endpoints:

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
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/argus-labs/world-engine/game/sample_game_server/server/component"
	"github.com/argus-labs/world-engine/game/sample_game_server/server/system"
	"github.com/argus-labs/world-engine/game/sample_game_server/server/transaction"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

func mustSetupWorld(world *ecs.World) {
	component.MustInitialize(world)
	transaction.MustInitialize(world)
	system.MustInitialize(world)
	if err := world.LoadGameState(); err != nil {
		panic(err)
	}
}

const EnvGameServerPort = "GAME_SERVER_PORT"

func main() {
	world := inmem.NewECSWorld()
	mustSetupWorld(world)

	port := os.Getenv(EnvGameServerPort)
	if port == "" {
		log.Fatalf("Must specify a port via %s", EnvGameServerPort)
	}
	h := &httpHandler{world: world}

	handlers := []struct {
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"list_players", h.listPlayers},
		{"create_player", h.createPlayer},
		{"create_fire", h.createFire},
		{"move_player", h.movePlayer},
	}

	log.Printf("Attempting to register %d handlers\n", len(handlers))
	var paths []string
	for _, h := range handlers {
		http.HandleFunc("/"+h.path, h.handler)
		paths = append(paths, h.path)
	}
	http.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		enc := json.NewEncoder(w)
		if err := enc.Encode(paths); err != nil {
			writeError(w, "cant marshal list", err)
		}
	})

	go gameLoop(world)

	log.Printf("Starting server on port %s\n", port)
	http.ListenAndServe(":"+port, nil)
}

func gameLoop(world *ecs.World) {
	for range time.Tick(time.Second) {
		if err := world.Tick(); err != nil {
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

func (h *httpHandler) listPlayers(w http.ResponseWriter, _ *http.Request) {
	players, err := getPlayerInfoFromWorld(h.world)
	if err != nil {
		writeError(w, "failed to list players", err)
	} else {
		writeResult(w, players)
	}
}

func (h *httpHandler) createFire(w http.ResponseWriter, r *http.Request) {
	createFire, err := decode[transaction.CreateFireTransaction](r)
	if err != nil {
		writeError(w, "unable to decode create fire tx", err)
	}
	transaction.CreateFire.AddToQueue(h.world, createFire)
	writeResult(w, "ok")
}

func (h *httpHandler) createPlayer(w http.ResponseWriter, r *http.Request) {
	createPlayer, err := decode[transaction.CreatePlayerTransaction](r)
	if err != nil {
		writeError(w, "unable to decode create player tx", err)
		return
	}
	transaction.CreatePlayer.AddToQueue(h.world, createPlayer)
	writeResult(w, "ok")
}

func (h *httpHandler) movePlayer(w http.ResponseWriter, r *http.Request) {
	movePlayer, err := decode[transaction.MoveTransaction](r)
	if err != nil {
		writeError(w, "unable to decode move tx", err)
		return
	}
	transaction.Move.AddToQueue(h.world, movePlayer)
	writeResult(w, "ok")
}

func queryPlayers(world *ecs.World) *ecs.Query {
	return ecs.NewQuery(filter.Exact(component.Health, component.Position))
}

func getPlayerInfoFromWorld(world *ecs.World) ([]playerInfo, error) {
	var players []playerInfo
	var errs []error

	queryPlayers(world).Each(world, func(id storage.EntityID) {
		currPos, err := component.Position.Get(world, id)
		if err != nil {
			errs = append(errs, err)
			return
		}
		currHealth, err := component.Health.Get(world, id)
		if err != nil {
			errs = append(errs, err)
			return
		}
		players = append(players, playerInfo{
			ID:     id,
			Health: currHealth.Val,
			XPos:   currPos.X,
			YPos:   currPos.Y,
		})
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
