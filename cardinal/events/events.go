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

type EventHub struct {
	websocketConnections map[*websocket.Conn]bool
	broadcast            chan []byte
	flush                chan bool
	register             chan *websocket.Conn
	unregister           chan *websocket.Conn
	shutdown             chan bool
	eventQueue           [][]byte
	isRunning            atomic.Bool
}

func NewEventHub() *EventHub {
	res := EventHub{
		websocketConnections: map[*websocket.Conn]bool{},
		broadcast:            make(chan []byte),
		flush:                make(chan bool),
		register:             make(chan *websocket.Conn),
		unregister:           make(chan *websocket.Conn),
		shutdown:             make(chan bool),
		eventQueue:           make([][]byte, 0),
		isRunning:            atomic.Bool{},
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
		case conn := <-eh.register:
			eh.websocketConnections[conn] = true
		case conn := <-eh.unregister:
			unregisterConnection(conn)
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
							log.Logger.Error().Err(err).Msg(eris.ToString(err, true))
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
