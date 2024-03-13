package events

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const shutdownPollInterval = 200

const (
	writeDeadline = 5 * time.Second
)

type EventHub struct {
	websocketConnections   map[*websocket.Conn]bool
	broadcast              chan []byte
	getEventQueueLength    chan chan int
	getAmountOfConnections chan chan int
	flush                  chan bool
	register               chan *websocket.Conn
	unregister             chan *websocket.Conn
	shutdown               chan bool
	eventQueue             [][]byte
	isRunning              atomic.Bool
}

func (eh *EventHub) EventQueueLength() int {
	lengthChan := make(chan int)
	eh.getEventQueueLength <- lengthChan
	return <-lengthChan
}

func (eh *EventHub) ConnectionAmount() int {
	connAmountChan := make(chan int)
	eh.getAmountOfConnections <- connAmountChan
	return <-connAmountChan
}

func NewEventHub() *EventHub {
	res := EventHub{
		websocketConnections:   map[*websocket.Conn]bool{},
		broadcast:              make(chan []byte),
		getEventQueueLength:    make(chan chan int),
		getAmountOfConnections: make(chan chan int),
		flush:                  make(chan bool),
		register:               make(chan *websocket.Conn),
		unregister:             make(chan *websocket.Conn),
		shutdown:               make(chan bool),
		eventQueue:             make([][]byte, 0),
		isRunning:              atomic.Bool{},
	}
	res.isRunning.Store(false)
	go func() {
		res.Run()
	}()
	return &res
}

func (eh *EventHub) EmitJSONEvent(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return eris.Wrap(err, "must use a json serializable type for emitting events")
	}
	eh.EmitEvent(data)
	return nil
}

func (eh *EventHub) EmitEvent(event []byte) {
	eh.broadcast <- event
}

func (eh *EventHub) FlushEvents() {
	eh.flush <- true
}

func (eh *EventHub) RegisterConnection(ws *websocket.Conn) {
	eh.register <- ws
}

func (eh *EventHub) UnregisterConnection(ws *websocket.Conn) {
	eh.unregister <- ws
}

func (eh *EventHub) Shutdown() {
	eh.shutdown <- true
	// block until the loop fully exits.
	for {
		if !eh.isRunning.Load() {
			break
		}
		time.Sleep(shutdownPollInterval * time.Millisecond)
	}
}

//nolint:gocognit
func (eh *EventHub) Run() {
	if eh.isRunning.Load() {
		return
	}
	eh.isRunning.Store(true)
	unregisterConnection := func(conn *websocket.Conn) {
		if _, ok := eh.websocketConnections[conn]; ok {
			delete(eh.websocketConnections, conn)
			err := eris.Wrap(conn.Close(), "")
			if err != nil {
				log.Logger.Error().Err(err).Msg(eris.ToString(err, true))
			}
		}
	}
Loop:
	for eh.isRunning.Load() {
		select {
		case connChan := <-eh.getAmountOfConnections:
			connChan <- len(eh.websocketConnections)
		case lengthChan := <-eh.getEventQueueLength:
			lengthChan <- len(eh.eventQueue)
		case conn := <-eh.register:
			eh.websocketConnections[conn] = true
			fmt.Printf("Registered websocket connection. Total %d\n", len(eh.websocketConnections))
		case conn := <-eh.unregister:
			unregisterConnection(conn)
		case event := <-eh.broadcast:
			eh.eventQueue = append(eh.eventQueue, event)
		case <-eh.flush:
			var waitGroup sync.WaitGroup
			fmt.Printf("amount of websocket connections: %d\n", len(eh.websocketConnections))
			fmt.Printf("amount of messages in event queue: %d\n", len(eh.eventQueue))
			acc := 0
			for conn := range eh.websocketConnections {
				waitGroup.Add(1)
				conn := conn
				go func() {
					defer waitGroup.Done()
					for _, event := range eh.eventQueue {
						err := eris.Wrap(conn.SetWriteDeadline(time.Now().Add(writeDeadline)), "")
						if err != nil {
							go func() {
								eh.UnregisterConnection(conn)
							}()
							log.Logger.Error().Err(err).Msg("Connections were unregistered because of this error: " + eris.ToString(err, true))
							acc -= 1
							break
						}
						err = eris.Wrap(conn.WriteMessage(websocket.TextMessage, event), "")
						if err != nil {
							go func() {
								eh.UnregisterConnection(conn)
							}()
							log.Logger.Error().Err(err).Msg(eris.ToString(err, true))
							break
						}
						acc += 1
					}
				}()
			}
			waitGroup.Wait()
			fmt.Printf("messages sent: %d\n", acc)
			eh.eventQueue = eh.eventQueue[:0]
		case <-eh.shutdown:
			go func() {
				for range eh.shutdown { //nolint:revive // This pattern drains the channel until closed
				}
			}()
			for conn := range eh.websocketConnections {
				unregisterConnection(conn)
			}
			break Loop
		}

	}
	eh.isRunning.Store(false)
}

func (eh *EventHub) NewWebSocketEventHandler() func(conn *websocket.Conn) {
	return func(conn *websocket.Conn) {
		fmt.Println("ws handler called registering connection!")
		eh.RegisterConnection(conn)
		fmt.Println("ws connection successfully registered.")
		var err error
		var mt int
		var msg []byte
		logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

		// error is swallowed here, the function signatures in fiber require this. Even the examples
		// swallow the error.
		for {
			if mt, msg, err = conn.ReadMessage(); err != nil {
				err = eris.Wrap(err, "")
				logger.Err(err).Msg("websocket read message failed")
				break
			}

			if err = conn.WriteMessage(mt, msg); err != nil {
				err = eris.Wrap(err, "")
				logger.Err(err).Msg("websocket write message failed")
				break
			}
		}
	}
}
