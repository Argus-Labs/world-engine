package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/alicebob/miniredis/v2"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

// TestFixture is a helper struct that manages a cardinal.World instance. It will automatically clean up its resources
// at the end of the test.
type TestFixture struct {
	testing.TB

	// Base url is something like "localhost:5050". You must attach http:// or ws:// as well as a resource path
	BaseURL string
	Redis   *miniredis.Miniredis
	World   *cardinal.World
	Engine  *ecs.Engine

	startTickCh chan time.Time
	doneTickCh  chan uint64
	doCleanup   func()
	startOnce   *sync.Once
}

// NewTestFixture creates a test fixture that manges the cardinal.World, ecs.Engine, http server, event hub,
// evm adapter, etc. Cardinal resources (such as Systems and Components) can be registered with the attached
// cardinal.World, but you must call StartWorld or DoTick to finalize the resources. If a nil miniRedis is passed
// in, a miniredis instance will be created for you.
func NewTestFixture(t testing.TB, miniRedis *miniredis.Miniredis, opts ...cardinal.WorldOption) *TestFixture {
	if miniRedis == nil {
		miniRedis = miniredis.RunT(t)
	}

	cardinalPort := getOpenPort(t)
	evmPort := getOpenPort(t)
	assert.Assert(t, cardinalPort != evmPort, "cardinal and evm port must be different")
	t.Setenv("CARDINAL_DEPLOY_MODE", "development")
	t.Setenv("REDIS_ADDRESS", miniRedis.Addr())
	t.Setenv("CARDINAL_EVM_PORT", evmPort)

	startTickCh, doneTickCh := make(chan time.Time), make(chan uint64)

	defaultOpts := []cardinal.WorldOption{
		cardinal.WithCustomMockRedis(miniRedis),
		cardinal.WithTickChannel(startTickCh),
		cardinal.WithTickDoneChannel(doneTickCh),
		cardinal.WithPort(cardinalPort),
	}

	// default options go first so that any user supplied options overwrite the defaults.
	opts = append(defaultOpts, opts...)

	world, err := cardinal.NewWorld(opts...)
	assert.NilError(t, err)

	testFixture := &TestFixture{
		TB:      t,
		BaseURL: "localhost:" + cardinalPort,
		Redis:   miniRedis,
		World:   world,
		Engine:  world.Engine(),

		startTickCh: startTickCh,
		doneTickCh:  doneTickCh,
		startOnce:   &sync.Once{},
		// Only register this method with t.Cleanup if the game server is actually started
		doCleanup: func() {
			close(startTickCh)
			go func() {
				for range doneTickCh { //nolint:revive // This pattern drains the channel until closed
				}
			}()
			assert.NilError(t, world.Shutdown())
		},
	}

	return testFixture
}

// StartWorld starts the game world and registers a cleanup function that will shut down
// the cardinal World at the end of the test. Components/Systems/Queries, etc should
// be registered before calling this function.
func (t *TestFixture) StartWorld() {
	t.startOnce.Do(func() {
		timeout := time.After(5 * time.Second) //nolint:gomnd // fine for now.
		startupError := make(chan error)
		go func() {
			// StartGame is meant to block forever, so any return value will be non-nil and cause for concern.
			// Also, calling t.Fatal from a non-main thread only reports a failure once the test on the main thread has
			// completed. By sending this error out on a channel we can fail the test right away (assuming doTick
			// has been called from the main thread).
			startupError <- t.World.StartGame()
		}()
		for !t.World.IsGameRunning() {
			select {
			case err := <-startupError:
				t.Fatalf("startup error: %v", err)
			case <-timeout:
				t.Fatal("timeout while waiting for game to start")
			default:
				time.Sleep(10 * time.Millisecond) //nolint:gomnd // its for testing its ok.
			}
		}
		t.Cleanup(t.doCleanup)
	})
}

// DoTick executes one game tick and blocks until the tick is complete. StartWorld is automatically called if it was
// not called before the first tick.
func (t *TestFixture) DoTick() {
	t.StartWorld()
	t.startTickCh <- time.Now()
	<-t.doneTickCh
}

func (t *TestFixture) httpURL(path string) string {
	return fmt.Sprintf("http://%s/%s", t.BaseURL, path)
}

// Post executes a http POST request to this TextFixture's cardinal server.
func (t *TestFixture) Post(path string, payload any) *http.Response {
	bz, err := json.Marshal(payload)
	assert.NilError(t, err)
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		t.httpURL(strings.Trim(path, "/")),
		bytes.NewReader(bz),
	)
	assert.NilError(t, err)
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	assert.NilError(t, err)
	return resp
}

// Get executes a http GET request to this TestFixture's cardinal server.
func (t *TestFixture) Get(path string) *http.Response {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, t.httpURL(strings.Trim(path, "/")),
		nil)
	assert.NilError(t, err)
	resp, err := http.DefaultClient.Do(req)
	assert.NilError(t, err)
	return resp
}

func getOpenPort(t testing.TB) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	defer func() {
		assert.NilError(t, l.Close())
	}()

	assert.NilError(t, err)
	tcpAddr, err := net.ResolveTCPAddr(l.Addr().Network(), l.Addr().String())
	assert.NilError(t, err)
	return fmt.Sprintf("%d", tcpAddr.Port)
}
