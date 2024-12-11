package testsuite

import (
	"context"
	"crypto/ecdsa"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/require"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

var (
	privateKey *ecdsa.PrivateKey
)

func SetTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			panic("test timed out")
		}
	}()
}

func UniqueSignatureWithName(name string) *sign.Transaction {
	if privateKey == nil {
		var err error
		privateKey, err = crypto.GenerateKey()
		if err != nil {
			panic(err)
		}
	}

	// We only verify signatures when hitting the HTTP server, and in tests we're likely just adding transactions
	// directly to the World tx pool. It's OK if the signature does not match the payload.
	sig, err := sign.NewTransaction(privateKey, name, "namespace", `{"some":"data"}`)
	if err != nil {
		panic(err)
	}
	return sig
}

func UniqueSignature() *sign.Transaction {
	return UniqueSignatureWithName("some_persona_tag")
}

func GetMessage[In any, Out any](w *cardinal.World) (*cardinal.MessageType[In, Out], error) {
	var msg cardinal.MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := w.GetMessageByType(msgType)
	if !ok {
		return nil, eris.Errorf("Could not find %q, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*cardinal.MessageType[In, Out])
	if !ok {
		return &msg, eris.New("wrong type")
	}
	return res, nil
}

// NewTestWorld creates a new world instance for testing purposes.
func NewTestWorld(t *testing.T, opts ...cardinal.WorldOption) *cardinal.World {
	t.Helper()

	// Create a new Redis instance for each test
	mr := miniredis.NewMiniRedis()
	mr.RequireAuth("") // Disable authentication

	// Start Redis and let it choose its own port
	err := mr.Start()
	require.NoError(t, err)

	// Get the address that Redis chose
	addr := mr.Addr()
	t.Logf("Started miniredis on %s", addr)

	// Ensure Redis is closed after the test
	t.Cleanup(func() {
		if mr != nil {
			mr.Close()
		}
	})

	// Set the Redis address environment variable
	t.Setenv("REDIS_ADDRESS", addr)

	// Add mock redis option
	opts = append(opts, cardinal.WithMockRedis())

	world, err := cardinal.NewWorld(opts...)
	require.NoError(t, err)

	// Register test components while world is in Init state
	RegisterComponents(world)

	// Start the world and wait for Running state
	errCh := make(chan error, 1)
	go func() {
		errCh <- world.StartGame()
	}()

	// Wait for world to be ready
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if world.IsGameRunning() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !world.IsGameRunning() {
		t.Fatal("world did not enter Running state within timeout")
	}

	// Ensure world is properly cleaned up after the test
	t.Cleanup(func() {
		if world != nil && world.IsGameRunning() {
			world.Shutdown()
			if err := <-errCh; err != nil && err != context.Canceled {
				t.Errorf("world.StartGame() error = %v", err)
			}
		}
	})

	return world
}
