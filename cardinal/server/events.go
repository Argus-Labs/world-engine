package server

import (
	"net/http"
	"time"

	"github.com/go-openapi/runtime/middleware"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func CreateEventHub() *EventHub {
	res := EventHub{
		websocket_connections: map[*websocket.Conn]bool{},
		Broadcast:             make(chan []byte),
		Register:              make(chan *websocket.Conn),
		Unregister:            make(chan *websocket.Conn),
		Shutdown:              make(chan bool),
	}
	return &res
}

type Event struct {
	message string
}

type EventHub struct {
	websocket_connections map[*websocket.Conn]bool
	Broadcast             chan []byte
	Unregister            chan *websocket.Conn
	Register              chan *websocket.Conn
	Shutdown              chan bool
}

func (h *EventHub) run() {
Loop:
	for {
		select {
		case conn := <-h.Register:
			h.websocket_connections[conn] = true
		case conn := <-h.Unregister:
			if _, ok := h.websocket_connections[conn]; ok {
				delete(h.websocket_connections, conn)
				err := conn.Close()
				if err != nil {
					log.Logger.Error().Err(err)
				}
			}
		case message := <-h.Broadcast:
			for conn := range h.websocket_connections {

				err := conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
				if err != nil {
					go func() {
						h.Unregister <- conn
					}()
					log.Logger.Error().Err(err)
					break Loop
				}
				err = conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					go func() {
						h.Unregister <- conn
					}()
					log.Logger.Error().Err(err)
				}

			}
		case <-h.Shutdown:
			go func() {
				for conn, _ := range h.websocket_connections {
					h.Unregister <- conn
				}
			}()
			break Loop
		}
	}
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

func CreateWebSocketEventHandler(hub *EventHub) func(conn *websocket.Conn) error {
	return func(conn *websocket.Conn) error {
		hub.Register <- conn

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
