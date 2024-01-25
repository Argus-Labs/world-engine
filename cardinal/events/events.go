package events

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const shutdownPollInterval = 200

type EventHub interface {
	EmitEvent(event *Event)
	FlushEvents()
	ShutdownEventHub()
	Run()
	UnregisterConnection(ws *websocket.Conn)
	RegisterConnection(ws *websocket.Conn)
}

const (
	writeDeadline = 5 * time.Second
	bufferSize    = 1024
)

type loggingEventHub struct {
	logger     *zerolog.Logger
	eventQueue []*Event
	running    atomic.Bool
	broadcast  chan *Event
	shutdown   chan bool
	flush      chan bool
}

func (eh *loggingEventHub) EmitEvent(event *Event) {
	eh.broadcast <- event
}

func (eh *loggingEventHub) FlushEvents() {
	eh.flush <- true
}

func (eh *loggingEventHub) UnregisterConnection(_ *websocket.Conn) {}

func (eh *loggingEventHub) RegisterConnection(_ *websocket.Conn) {}

func (eh *loggingEventHub) Run() {
	if eh.running.Load() {
		return
	}
	eh.running.Store(true)
	for eh.running.Load() {
		select {
		case event := <-eh.broadcast:
			eh.eventQueue = append(eh.eventQueue, event)
		case <-eh.flush:
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, event := range eh.eventQueue {
					eh.logger.Info().Msg("EVENT: " + event.Message)
				}
			}() // a goroutine is not technically necessary here but this imitates the websocket eventhub as much as possible.
			wg.Wait()
			eh.eventQueue = eh.eventQueue[:0]
		case <-eh.shutdown:
			eh.running.Store(false)
		}
	}
}

func (eh *loggingEventHub) ShutdownEventHub() {
	eh.shutdown <- true
}

func NewLoggingEventHub(logger *zerolog.Logger) EventHub {
	res := loggingEventHub{
		eventQueue: make([]*Event, 0),
		running:    atomic.Bool{},
		broadcast:  make(chan *Event),
		shutdown:   make(chan bool),
		flush:      make(chan bool),
		logger:     logger,
	}
	res.running.Store(false)
	go func() {
		res.Run()
	}()
	return &res
}

func NewWebSocketEventHub() EventHub {
	res := webSocketEventHub{
		websocketConnections: map[*websocket.Conn]bool{},
		broadcast:            make(chan *Event),
		flush:                make(chan bool),
		register:             make(chan *websocket.Conn),
		unregister:           make(chan *websocket.Conn),
		shutdown:             make(chan bool),
		running:              atomic.Bool{},
	}
	res.running.Store(false)
	go func() {
		res.Run()
	}()
	return &res
}

type Event struct {
	Message string
}

type webSocketEventHub struct {
	websocketConnections map[*websocket.Conn]bool
	broadcast            chan *Event
	flush                chan bool
	unregister           chan *websocket.Conn
	register             chan *websocket.Conn
	shutdown             chan bool
	eventQueue           []*Event
	running              atomic.Bool
}

func (eh *webSocketEventHub) EmitEvent(event *Event) {
	eh.broadcast <- event
}

func (eh *webSocketEventHub) FlushEvents() {
	eh.flush <- true
}

func (eh *webSocketEventHub) RegisterConnection(ws *websocket.Conn) {
	eh.register <- ws
}

func (eh *webSocketEventHub) UnregisterConnection(ws *websocket.Conn) {
	eh.unregister <- ws
}

func (eh *webSocketEventHub) ShutdownEventHub() {
	eh.shutdown <- true
	// block until the loop fully exits.
	for {
		if !eh.running.Load() {
			break
		}
		time.Sleep(shutdownPollInterval * time.Millisecond)
	}
}

//nolint:gocognit
func (eh *webSocketEventHub) Run() {
	if eh.running.Load() {
		return
	}
	eh.running.Store(true)
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
	for eh.running.Load() {
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
						err = eris.Wrap(conn.WriteMessage(websocket.TextMessage, []byte(event.Message)), "")
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
	eh.running.Store(false)
}

func CreateWebSocketEventHandler(hub EventHub) func(conn *websocket.Conn) {
	return func(conn *websocket.Conn) {
		hub.RegisterConnection(conn)
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

func WebSocketEchoHandler(ws *websocket.Conn) error {
	if ws == nil {
		return eris.New("websocket connection cannot be nil")
	}
	for {
		mt, message, err := ws.ReadMessage()
		if err != nil {
			return eris.Wrap(err, "")
		}
		log.Printf("recv: %s", message)
		err = ws.WriteMessage(mt, message)
		if err != nil {
			return eris.Wrap(err, "")
		}
	}
}

func FiberWebSocketUpgrader(c *fiber.Ctx) error {
	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
		err := eris.Wrap(c.Next(), "")
		return err
	}
	return fiber.ErrUpgradeRequired
}
