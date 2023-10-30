package main

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/heroiclabs/nakama-common/runtime"
)

type Event struct {
	message string
}

type EventHub struct {
	inputConnection *websocket.Conn
	channels        *sync.Map //map[string]chan *Event
	didShutdown     atomic.Bool
}

func createEventHub(logger runtime.Logger) (*EventHub, error) {
	url := makeWebSocketURL(eventEndpoint)
	fmt.Println(url)
	webSocketConnection, _, err := websocket.DefaultDialer.Dial(url, nil)
	for err != nil {
		if errors.Is(err, &net.DNSError{}) {
			//sleep a little try again...
			logger.Info("No host found.")
			logger.Info(err.Error())
			time.Sleep(2 * time.Second)
			webSocketConnection, _, err = websocket.DefaultDialer.Dial(url, nil)
		} else {
			return nil, err
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

func (eh *EventHub) subscribe(session string) chan *Event {
	channel := make(chan *Event)
	eh.channels.Store(session, channel)
	return channel
}

func (eh *EventHub) unsubscribe(session string) {
	eventChannelUntyped, ok := eh.channels.Load(session)
	if !ok {
		panic(errors.New("session not found"))
	}
	eventChannel, ok := eventChannelUntyped.(chan *Event)
	if !ok {
		panic(errors.New("found object that was not a event channel in event hub"))
	}
	close(eventChannel)
	eh.channels.Delete(session)
}

func (eh *EventHub) shutdown() {
	eh.didShutdown.Store(true)
}

// dispatch continually drains eh.inputConnection (events from cardinal) and sends copies to all subscribed channels.
// This function is meant to be called in a goroutine.
func (eh *EventHub) dispatch(log runtime.Logger) error {
	var err error
	for !eh.didShutdown.Load() {
		messageType, message, err := eh.inputConnection.ReadMessage() //will block
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage {
			break
		}
		eh.channels.Range(func(key any, value any) bool {
			channel, ok := value.(chan *Event)
			if !ok {
				err = errors.New("not a channel")
				return false
			}
			channel <- &Event{message: string(message)}
			return true
		})
		if err != nil {
			break
		}
	}
	eh.didShutdown.Store(true)
	eh.channels.Range(func(key any, value any) bool {
		log.Info(fmt.Sprintf("shutting down: %s", key.(string)))
		channel := value.(chan *Event)
		close(channel)
		return true
	})
	err = errors.Join(eh.inputConnection.Close(), err)
	return err
}
