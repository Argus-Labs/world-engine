package main

import (
	"encoding/json"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"log"
	"net/http"
	"os"
)

const EnvGameServerPort = "GAME_SERVER_PORT"

func newWorld() *ecs.World {
	s, err := miniredis.Run()
	if err != nil {
		panic(fmt.Sprintf("Unable to start miniredis: %v", err))
	}
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "",
		DB:       0,
	}, "main-world")
	worldStorage := storage.NewWorldStorage(
		storage.Components{Store: &rs, ComponentIndices: &rs},
		&rs,
		storage.NewArchetypeComponentIndex(),
		storage.NewArchetypeAccessor(),
		&rs,
		&rs)
	return ecs.NewWorld(worldStorage)
}

type BoardComponent struct {
	Xs, Os int
}

type PlayerComponent struct {
	Name string
}

var (
	world    = newWorld()
	Board    = ecs.NewComponentType[BoardComponent]()
	Host     = ecs.NewComponentType[PlayerComponent]()
	Opponent = ecs.NewComponentType[PlayerComponent]()
)

func init() {
	world.RegisterComponents(Board, Host, Opponent)
}

func main() {
	port := os.Getenv(EnvGameServerPort)
	if port == "" {
		log.Fatalf("Must specify a port via %s", EnvGameServerPort)
	}

	handlers := []struct {
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"games", handleGames},
		{"games/create", handleCreateGame},
		{"games/move", handleMakeMove},
	}

	log.Printf("Attempting to register %d handlers\n", len(handlers))
	paths := []string{}
	for _, h := range handlers {
		http.HandleFunc("/"+h.path, h.handler)
		paths = append(paths, h.path)
	}
	http.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		enc := json.NewEncoder(w)
		if err := enc.Encode(paths); err != nil {
			writeError(w, "can't marshal list", err)
		}
	})

	log.Printf("Starting server on port %s\n", port)
	http.ListenAndServe(":"+port, nil)
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

type NewGame struct {
	Host     string
	Opponent string
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func handleCreateGame(w http.ResponseWriter, r *http.Request) {
	gameParams := NewGame{}
	if err := decode(r, &gameParams); err != nil {
		writeError(w, "decode failed", err)
		return
	}
	if gameParams.Host == "" {
		writeError(w, "must specify host", nil)
		return
	}
	if gameParams.Opponent == "" {
		writeError(w, "must specify opponent", nil)
		return
	}

	gameID, err := world.Create(Board, Host, Opponent)
	if err != nil {
		writeError(w, "game creation failed", err)
	}
	gameEnt, err := world.Entity(gameID)
	if err != nil {
		writeError(w, "game to entity failed", err)
	}

	Host.Set(gameEnt, &PlayerComponent{gameParams.Host})
	Opponent.Set(gameEnt, &PlayerComponent{gameParams.Opponent})
	w.WriteHeader(200)
	writeResult(w, "success")
}

func handleMakeMove(w http.ResponseWriter, r *http.Request) {
	writeError(w, "not implemented", nil)
}

func handleGames(w http.ResponseWriter, r *http.Request) {
	ids := []storage.EntityID{}
	Host.Each(world, func(entity storage.Entity) {
		ids = append(ids, entity.ID)
	})

	writeResult(w, ids)
}
