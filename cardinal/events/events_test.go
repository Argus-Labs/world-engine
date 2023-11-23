package events_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/testutils"

	"pkg.world.dev/world-engine/assert"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"pkg.world.dev/world-engine/cardinal/ecs"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/server"
)

func TestEventError(t *testing.T) {
	err := events.WebSocketEchoHandler(nil)
	// fmt.Println(eris.ToString(err, true))
	mappedJSON := eris.ToJSON(err, true)
	v, ok := mappedJSON["root"]
	assert.Assert(t, ok)
	v1, ok := v.(map[string]interface{})
	assert.Assert(t, ok)
	v2, ok := v1["stack"]
	assert.Assert(t, ok)
	v3, ok := v2.([]string)
	assert.Assert(t, ok)
	assert.Assert(t, len(v3) > 0)
}

func TestEvents(t *testing.T) {
	// broadcast 5 messages to 5 clients means 25 messages received.
	numberToTest := 5
	w := testutils.NewTestWorld(t).Instance()
	assert.NilError(t, w.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	url := txh.MakeWebSocketURL("events")
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
			txh.EventHub.EmitEvent(&events.Event{Message: fmt.Sprintf("test%d", i)})
		}()
	}
	wg.Wait()
	go func() {
		txh.EventHub.FlushEvents()
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
	txh.EventHub.ShutdownEventHub()
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
	w := testutils.NewTestWorld(t).Instance()
	sendTx := ecs.NewMessageType[SendEnergyTx, SendEnergyTxResult]("send-energy")
	assert.NilError(t, w.RegisterMessages(sendTx))
	counter1 := atomic.Int32{}
	counter1.Store(0)
	for i := 0; i < numberToTest; i++ {
		w.RegisterSystem(func(wCtx ecs.WorldContext) error {
			wCtx.GetWorld().EmitEvent(&events.Event{Message: "test"})
			counter1.Add(1)
			return nil
		})
	}
	assert.NilError(t, ecs.RegisterComponent[garbageStructAlpha](w))
	assert.NilError(t, ecs.RegisterComponent[garbageStructBeta](w))
	assert.NilError(t, w.LoadGameState())
	txh := testutils.MakeTestTransactionHandler(t, w, server.DisableSignatureVerification())
	url := txh.MakeWebSocketURL("events")
	dialers := make([]*websocket.Conn, numberToTest)
	for i := range dialers {
		dial, _, err := websocket.DefaultDialer.Dial(url, nil)
		assert.NilError(t, err)
		dialers[i] = dial
	}
	ctx := context.Background()
	waitForTicks := sync.WaitGroup{}
	waitForTicks.Add(1)
	go func() {
		defer waitForTicks.Done()
		for i := 0; i < numberToTest; i++ {
			err := w.Tick(ctx)
			assert.NilError(t, err)
		}
	}()

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
	waitForTicks.Wait()

	assert.Equal(t, counter1.Load(), int32(numberToTest*numberToTest))
	assert.Equal(t, counter2.Load(), int32(numberToTest*numberToTest))
}

func TestEventHubLogger(t *testing.T) {
	// replaces internal Logger with one that logs to the buf variable above.
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	cardinalLogger := ecslog.Logger{
		&bufLogger,
	}
	w := testutils.NewTestWorld(t, cardinal.WithLoggingEventHub(&cardinalLogger)).Instance()

	// testutils.NewTestWorld sets the log level to error, so we need to set it to zerolog.DebugLevel to pass this test
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	numberToTest := 5
	for i := 0; i < numberToTest; i++ {
		w.RegisterSystem(func(wCtx ecs.WorldContext) error {
			wCtx.GetWorld().EmitEvent(&events.Event{Message: "test"})
			return nil
		})
	}
	assert.NilError(t, w.LoadGameState())
	ctx := context.Background()
	for i := 0; i < numberToTest; i++ {
		err := w.Tick(ctx)
		assert.NilError(t, err)
	}
	testString := "{\"level\":\"info\",\"message\":\"EVENT: test\"}\n"
	eventsLogs := buf.String()
	splitLogs := strings.Split(eventsLogs, "\n")
	splitLogs = splitLogs[:len(splitLogs)-1]
	assert.Equal(t, 25, len(splitLogs))
	for _, logEntry := range splitLogs {
		require.JSONEq(t, testString, logEntry)
	}
}
