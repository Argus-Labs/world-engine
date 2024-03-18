package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/relay/nakama/utils"
)

type EventHub struct {
	inputConnection *websocket.Conn
	channels        *sync.Map // map[string]chan []byte or []Receipt
	didShutdown     atomic.Bool
}

type TickResults struct {
	Tick     uint64
	Receipts []Receipt
	Events   [][]byte
}

func NewEventHub(logger runtime.Logger, eventsEndpoint string, cardinalAddress string) (*EventHub, error) {
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

func (eh *EventHub) SubscribeToEvents(session string) chan []byte {
	channel := make(chan []byte)
	eh.channels.Store(session, channel)
	return channel
}

func (eh *EventHub) SubscribeToReceipts(session string) chan []Receipt {
	channel := make(chan []Receipt)
	eh.channels.Store(session, channel)
	return channel
}

func (eh *EventHub) Unsubscribe(session string) {
	eventChannelUntyped, ok := eh.channels.Load(session)
	if !ok {
		panic(eris.New("session not found"))
	}

	switch ch := eventChannelUntyped.(type) {
	case chan []byte:
		close(ch)
	case chan []Receipt:
		close(ch)
	default:
		panic(eris.New("found object that was not a recognized channel type in event hub"))
	}

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
		var messageType int
		var message []byte
		messageType, message, err = eh.inputConnection.ReadMessage() // will block
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
			log.Error("unable to unmarshal message into TickResults: ", err)
			continue
		}

		eh.channels.Range(func(_ any, value any) bool {
			switch ch := value.(type) {
			case chan []byte:
				for _, e := range receivedTickResults.Events {
					ch <- e
				}
			case chan []Receipt:
				ch <- receivedTickResults.Receipts
			default:
				log.Warn("Found an unhandled channel type")
			}

			return true
		})
	}
	eh.channels.Range(func(key any, _ any) bool {
		log.Info(fmt.Sprintf("shutting down: %s", key.(string)))
		eh.Unsubscribe(key.(string))
		return true
	})
	err = errors.Join(eris.Wrap(eh.inputConnection.Close(), ""), err)
	return err
}
