package testutils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/rotisserie/eris"
	"golang.org/x/sync/errgroup"
	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

// TestFixture is a helper struct that manages a cardinal.World instance. It will automatically clean up its resources
// at the end of the test.
type TestFixture struct {
	testing.TB

	// Base url is something like "localhost:5050". You must attach http:// or ws:// as well as a resource path
	BaseURL string
	World   *cardinal.World
	Redis   *miniredis.Miniredis

	StartTickCh  chan time.Time
	DoneTickCh   chan uint64
	PanicTickCh  chan error
	startOnce    *sync.Once
	shutdownOnce *sync.Once
}

// NewTestFixture creates a test fixture with user defined port for Cardinal integration tests.
func NewTestFixture(t testing.TB, redis *miniredis.Miniredis, opts ...cardinal.WorldOption) *TestFixture {
	if redis == nil {
		redis = miniredis.RunT(t)
	}

	ports, err := findOpenPorts(2) //nolint:gomnd
	assert.NilError(t, err)

	cardinalPort := ports[0]
	evmPort := ports[1]

	t.Setenv("CARDINAL_DEPLOY_MODE", "development")
	t.Setenv("CARDINAL_EVM_PORT", evmPort)
	t.Setenv("REDIS_ADDRESS", redis.Addr())

	startTickCh, doneTickCh, panicTickCh := make(chan time.Time), make(chan uint64), make(chan error)

	defaultOpts := []cardinal.WorldOption{
		cardinal.WithTickChannel(startTickCh),
		cardinal.WithTickDoneChannel(doneTickCh),
		cardinal.WithTickPanicChannel(panicTickCh),
		cardinal.WithPort(cardinalPort),
	}

	// Default options go first so that any user supplied options overwrite the defaults.
	world, err := cardinal.NewWorld(append(defaultOpts, opts...)...)
	assert.NilError(t, err)

	return &TestFixture{
		TB:      t,
		BaseURL: "localhost:" + cardinalPort,
		World:   world,
		Redis:   redis,

		StartTickCh:  startTickCh,
		DoneTickCh:   doneTickCh,
		PanicTickCh:  panicTickCh,
		startOnce:    &sync.Once{},
		shutdownOnce: &sync.Once{},
	}
}

// StartWorld starts the world and will automatically clean up its resources when the test finishes.
// Components, systems, queries, etc. should be registered before calling this function.
func (t *TestFixture) StartWorld() {
	t.startOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:gomnd
		defer cancel()

		g, gCtx := errgroup.WithContext(ctx)
		g.Go(func() error {
			return t.World.StartGame()
		})

		// While world is still not running, wait until there is an error or it times out.
		for !t.World.IsGameRunning() {
			select {
			case <-gCtx.Done():
				t.Fatal("failed to start world", gCtx.Err())
			default:
				time.Sleep(10 * time.Millisecond) //nolint:gomnd
			}
		}

		// If the world successfully starts, register the cleanup function.
		t.Cleanup(t.Shutdown)
	})
}

// Shutdown will gracefully shut down the world and clean up its resources.
// This function will automatically be called when the test finishes.
func (t *TestFixture) Shutdown() {
	t.shutdownOnce.Do(func() {
		// Next, shut down the world, but only if it is still running
		// The world might have been shut down via a SIGINT, etc.
		if t.World.IsGameRunning() {
			assert.NilError(t, t.World.Shutdown())
		}
	})
}

// DoTick executes one game tick and blocks until the tick is complete. StartWorld is automatically called if it was
// not called before the first tick.
func (t *TestFixture) DoTick() (uint64, error) {
	t.StartWorld()
	t.StartTickCh <- time.Now()

	var tick uint64
	select {
	case err := <-t.PanicTickCh:
		return 0, err
	case tick = <-t.DoneTickCh:
		return tick, nil
	}
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

func (t *TestFixture) AddTransaction(txID types.MessageID, tx any, sigs ...*sign.Transaction) types.TxHash {
	sig := &sign.Transaction{}
	if len(sigs) > 0 {
		sig = sigs[0]
	}
	_, id := t.World.AddTransaction(txID, tx, sig)
	return id
}

func (t *TestFixture) CreatePersona(personaTag, signerAddr string) {
	personaMsg := msg.CreatePersona{
		PersonaTag:    personaTag,
		SignerAddress: signerAddr,
	}
	createPersonaMsg, exists := t.World.GetMessageByFullName("persona." + msg.CreatePersonaMessageName)
	assert.Check(
		t,
		exists,
		"message with name %q not registered in World", msg.CreatePersonaMessageName,
	)
	t.AddTransaction(createPersonaMsg.ID(), personaMsg, &sign.Transaction{})
	_, err := t.DoTick()
	assert.NilError(t, err)
}

// findOpenPorts finds a set of open ports and returns them as a slice of strings.
// It is guaranteed that the returned slice will have the amount of ports requested and that there is no duplicate
// ports in the slice.
func findOpenPorts(amount int) ([]string, error) {
	ports := make([]string, 0, amount)

	// Try to find open ports until we find the target amount or we run out of retries
	for i := 0; i < amount; i++ {
		var found bool

		// Try to find a random port, retying if it turns out to be a duplicate in list of ports up to 10 times
		for retries := 10; retries > 0; retries-- {
			port, err := findOpenPort()
			if err != nil {
				continue
			}

			// Check for duplicate ports
			for _, existingPort := range ports {
				if port == existingPort {
					continue
				}
			}

			// Add the port to the list and break out of the inner loop
			ports = append(ports, port)
			found = true
			break
		}

		if !found {
			return nil, eris.New("failed to find open ports after retries")
		}
	}

	return ports, nil
}

// findOpenPort finds an open port and returns it as a string.
// If you need to find multiple ports, use findOpenPorts to make sure that the ports are unique.
func findOpenPort() (string, error) {
	findFn := func() (string, error) {
		// Try to get a random port using the wildcard 0 port
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", eris.Wrap(err, "failed to initialize listener")
		}

		// Get the autoamtically assigned port number from the listener
		tcpAddr, err := net.ResolveTCPAddr(l.Addr().Network(), l.Addr().String())
		if err != nil {
			return "", eris.Wrap(err, "failed to resolve address")
		}

		// Close the listener when the function returns
		err = l.Close()
		if err != nil {
			return "", eris.Wrap(err, "failed to close listener")
		}
		return strconv.Itoa(tcpAddr.Port), nil
	}

	for retries := 10; retries > 0; retries-- {
		port, err := findFn()
		if err == nil {
			return port, nil
		}
		time.Sleep(10 * time.Millisecond) //nolint:gomnd // it's fine.
	}

	return "", eris.New("failed to find an open port")
}
