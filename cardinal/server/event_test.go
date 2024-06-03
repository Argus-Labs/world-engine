package server_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
)

type SendEnergyTx struct {
	From, To string
	Amount   uint64
}

type SendEnergyTxResult struct{}

type Alpha struct {
	Something int `json:"something"`
}

func (Alpha) Name() string { return "alpha" }

type Beta struct {
	Something int `json:"something"`
}

func (Beta) Name() string { return "beta" }

func TestEventsThroughSystems(t *testing.T) {
	numberToTest := 5
	tf := cardinal.NewTestFixture(t, nil, cardinal.WithDisableSignatureVerification())
	world, addr := tf.World, tf.BaseURL
	assert.NilError(t, cardinal.RegisterMessage[SendEnergyTx, SendEnergyTxResult](world, "send-energy"))
	counter1 := atomic.Int32{}
	counter1.Store(0)
	event := map[string]any{"message": "test"}
	sys1 := func(wCtx cardinal.WorldContext) error {
		err := wCtx.EmitEvent(event)
		assert.Check(t, err == nil, "emit event encountered error is system 1: %v", err)
		counter1.Add(1)
		return nil
	}
	sys2 := func(wCtx cardinal.WorldContext) error {
		err := wCtx.EmitEvent(event)
		assert.Check(t, err == nil, "emit event encountered error is system 2: %v", err)
		counter1.Add(1)
		return nil
	}
	sys3 := func(wCtx cardinal.WorldContext) error {
		err := wCtx.EmitEvent(event)
		assert.Check(t, err == nil, "emit event encountered error is system 3: %v", err)
		counter1.Add(1)
		return nil
	}
	sys4 := func(wCtx cardinal.WorldContext) error {
		err := wCtx.EmitEvent(event)
		assert.Check(t, err == nil, "emit event encountered error is system 4: %v", err)
		counter1.Add(1)
		return nil
	}
	sys5 := func(wCtx cardinal.WorldContext) error {
		err := wCtx.EmitEvent(event)
		assert.Check(t, err == nil, "emit event encountered error is system 5: %v", err)
		counter1.Add(1)
		return nil
	}
	err := cardinal.RegisterSystems(world, sys1, sys2, sys3, sys4, sys5)
	assert.NilError(t, err)
	assert.NilError(t, cardinal.RegisterComponent[Alpha](world))
	assert.NilError(t, cardinal.RegisterComponent[Beta](world))
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
				receivedTickResults := cardinal.TickResults{}
				assert.NilError(t, json.Unmarshal(message, &receivedTickResults))
				event := make(map[string]any)
				assert.Equal(t, len(receivedTickResults.Events), 5)
				assert.NilError(t, json.Unmarshal(receivedTickResults.Events[0], &event))
				assert.Equal(t, mode, websocket.TextMessage)
				assert.Equal(t, event["message"], "test")
				counter2.Add(1)
			}
		}()
	}
	waitForDialersToRead.Wait()

	assert.Equal(t, counter1.Load(), int32(numberToTest*numberToTest))
	assert.Equal(t, counter2.Load(), int32(numberToTest*numberToTest))
}

func wsURL(addr, path string) string {
	return fmt.Sprintf("ws://%s/%s", addr, path)
}
