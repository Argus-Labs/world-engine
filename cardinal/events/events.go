package events

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
)

type EventHub interface {
	EmitEvent(event *Event)
	FlushEvents()
	ShutdownEventHub()
	Run()
}

type loggingEventHub struct {
	logger     *ecslog.Logger
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
				acc := 0
				for _, event := range eh.eventQueue {
					eh.logger.Info().Msg(event.Message)
					acc += 1
				}
				fmt.Printf("called %d times\n", acc)
			}() //a goroutine is not technically necessary here but this imitates the websocket eventhub as much as possible.
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

func CreateLoggingEventHub(logger *ecslog.Logger) *loggingEventHub {
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

func CreateWebSocketEventHub() *WebSocketEventHub {
	res := WebSocketEventHub{
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

type WebSocketEventHub struct {
	websocketConnections map[*websocket.Conn]bool
	broadcast            chan *Event
	flush                chan bool
	unregister           chan *websocket.Conn
	register             chan *websocket.Conn
	shutdown             chan bool
	eventQueue           []*Event
	running              atomic.Bool
}

func (eh *WebSocketEventHub) EmitEvent(event *Event) {
	eh.broadcast <- event
}

func (eh *WebSocketEventHub) FlushEvents() {
	eh.flush <- true
}

func (eh *WebSocketEventHub) RegisterConnection(ws *websocket.Conn) {
	eh.register <- ws
}

func (eh *WebSocketEventHub) UnregisterConnection(ws *websocket.Conn) {
	eh.unregister <- ws
}

func (eh *WebSocketEventHub) ShutdownEventHub() {
	eh.shutdown <- true
}

func (eh *WebSocketEventHub) Run() {
	if eh.running.Load() {
		return
	}
	eh.running.Store(true)
	unregisterConnection := func(conn *websocket.Conn) {
		if _, ok := eh.websocketConnections[conn]; ok {
			delete(eh.websocketConnections, conn)
			err := conn.Close()
			if err != nil {
				log.Logger.Error().Err(err)
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
						err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
						if err != nil {
							go func() {
								eh.UnregisterConnection(conn)
							}()
							log.Logger.Error().Err(err)
							break
						}
						err = conn.WriteMessage(websocket.TextMessage, []byte(event.Message))
						if err != nil {
							go func() {
								eh.UnregisterConnection(conn)
							}()
							log.Logger.Error().Err(err)
							break
						}
					}
				}()
			}
			waitGroup.Wait()
			eh.eventQueue = eh.eventQueue[:0]
		case <-eh.shutdown:
			for conn, _ := range eh.websocketConnections {
				unregisterConnection(conn)
			}
			break Loop
		}
	}
	eh.running.Store(false)
}

type webSocketHandler struct {
	internalServe func(*websocket.Conn) error
	path          string
	parentHandler http.Handler
	upgrader      websocket.Upgrader
}

var upgrader = websocket.Upgrader{}

func (w *webSocketHandler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	if request.URL.Path == w.path {
		ws, err := w.upgrader.Upgrade(responseWriter, request, nil)
		if err != nil {
			//Do some error.
			responseWriter.WriteHeader(500)
		} else {
			err = w.internalServe(ws)
			if err != nil {
				responseWriter.WriteHeader(500)
			}
		}
	} else {
		w.parentHandler.ServeHTTP(responseWriter, request)
	}
}

func CreateNewWebSocketBuilder(path string, websocketConnectionHandler func(conn *websocket.Conn) error) middleware.Builder {
	return func(handler http.Handler) http.Handler {
		upgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
		res := webSocketHandler{
			internalServe: websocketConnectionHandler,
			path:          path,
			parentHandler: handler,
			upgrader:      upgrader,
		}
		return &res
	}
}

func CreateWebSocketEventHandler(hub *WebSocketEventHub) func(conn *websocket.Conn) error {
	return func(conn *websocket.Conn) error {
		hub.RegisterConnection(conn)
		return nil
	}
}

func WebSocketEchoHandler(ws *websocket.Conn) error {
	for {
		mt, message, err := ws.ReadMessage()
		if err != nil {
			log.Print("read:", err)
			return err
		}
		log.Printf("recv: %s", message)
		err = ws.WriteMessage(mt, message)
		if err != nil {
			log.Print("write:", err)
			return err
		}
	}
}

func Echo(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		w.WriteHeader(500)
	}
	err = WebSocketEchoHandler(c)
	err = c.Close()
	if err != nil {
		w.WriteHeader(500)
	}

}
