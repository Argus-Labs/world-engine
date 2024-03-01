package events_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func wsURL(addr, path string) string {
	return fmt.Sprintf("ws://%s/%s", addr, path)
}

type Event struct {
	Message string `json:"message"`
}

func TestEvents(t *testing.T) {
	// broadcast 5 messages to 5 clients means 25 messages received.
	numberToTest := 5
	tf := testutils.NewTestFixture(t, nil, cardinal.WithDisableSignatureVerification())
	addr := tf.BaseURL
	tf.StartWorld()
	url := wsURL(addr, "events")
	dialers := make([]*websocket.Conn, numberToTest)
	for i := range dialers {
		dial, _, err := websocket.DefaultDialer.Dial(url, nil)
		assert.NilError(t, err)
		dialers[i] = dial
	}
	var wg sync.WaitGroup
	for i := 0; i < numberToTest; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			tf.World.GetEventHub().EmitEvent(map[string]any{"message": fmt.Sprintf("test%d", i)})
		}()
	}
	wg.Wait()
	go func() {
		tf.World.GetEventHub().FlushEvents()
	}()
	var count atomic.Int32
	count.Store(0)
	for _, dialer := range dialers {
		wg.Add(1)
		dialer := dialer
		go func() {
			defer wg.Done()
			for j := 0; j < numberToTest; j++ {
				mode, message, err := dialer.ReadMessage()
				assert.NilError(t, err)
				assert.Equal(t, mode, websocket.TextMessage)
				var messageMap = make(map[string]string, 0)
				err = json.Unmarshal(message, &messageMap)
				assert.NilError(t, err)
				messageString, ok := messageMap["message"]
				assert.True(t, ok)
				assert.Equal(t, messageString[:4], "test")
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, count.Load(), int32(numberToTest*numberToTest))
}

type garbageStructAlpha struct {
	Something int `json:"something"`
}

func (garbageStructAlpha) Name() string { return "alpha" }

type garbageStructBeta struct {
	Something int `json:"something"`
}

func (garbageStructBeta) Name() string { return "beta" }

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

func TestEventsThroughSystems(t *testing.T) {
	numberToTest := 5
	tf := testutils.NewTestFixture(t, nil, cardinal.WithDisableSignatureVerification())
	world, addr := tf.World, tf.BaseURL
	assert.NilError(t, cardinal.RegisterMessage[SendEnergyTx, SendEnergyTxResult](world, "send-energy"))
	counter1 := atomic.Int32{}
	counter1.Store(0)
	event := map[string]any{"message": "test"}
	sys1 := func(wCtx engine.Context) error {
		wCtx.EmitEvent(event)
		counter1.Add(1)
		return nil
	}
	sys2 := func(wCtx engine.Context) error {
		wCtx.EmitEvent(event)
		counter1.Add(1)
		return nil
	}
	sys3 := func(wCtx engine.Context) error {
		wCtx.EmitEvent(event)
		counter1.Add(1)
		return nil
	}
	sys4 := func(wCtx engine.Context) error {
		wCtx.EmitEvent(event)
		counter1.Add(1)
		return nil
	}
	sys5 := func(wCtx engine.Context) error {
		wCtx.EmitEvent(event)
		counter1.Add(1)
		return nil
	}
	err := cardinal.RegisterSystems(world, sys1, sys2, sys3, sys4, sys5)
	assert.NilError(t, err)
	assert.NilError(t, cardinal.RegisterComponent[garbageStructAlpha](world))
	assert.NilError(t, cardinal.RegisterComponent[garbageStructBeta](world))
	tf.StartWorld()
	url := wsURL(addr, "events")
	dialers := make([]*websocket.Conn, numberToTest)
	for i := range dialers {
		dial, _, err := websocket.DefaultDialer.Dial(url, nil)
		assert.NilError(t, err)
		dialers[i] = dial
	}
	for i := 0; i < numberToTest; i++ {
		tf.DoTick()
	}

	waitForDialersToRead := sync.WaitGroup{}
	counter2 := atomic.Int32{}
	counter2.Store(0)
	for _, dialer := range dialers {
		dialer := dialer
		waitForDialersToRead.Add(1)
		go func() {
			defer waitForDialersToRead.Done()
			for i := 0; i < numberToTest; i++ {
				mode, message, err := dialer.ReadMessage()
				assert.NilError(t, err)
				receivedEvent := Event{}
				assert.NilError(t, json.Unmarshal(message, &receivedEvent))
				assert.Equal(t, mode, websocket.TextMessage)
				assert.Equal(t, receivedEvent.Message, "test")
				counter2.Add(1)
			}
		}()
	}
	waitForDialersToRead.Wait()

	assert.Equal(t, counter1.Load(), int32(numberToTest*numberToTest))
	assert.Equal(t, counter2.Load(), int32(numberToTest*numberToTest))
}

type ThreadSafeBuffer struct {
	internalBuffer bytes.Buffer
	mutex          sync.Mutex
}

func (b *ThreadSafeBuffer) Write(p []byte) (n int, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.internalBuffer.Write(p)
}

func (b *ThreadSafeBuffer) String() string {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.internalBuffer.String()
}
