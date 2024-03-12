package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"pkg.world.dev/world-engine/relay/nakama/receipt"
	"sync"
	"sync/atomic"
	"time"

	"pkg.world.dev/world-engine/relay/nakama/utils"

	"github.com/gorilla/websocket"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

type EventHub struct {
	inputConnection *websocket.Conn
	channels        *sync.Map // map[string]chan []byte
	didShutdown     atomic.Bool
}

type TickResults struct {
	Tick     uint64
	Receipts []receipt.Receipt
	Events   [][]byte
}

func CreateEventHub(logger runtime.Logger, eventsEndpoint string, cardinalAddress string) (*EventHub, error) {
	url := utils.MakeWebSocketURL(eventsEndpoint, cardinalAddress)
	webSocketConnection, _, err := websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose // no need.
	for err != nil {
		if errors.Is(err, &net.DNSError{}) {
			// sleep a little try again...
			logger.Info("No host found.")
			logger.Info(err.Error())
			time.Sleep(2 * time.Second)                                          //nolint:gomnd // its ok.
			webSocketConnection, _, err = websocket.DefaultDialer.Dial(url, nil) //nolint:bodyclose // no need.
		} else {
			return nil, eris.Wrap(err, "")
		}
	}
	channelMap := sync.Map{}
	res := EventHub{
		inputConnection: webSocketConnection,
		channels:        &channelMap,
		didShutdown:     atomic.Bool{},
	}
	res.didShutdown.Store(false)
	return &res, nil
}

func (eh *EventHub) Subscribe(session string) chan []byte {
	channel := make(chan []byte)
	eh.channels.Store(session, channel)
	return channel
}

func (eh *EventHub) Unsubscribe(session string) {
	eventChannelUntyped, ok := eh.channels.Load(session)
	if !ok {
		panic(eris.New("session not found"))
	}
	eventChannel, ok := eventChannelUntyped.(chan []byte)
	if !ok {
		panic(eris.New("found object that was not a event channel in event hub"))
	}
	close(eventChannel)
	eh.channels.Delete(session)
}

func (eh *EventHub) Shutdown() {
	eh.didShutdown.Store(true)
}

// Dispatch continually drains eh.inputConnection (events from cardinal) and sends copies to all subscribed channels.
// This function is meant to be called in a goroutine.
func (eh *EventHub) Dispatch(log runtime.Logger) error {
	var err error
	for !eh.didShutdown.Load() {
		messageType, message, err := eh.inputConnection.ReadMessage() // will block
		if err != nil {
			err = eris.Wrap(err, "")
			eh.Shutdown()
			continue
		}
		if messageType != websocket.TextMessage {
			eh.Shutdown()
			continue
		}
		receivedTickResults := TickResults{}
		err = json.Unmarshal(message, &receivedTickResults)
		if err != nil {
			fmt.Println("BIG ERROR: ", err)
		}

		eh.channels.Range(func(_ any, value any) bool {
			channel, ok := value.(chan []byte)
			if !ok {
				err = eris.New("not a channel")
				eh.Shutdown()
				return false
			}

			for i := 0; i < len(receivedTickResults.Events); i++ {
				channel <- receivedTickResults.Events[i]
			}

			return true
		})
		if err != nil {
			eh.Shutdown()
			continue
		}
	}
	eh.channels.Range(func(key any, _ any) bool {
		log.Info(fmt.Sprintf("shutting down: %s", key.(string)))
		eh.Unsubscribe(key.(string))
		return true
	})
	err = errors.Join(eris.Wrap(eh.inputConnection.Close(), ""), err)
	return err
}
