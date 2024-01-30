package events_test

import (
	"bytes"
	"fmt"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

func wsURL(addr, path string) string {
	return fmt.Sprintf("ws://%s/%s", addr, path)
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
			tf.World.Engine().GetEventHub().EmitEvent(&events.Event{Message: fmt.Sprintf("test%d", i)})
		}()
	}
	wg.Wait()
	go func() {
		tf.World.Engine().GetEventHub().FlushEvents()
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
				assert.Equal(t, string(message)[:4], "test")
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
	eng, addr := tf.Engine, tf.BaseURL
	sendTx := ecs.NewMessageType[SendEnergyTx, SendEnergyTxResult]("send-energy")
	assert.NilError(t, eng.RegisterMessages(sendTx))
	counter1 := atomic.Int32{}
	counter1.Store(0)
	sys1 := func(eCtx engine.Context) error {
		eCtx.EmitEvent("test")
		counter1.Add(1)
		return nil
	}
	sys2 := func(eCtx engine.Context) error {
		eCtx.EmitEvent("test")
		counter1.Add(1)
		return nil
	}
	sys3 := func(eCtx engine.Context) error {
		eCtx.EmitEvent("test")
		counter1.Add(1)
		return nil
	}
	sys4 := func(eCtx engine.Context) error {
		eCtx.EmitEvent("test")
		counter1.Add(1)
		return nil
	}
	sys5 := func(eCtx engine.Context) error {
		eCtx.EmitEvent("test")
		counter1.Add(1)
		return nil
	}
	err := eng.RegisterSystems(sys1, sys2, sys3, sys4, sys5)
	assert.NilError(t, err)
	assert.NilError(t, ecs.RegisterComponent[garbageStructAlpha](eng))
	assert.NilError(t, ecs.RegisterComponent[garbageStructBeta](eng))
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
				assert.Equal(t, mode, websocket.TextMessage)
				assert.Equal(t, string(message), "test")
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
