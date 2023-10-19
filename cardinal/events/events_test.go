package events_test

import (
	"fmt"
	"sync"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/gorilla/websocket"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/test_utils"
)

func TestEvents(t *testing.T) {
	//broadcast 5 messages to 5 clients means 25 messages received.
	numberToTest := 5
	w := ecs.NewTestWorld(t)
	assert.NilError(t, w.LoadGameState())
	txh := test_utils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	url := txh.MakeWebSocketURL("events")
	dialers := make([]*websocket.Conn, numberToTest, numberToTest)
	for i, _ := range dialers {
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
			//txh.eventHub.broadcast <- []byte(fmt.Sprintf("test%d", i))
			txh.EventHub.EmitEvent(&events.Event{Message: fmt.Sprintf("test%d", i)})
		}()
	}
	wg.Wait()
	go func() {
		txh.EventHub.FlushEvents()
	}()
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
				//fmt.Println(string(message))
			}
		}()
	}
	wg.Wait()
	txh.EventHub.shutdown <- true
}
