package server

import (
	"net/http"
	"time"

	"github.com/go-openapi/runtime/middleware"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type webSocketHandler struct {
	internalServe func(*websocket.Conn) error
	path          string
	parentHandler http.Handler
	upgrader      websocket.Upgrader
}

func (w *webSocketHandler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	if request.URL.Path == w.path {
		ws, err := w.upgrader.Upgrade(responseWriter, request, nil)
		if err != nil {
			//Do some error.
		} else {
			err = w.internalServe(ws)
			if err != nil {

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

var WebSocketEchoBuilder = CreateNewWebSocketBuilder("/echo", webSocketEchoHandler)

func webSocketEchoHandler(conn *websocket.Conn) error {
	messageType, p, err := conn.ReadMessage()
	if err != nil {
		//Do some error
	}
	log.Info().Msg(string(p))
	for {
		time.Sleep(2 * time.Second)
		if err = conn.WriteMessage(messageType, p); err != nil {
			if websocket.IsUnexpectedCloseError(err) {
				return nil
			}
			return err
		}
	}
}
