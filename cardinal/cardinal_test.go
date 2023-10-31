package cardinal_test

import (
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/test_utils"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

func TestCanQueryInsideSystem(t *testing.T) {
	test_utils.SetTestTimeout(t, 10*time.Second)

	world, doTick := test_utils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	wantNumOfEntities := 10
	world.Init(func(worldCtx cardinal.WorldContext) {
		_, err := cardinal.CreateMany(worldCtx, wantNumOfEntities, Foo{})
		assert.NilError(t, err)
	})
	gotNumOfEntities := 0
	cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
		q, err := worldCtx.NewSearch(cardinal.Exact(Foo{}))
		assert.NilError(t, err)
		err = q.Each(worldCtx, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		assert.NilError(t, err)
		return nil
	})

	doTick()

	err := world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}

func TestShutdownViaSignal(t *testing.T) {
	//test_utils.SetTestTimeout(t, 10*time.Second) // If this test is frozen then it failed to shut down, create a failure with panic.
	world, err := cardinal.NewMockWorld()
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	assert.NilError(t, err)
	wantNumOfEntities := 10
	world.Init(func(worldCtx cardinal.WorldContext) {
		_, err := cardinal.CreateMany(worldCtx, wantNumOfEntities, Foo{})
		assert.NilError(t, err)
	})
	go func() {
		err = world.StartGame()
		assert.NilError(t, err)
	}()
	for !world.IsGameRunning() {
		//wait until game loop is running
		time.Sleep(500 * time.Millisecond)
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:4040/events", nil)
	assert.NilError(t, err)
	finish := make(chan bool)
	go func() {
		_, _, err := conn.ReadMessage()
		assert.Assert(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure))
		finish <- true
	}()
	// Send a SIGINT signal.
	cmd := exec.Command("kill", "-INT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	for world.IsGameRunning() {
		//wait until game loop is not running
		time.Sleep(500 * time.Millisecond)
	}
	<-finish

}
