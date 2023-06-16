package main

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type RoomID int

type GameComponent struct {
	ID             storage.EntityID
	ArrowCount     int
	WumpusLocation RoomID
	PlayerLocation RoomID
	IsWumpusDead   bool
	Message        string
}

func (g GameComponent) isGameOver() bool {
	if g.IsWumpusDead {
		return true
	}
	if g.ArrowCount == 0 || g.WumpusLocation == g.PlayerLocation {
		return true
	}
	return false
}

var (
	GameComp = ecs.NewComponentType[GameComponent]()
)

type MoveTransaction struct {
	GameID storage.EntityID
	Room   RoomID
}

type FireTransaction struct {
	GameID storage.EntityID
	Room   RoomID
}

type NewGameTransaction struct{}

var (
	MoveTx    = ecs.NewTransactionType[MoveTransaction]()
	FireTx    = ecs.NewTransactionType[FireTransaction]()
	NewGameTx = ecs.NewTransactionType[NewGameTransaction]()
)

func (r RoomID) isValid() bool {
	return r >= 1 && r <= 8
}

func SystemHandleMove(w *ecs.World, tq *ecs.TransactionQueue) error {
	moves := MoveTx.In(tq)
	for _, m := range moves {
		game, err := GameComp.Get(w, m.GameID)
		if err != nil || game.isGameOver() || !m.Room.isValid() {
			continue
		}
		game.PlayerLocation = m.Room
		if game.PlayerLocation == game.WumpusLocation {
			game.Message = "You woke the Wumpus. It ate you. You lose."
		}
		if err := GameComp.Set(w, m.GameID, game); err != nil {
			return err
		}
	}
	return nil
}

func SystemHandleFire(w *ecs.World, tq *ecs.TransactionQueue) error {
	for _, s := range FireTx.In(tq) {
		game, err := GameComp.Get(w, s.GameID)
		if err != nil || game.isGameOver() || !s.Room.isValid() {
			continue
		}
		if s.Room == game.WumpusLocation {
			game.IsWumpusDead = true
			game.Message = "The Wumpus has been killed. You win!"
		}
		game.ArrowCount--
		if game.ArrowCount == 0 {
			game.Message = "You ran out of arrows. Your death is inevitable. You lose."
		}

		if err := GameComp.Set(w, s.GameID, game); err != nil {
			return err
		}
	}
	return nil
}

func SystemStartNewGame(w *ecs.World, tq *ecs.TransactionQueue) error {
	newgameRequests := NewGameTx.In(tq)
	if len(newgameRequests) == 0 {
		return nil
	}
	id, err := w.Create(GameComp)
	if err != nil {
		return err
	}

	playerAt := randomRoom(0)
	game := GameComponent{
		ID:             id,
		PlayerLocation: playerAt,
		WumpusLocation: randomRoom(playerAt),
		ArrowCount:     5,
	}
	if err := GameComp.Set(w, id, game); err != nil {
		return err
	}
	fmt.Println("new game started with ID", game.ID)
	return nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

var gameID storage.EntityID

func main() {
	world := inmem.NewECSWorld()
	must(world.RegisterComponents(GameComp))
	must(world.RegisterTransactions(MoveTx, FireTx, NewGameTx))
	world.AddSystem(SystemStartNewGame)
	world.AddSystem(SystemHandleMove)
	world.AddSystem(SystemHandleFire)

	must(world.LoadGameState())

	go tickInBG(world)

	for {
		fmt.Println(inputLoop(world))
	}
}

const helpText = `Commands are:
	new game
		to create a new game
	play [X]
		to start playing the game with the ID X
	move [1-8]
		to move your character to the given room
	shoot [1-8]
		to fire an arrow at the given room
	look
		to check what room you're in and listen for the wumpus	
	listen
		to check what room you're in and listen for the wumpus	
`

func inputLoop(world *ecs.World) string {
	cmd := getCmd()
	if len(cmd) == 2 {
		if cmd[0] == "new" && cmd[1] == "game" {
			NewGameTx.AddToQueue(world, NewGameTransaction{})
			return "you want to start a new game"
		} else if cmd[0] == "play" {
			id, err := strconv.Atoi(cmd[1])
			if err != nil {
				return fmt.Sprintf("unknown game id %v", cmd[1])
			}
			gameID = storage.EntityID(id)
			return fmt.Sprintf("now playing game %d", gameID)
		} else if cmd[0] == "move" {
			target, err := strconv.Atoi(cmd[1])
			if err != nil {
				return fmt.Sprintf("unknown target room %v", cmd[1])
			}
			MoveTx.AddToQueue(world, MoveTransaction{gameID, RoomID(target)})
			return "move command registered"
		} else if cmd[0] == "shoot" || cmd[0] == "fire" {
			target, err := strconv.Atoi(cmd[1])
			if err != nil {
				return fmt.Sprintf("unknown target room %v", cmd[1])
			}
			FireTx.AddToQueue(world, FireTransaction{gameID, RoomID(target)})
			return "fire command registered"
		}
	} else if len(cmd) == 1 {
		if cmd[0] == "help" {
			fmt.Println(helpText)
		} else if cmd[0] == "look" || cmd[0] == "listen" {
			game, err := GameComp.Get(world, gameID)
			if err != nil {
				return fmt.Sprintf("unable to get game: %v", err)
			}
			if game.isGameOver() {
				fmt.Println("This game is over")
				fmt.Println(game.Message)
				return ""
			}
			fmt.Printf("You are in room %d.\n", game.PlayerLocation)
			dramaticPause()
			fmt.Printf("Adjacent rooms are %v\n", worldMap[game.PlayerLocation])
			dramaticPause()
			fmt.Println("It is very dark...")
			dramaticPause()
			fmt.Println("You listen carefully...")
			dramaticPause()
			adj := "faint"
			if isAdjacent(game.PlayerLocation, game.WumpusLocation) {
				adj = "loud"
			}
			fmt.Printf("You hear deep, %s snoring.\n", adj)
			return ""
		}
	}

	return "unknown command. type 'help' for help"
}

func dramaticPause() {
	time.Sleep(time.Second)
}

func getCmd() []string {
	fmt.Print(">")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := scanner.Text()
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return strings.Split(input, " ")
}

func tickInBG(world *ecs.World) {
	for range time.Tick(time.Second) {
		if err := world.Tick(); err != nil {
			log.Fatal(err)
		}
	}
}

// The game map takes places on the vertices of a cube
var worldMap = map[RoomID][3]RoomID{
	1: {2, 4, 8},
	2: {1, 3, 7},
	3: {2, 4, 6},
	4: {1, 3, 5},
	5: {8, 4, 6},
	6: {5, 3, 7},
	7: {8, 2, 6},
	8: {1, 7, 5},
}

func isAdjacent(a, b RoomID) bool {
	ns, ok := worldMap[a]
	if !ok {
		return false
	}
	return b == ns[0] || b == ns[1] || b == ns[2]
}

func randomRoom(butNot RoomID) RoomID {
	n := rand.Intn(8) + 1
	if RoomID(n) == butNot {
		n--
		if n == 0 {
			n = 8
		}
	}
	return RoomID(n)
}
