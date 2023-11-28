package cardinal_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/assert"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Rawbodytx struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

func TestCreatePersona(t *testing.T) {
	var wg sync.WaitGroup
	namespace := "custom-namespace"
	t.Setenv("CARDINAL_NAMESPACE", namespace)
	world := testutils.NewTestWorld(t)
	wg.Add(1)
	go func() {
		err := world.StartGame()
		assert.NilError(t, err)
		wg.Done()
	}()
	for !world.IsGameRunning() {
		// wait until game loop is running
		time.Sleep(50 * time.Millisecond)
	}
	goodKey, err := crypto.GenerateKey()
	assert.NilError(t, err)
	body := Rawbodytx{
		PersonaTag:    "a",
		SignerAddress: crypto.PubkeyToAddress(goodKey.PublicKey).Hex(),
	}
	wantBody, err := json.Marshal(body)
	assert.NilError(t, err)
	wantNonce := uint64(100)
	sp, err := sign.NewSystemTransaction(goodKey, namespace, wantNonce, wantBody)
	assert.NilError(t, err)
	bodyBytes, err := json.Marshal(sp)
	assert.NilError(t, err)
	client := &http.Client{}
	req, err := http.NewRequest(
		http.MethodPost, "http://localhost:4040/tx/persona/create-persona", bytes.NewBuffer(bodyBytes))
	assert.NilError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
	err = world.ShutDown()
	assert.NilError(t, err)
}

func TestNewWorld(t *testing.T) {
	world, err := cardinal.NewMockWorld()
	assert.NilError(t, err)
	assert.Equal(t, string(world.Instance().Namespace()), cardinal.DefaultNamespace)
	err = world.ShutDown()
	assert.NilError(t, err)
}

func TestNewWorldWithCustomNamespace(t *testing.T) {
	t.Setenv("CARDINAL_NAMESPACE", "custom-namespace")
	world, err := cardinal.NewWorld()
	assert.NilError(t, err)
	assert.Equal(t, string(world.Instance().Namespace()), "custom-namespace")
}

func TestCanQueryInsideSystem(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)

	world, doTick := testutils.MakeWorldAndTicker(t)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	gotNumOfEntities := 0
	err := cardinal.RegisterSystems(world, func(worldCtx cardinal.WorldContext) error {
		q, err := worldCtx.NewSearch(cardinal.Exact(Foo{}))
		assert.NilError(t, err)
		err = q.Each(worldCtx, func(cardinal.EntityID) bool {
			gotNumOfEntities++
			return true
		})
		assert.NilError(t, err)
		return nil
	})
	assert.NilError(t, err)

	doTick()
	wantNumOfEntities := 10
	wCtx := cardinal.TestingWorldToWorldContext(world)
	_, err = cardinal.CreateMany(wCtx, wantNumOfEntities, Foo{})
	assert.NilError(t, err)
	doTick()
	assert.Equal(t, world.CurrentTick(), uint64(2))
	err = world.ShutDown()
	assert.Assert(t, err)
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}

func TestShutdownViaSignal(t *testing.T) {
	// If this test is frozen then it failed to shut down, create a failure with panic.
	var wg sync.WaitGroup
	testutils.SetTestTimeout(t, 10*time.Second)
	world := testutils.NewTestWorld(t)
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	wantNumOfEntities := 10
	world.Init(func(worldCtx cardinal.WorldContext) error {
		_, err := cardinal.CreateMany(worldCtx, wantNumOfEntities/2, Foo{})
		if err != nil {
			return err
		}
		return nil
	})
	wg.Add(1)
	go func() {
		err := world.StartGame()
		assert.NilError(t, err)
		wg.Done()
	}()
	for !world.IsGameRunning() {
		// wait until game loop is running
		time.Sleep(50 * time.Millisecond)
	}
	wCtx := cardinal.TestingWorldToWorldContext(world)
	_, err := cardinal.CreateMany(wCtx, wantNumOfEntities/2, Foo{})
	assert.NilError(t, err)
	// test CORS with cardinal
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, "http://localhost:4040/query/http/endpoints", nil)
	assert.NilError(t, err)
	req.Header.Set("Origin", "http://www.bullshit.com") // test CORS
	resp, err := client.Do(req)
	assert.NilError(t, err)
	v := resp.Header.Get("Access-Control-Allow-Origin")
	assert.Equal(t, v, "*")
	assert.Equal(t, resp.StatusCode, 200)

	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:4040/events", nil)
	assert.NilError(t, err)
	wg.Add(1)
	go func() {
		_, _, err := conn.ReadMessage()
		assert.Assert(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure))
		wg.Done()
	}()
	// Send a SIGINT signal.
	cmd := exec.Command("kill", "-INT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	for world.IsGameRunning() {
		// wait until game loop is not running
		time.Sleep(50 * time.Millisecond)
	}
	wg.Wait()
}
