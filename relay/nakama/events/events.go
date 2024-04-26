package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/relay/nakama/utils"
)

var (
	ErrEventHubIsShuttingDown = errors.New("event hub is shutting down")
)

type EventHub struct {
	inputConnection *websocket.Conn
	channels        *sync.Map // map[string]chan []byte or []Receipt
	connectMutex    *sync.Mutex
	didShutdown     bool
	wsURL           string
}

type TickResults struct {
	Tick     uint64
	Receipts []Receipt
	Events   [][]byte
}

func NewEventHub(logger runtime.Logger, eventsEndpoint string, cardinalAddress string) (*EventHub, error) {
	channelMap := sync.Map{}
	res := &EventHub{
		channels:     &channelMap,
		connectMutex: &sync.Mutex{},
		didShutdown:  false,
		wsURL:        utils.MakeWebSocketURL(eventsEndpoint, cardinalAddress),
	}
	if err := res.connectWithRetry(logger); err != nil {
		return nil, eris.Wrap(err, "failed to make initial websocket connection")
	}

	return res, nil
}

// connectWithRetry attempts to make a websocket connection. If Shutdown is called while this method is
// running ErrEventHubIsShuttingDown will be returned
func (eh *EventHub) connectWithRetry(logger runtime.Logger) error {
	for tries := 1; ; tries++ {
		if err := eh.establishConnection(); errors.Is(err, &net.DNSError{}) {
			// sleep a little try again...
			logger.Info("No host found: %v", err)
			time.Sleep(2 * time.Second) //nolint:gomnd // its ok.
			continue
		} else if err != nil {
			return eris.Wrapf(err, "failed to connect after %d attempts", tries)
		}

		// success!
		break
	}
	return nil
}

// establishConnection attempts to establish a connection to cardinal. A previous connection will be closed before
// attempting to dial again. If nil is returned, it means a connection has been made and is ready for use.
func (eh *EventHub) establishConnection() error {
	eh.connectMutex.Lock()
	defer eh.connectMutex.Unlock()
	if eh.didShutdown {
		return ErrEventHubIsShuttingDown
	}

	if eh.inputConnection != nil {
		if err := eh.inputConnection.Close(); err != nil {
			return eris.Wrap(err, "failed to close old connection")
		}
		eh.inputConnection = nil
	}
	webSocketConnection, _, err := websocket.DefaultDialer.Dial(eh.wsURL, nil) //nolint:bodyclose // no need.
	if err != nil {
		return eris.Wrap(err, "websocket dial failed")
	}
	eh.inputConnection = webSocketConnection
	return nil
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
	eh.connectMutex.Lock()
	defer eh.connectMutex.Unlock()
	if eh.didShutdown {
		return
	}
	eh.didShutdown = true
	if eh.inputConnection != nil {
		_ = eh.inputConnection.Close()
	}
}

// readMessage will block until a new message is available on the websocket. If any errors are encountered,
// the socket will be closed and a new connection will attempt to be established. This method blocks until
// a message has successfully been fetched, or until EventHub.Shutdown is called.
func (eh *EventHub) readMessage(log runtime.Logger) (messageType int, message []byte, err error) {
	for {
		messageType, message, err = eh.inputConnection.ReadMessage()
		if err != nil {
			log.Warn("read from websocket failed: %v", err)
			// Something went wrong. Try to reestablish the connection.
			if err = eh.connectWithRetry(log); err != nil {
				return 0, nil, eris.Wrap(err, "failed to reestablish a websocket connection")
			}
		} else {
			break
		}
	}
	return messageType, message, nil
}

// Dispatch continually drains eh.inputConnection (events from cardinal) and sends copies to all subscribed channels.
// This function is meant to be called in a goroutine.
func (eh *EventHub) Dispatch(log runtime.Logger) error {
	defer eh.Shutdown()
	defer func() {
		eh.channels.Range(func(key any, _ any) bool {
			log.Info(fmt.Sprintf("shutting down: %s", key.(string)))
			eh.Unsubscribe(key.(string))
			return true
		})
	}()
	for {
		messageType, message, err := eh.readMessage(log) // will block
		if errors.Is(err, ErrEventHubIsShuttingDown) {
			return nil
		} else if err != nil {
			return eris.Wrap(err, "")
		}
		if messageType != websocket.TextMessage {
			return eris.Errorf("unexpected message type %v on web socket", messageType)
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
}
