package events

import (
	"encoding/json"
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

// websocketAndDoneChan stores a websocket connection along with a channel to signal when something is done.
// it's sent by a webhandler into eventHub and the done channel is to allow eventHub to signal back into the web
// framework
type websocketAndDoneChan struct {
	connection *websocket.Conn
	doneChan   chan bool
}

type EventHub struct {
	websocketConnections   map[*websocket.Conn]bool
	broadcast              chan []byte
	getEventQueueLength    chan chan int
	getAmountOfConnections chan chan int
	flush                  chan bool
	register               chan websocketAndDoneChan
	unregister             chan websocketAndDoneChan
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
		register:               make(chan websocketAndDoneChan),
		unregister:             make(chan websocketAndDoneChan),
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

func (eh *EventHub) EmitEvent(event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return eris.Wrap(err, "must use a json serializable type for emitting events")
	}
	eh.broadcast <- data
	return nil
}

func (eh *EventHub) FlushEvents() {
	eh.flush <- true
}

func (eh *EventHub) RegisterConnection(ws *websocket.Conn) {
	doneChan := make(chan bool)
	eh.register <- websocketAndDoneChan{
		connection: ws,
		doneChan:   doneChan,
	}
	<-doneChan
}

func (eh *EventHub) UnregisterConnection(ws *websocket.Conn) {
	doneChan := make(chan bool)
	eh.unregister <- websocketAndDoneChan{
		connection: ws,
		doneChan:   doneChan,
	}
	<-doneChan
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
		case websocketAndDoneChan := <-eh.register:
			conn := websocketAndDoneChan.connection
			doneChan := websocketAndDoneChan.doneChan
			eh.websocketConnections[conn] = true
			doneChan <- true
		case websocketAndDoneChan := <-eh.unregister:
			conn := websocketAndDoneChan.connection
			unregisterConnection(conn)
			websocketAndDoneChan.doneChan <- true
		case event := <-eh.broadcast:
			eh.eventQueue = append(eh.eventQueue, event)
		case <-eh.flush:
			var waitGroup sync.WaitGroup
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
							log.Logger.
								Error().
								Err(err).
								Msg("Connections were unregistered because of this error: " + eris.ToString(err, true))
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
					}
				}()
			}
			waitGroup.Wait()
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
		eh.RegisterConnection(conn)
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
