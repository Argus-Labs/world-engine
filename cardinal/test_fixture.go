package cardinal

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rotisserie/eris"
	"github.com/spf13/viper"
	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/tick"
	"pkg.world.dev/world-engine/cardinal/world"
	"pkg.world.dev/world-engine/sign"
)

const (
	DefaultTestPersonaTag = "testpersona"
)

// TestCardinal is a helper struct that manages a cardinal.World instance. It will automatically clean up its resources
// at the end of the test.
type TestCardinal struct {
	testing.TB
	*Cardinal

	signer ecdsa.PrivateKey
	nonce  uint64

	// Base url is something like "localhost:5050". You must attach http:// or ws:// as well as a resource path
	BaseURL string
	Redis   *miniredis.Miniredis

	TickTrigger       chan time.Time
	TickSubscription  <-chan *tick.Tick
	startSubscription <-chan bool

	doCleanup func()
	startOnce *sync.Once
}

// NewTestCardinal creates a test fixture with user defined port for Cardinal integration tests.
func NewTestCardinal(t testing.TB, redis *miniredis.Miniredis, opts ...CardinalOption) *TestCardinal {
	if redis == nil {
		redis = miniredis.RunT(t)
	}

	ports, err := findOpenPorts(2) //nolint:gomnd
	assert.NilError(t, err)

	cardinalPort := ports[0]
	evmPort := ports[1]

	t.Setenv("BASE_SHARD_SEQUENCER_ADDRESS", "localhost:"+evmPort)
	t.Setenv("CARDINAL_LOG_PRETTY", "true")
	t.Setenv("CARDINAL_PORT", cardinalPort)
	t.Setenv("REDIS_ADDRESS", redis.Addr())

	tickTrigger, doneTickCh := make(chan time.Time), make(chan uint64)

	startSubscription := make(chan bool)
	defaultOpts := []CardinalOption{
		WithTickChannel(tickTrigger),
		WithMockJobQueue(),
		WithStartHook(func() error {
			startSubscription <- true
			close(startSubscription)
			return nil
		}),
	}

	// Default options go first so that any user supplied options overwrite the defaults.
	c, _, err := New(append(defaultOpts, opts...)...)
	assert.NilError(t, err)

	signer, err := crypto.GenerateKey()
	assert.NilError(t, err)

	return &TestCardinal{
		TB:       t,
		Cardinal: c,

		signer: *signer,
		nonce:  0,

		BaseURL: "localhost:" + cardinalPort,
		Redis:   redis,

		TickTrigger:       tickTrigger,
		TickSubscription:  c.Subscribe(),
		startSubscription: startSubscription,

		startOnce: &sync.Once{},
		// Only register this method with t.Cleanup if the game server is actually started
		doCleanup: func() {
			viper.Reset()

			// Optionally, you can also clear environment variables if needed
			for _, key := range viper.AllKeys() {
				err := os.Unsetenv(key)
				if err != nil {
					t.Errorf("failed to unset env var %s: %v", key, err)
				}
			}

			// First, make sure completed ticks will never be blocked
			go func() {
				for range doneTickCh { //nolint:revive // This pattern drains the channel until closed
				}
			}()

			// Next, shut down the world
			c.Stop()

			// The world is shut down; No more ticks will be started
			close(tickTrigger)
		},
	}
}

// StartWorld starts the game world and registers a cleanup function that will shut down
// the cardinal World at the end of the test. Components/Systems/Queries, etc should
// be registered before calling this function.
func (c *TestCardinal) StartWorld() {
	c.startOnce.Do(func() {
		startupError := make(chan error)
		go func() {
			// StartGame is meant to block forever, so any return value will be non-nil and cause for concern.
			// Also, calling t.Fatal from a non-main thread only reports a failure once the test on the main thread has
			// completed. By sending this error out on a channel we can fail the test right away (assuming doTick
			// has been called from the main thread).
			startupError <- c.Cardinal.Start()
		}()

		// Wait for the start hook to trigger and mark the world is ready
		<-c.startSubscription

		c.Cleanup(c.doCleanup)
	})
}

// DoTick executes one game tick and blocks until the tick is complete. StartWorld is automatically called if it was
// not called before the first tick.
func (c *TestCardinal) DoTick() {
	c.StartWorld()
	c.TickTrigger <- time.Now()
	<-c.TickSubscription
}

func (c *TestCardinal) httpURL(path string) string {
	return fmt.Sprintf("http://%s/%s", c.BaseURL, path)
}

// Post executes a http POST request to this TextFixture's cardinal server.
func (c *TestCardinal) Post(path string, payload any) *http.Response {
	bz, err := json.Marshal(payload)
	assert.NilError(c, err)
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		c.httpURL(strings.Trim(path, "/")),
		bytes.NewReader(bz),
	)
	assert.NilError(c, err)
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	assert.NilError(c, err)
	return resp
}

// Get executes a http GET request to this TestCardinal's cardinal server.
func (c *TestCardinal) Get(path string) *http.Response {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.httpURL(strings.Trim(path, "/")),
		nil)
	assert.NilError(c, err)
	resp, err := http.DefaultClient.Do(req)
	assert.NilError(c, err)
	return resp
}

func (c *TestCardinal) AddTransactionWithPersona(msgName string, personaTag string, msg any) common.Hash {
	tx, err := c.newTx(personaTag, msg)
	assert.NilError(c, err)

	txHash, err := c.world.AddTransaction(msgName, tx)
	assert.NilError(c, err)

	return *txHash
}

// AddTransaction adds a transaction with a default persona tag to the transaction pool.
func (c *TestCardinal) AddTransaction(msgName string, msg any) common.Hash {
	wCtx := world.NewWorldContextReadOnly(c.World().State(), c.World().Persona())

	_, _, err := wCtx.GetPersona(DefaultTestPersonaTag)
	if err != nil {
		if eris.Is(err, world.ErrPersonaNotRegistered) {
			c.CreatePersona(DefaultTestPersonaTag)
		} else {
			assert.NilError(c, err)
		}
	}

	tx, err := c.newTx(DefaultTestPersonaTag, msg)
	assert.NilError(c, err)

	txHash, err := c.world.AddTransaction(msgName, tx)
	assert.NilError(c, err)

	return *txHash
}

func (c *TestCardinal) CreatePersona(personaTag string) {
	c.AddTransactionWithPersona("persona.create-persona", "test", world.CreatePersona{PersonaTag: personaTag})
	c.DoTick()
}

func (c *TestCardinal) HandleQuery(group string, name string, request any) ([]byte, error) {
	// Marshal request payload to JSON bytes
	reqBz, err := json.Marshal(request)
	assert.NilError(c, err)

	// Call the HandleQuery method on the QueryManager
	return c.Cardinal.World().HandleQuery(group, name, reqBz)
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

func (c *TestCardinal) newTx(personaTag string, msg any) (*sign.Transaction, error) {
	c.nonce++
	sig, err := sign.NewTransaction(&c.signer, personaTag, c.world.Namespace(), c.nonce, msg)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

func (c *TestCardinal) SignerAddress() string {
	return crypto.PubkeyToAddress(c.signer.PublicKey).Hex()
}
