package cardinal_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
)

type Foo struct{}

func (Foo) Name() string { return "foo" }

type Bar struct{}

func (Bar) Name() string { return "bar" }

type Qux struct{}

func (Qux) Name() string { return "qux" }

type Rawbodytx struct {
	PersonaTag    string `json:"personaTag"`
	SignerAddress string `json:"signerAddress"`
}

func TestCreatePersona(t *testing.T) {
	namespace := "custom-namespace"
	t.Setenv("CARDINAL_NAMESPACE", namespace)
	tf := testutils.NewTestFixture(t, nil)
	addr := tf.BaseURL
	tf.DoTick()

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
		http.MethodPost, "http://"+addr+"/tx/persona/create-persona", bytes.NewBuffer(bodyBytes))
	assert.NilError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	assert.NilError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestNewWorld(t *testing.T) {
	world, err := cardinal.NewMockWorld()
	assert.NilError(t, err)
	assert.Equal(t, string(world.Namespace()), cardinal.DefaultNamespace)
	err = world.Shutdown()
	assert.NilError(t, err)
}

func TestNewWorldWithCustomNamespace(t *testing.T) {
	t.Setenv("CARDINAL_NAMESPACE", "custom-namespace")
	world, err := cardinal.NewMockWorld()
	assert.NilError(t, err)
	assert.Equal(t, string(world.Namespace()), "custom-namespace")
	err = world.Shutdown()
	assert.NilError(t, err)
}

func TestCanQueryInsideSystem(t *testing.T) {
	testutils.SetTestTimeout(t, 10*time.Second)

	tf := testutils.NewTestFixture(t, nil)
	world, doTick := tf.World, tf.DoTick
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))

	gotNumOfEntities := 0
	err := cardinal.RegisterSystems(world, func(eCtx engine.Context) error {
		err := cardinal.NewSearch(eCtx, filter.Exact(Foo{})).Each(func(id entity.ID) bool {
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
	assert.Equal(t, gotNumOfEntities, wantNumOfEntities)
}

func TestCanGetTimestampFromWorldContext(t *testing.T) {
	var ts uint64
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	err := cardinal.RegisterSystems(world, func(context engine.Context) error {
		ts = context.Timestamp()
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()
	tf.DoTick()
	lastTS := ts
	time.Sleep(time.Second)
	tf.DoTick()
	assert.Check(t, ts > lastTS)
}

func TestShutdownViaSignal(t *testing.T) {
	// If this test is frozen then it failed to shut down, cardinal.Create a failure with panic.
	testutils.SetTestTimeout(t, 10*time.Second)
	tf := testutils.NewTestFixture(t, nil)
	world, addr := tf.World, tf.BaseURL
	httpBaseURL := "http://" + addr
	assert.NilError(t, cardinal.RegisterComponent[Foo](world))
	wantNumOfEntities := 10
	err := cardinal.RegisterInitSystems(world, func(eCtx engine.Context) error {
		_, err := cardinal.CreateMany(eCtx, wantNumOfEntities/2, Foo{})
		if err != nil {
			return err
		}
		return nil
	})
	assert.NilError(t, err)
	tf.StartWorld()
	wCtx := cardinal.TestingWorldToWorldContext(world)
	_, err = cardinal.CreateMany(wCtx, wantNumOfEntities/2, Foo{})
	assert.NilError(t, err)
	// test CORS with cardinal
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodGet, httpBaseURL+"/query/http/endpoints", nil)
	assert.NilError(t, err)
	req.Header.Set("Origin", "http://www.bullshit.com") // test CORS
	resp, err := client.Do(req)
	assert.NilError(t, err)
	v := resp.Header.Get("Access-Control-Allow-Origin")
	assert.Equal(t, v, "*")
	assert.Equal(t, resp.StatusCode, 200)

	// TODO(scott): not sure what we are triyng to test here, but this seems to just be blocked at conn.ReadMessage()
	//wsBaseURL := "ws://" + addr
	//conn, _, err := websocket.DefaultDialer.Dial(wsBaseURL+"/events", nil)
	//assert.NilError(t, err)
	//var wg sync.WaitGroup
	//wg.Add(1)
	//go func() {
	//	_, _, err := conn.ReadMessage()
	//	assert.Assert(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure))
	//	wg.Done()
	//}()

	// Send a SIGINT signal.
	fmt.Println("Sending SIGINT signal to process", os.Getpid())
	cmd := exec.Command("kill", "-SIGINT", strconv.Itoa(os.Getpid()))
	err = cmd.Run()
	assert.NilError(t, err)

	for world.IsGameRunning() {
		// wait until game loop is not running
		fmt.Println("Waiting for game loop to stop")
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("Game loop stopped")
}
