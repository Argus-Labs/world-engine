package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/ecs"
)

const (
	// listEndpoint is a reserved endpoint used to inform consumers of the TransactionHandler's endpoints.
	listEndpoint = "/list"
)

type handler struct {
	fn   http.HandlerFunc
	path string
}

// TransactionHandler is a type that contains endpoints for transactions in a given ecs world.
type TransactionHandler struct {
	w        *ecs.World
	handlers []handler
}

// NewTransactionHandler returns a new TransactionHandler
func NewTransactionHandler(w *ecs.World) *TransactionHandler {
	return &TransactionHandler{w: w}
}

// fixes a path to contain a leading slash.
// if the path already contains a leading slash, it is simply returned as is.
func conformPath(p string) string {
	if p[0] != '/' {
		p = "/" + p
	}
	return p
}

// NewHandler builds a new http handler. path is the endpoint used to trigger the http handler function.
// path example: "move", "send_energy", "claim_planet".
// fn is the underlying function that handles the transaction. It should handle unmarshalling the JSON blob into
// the proper transaction type, as well as queuing it in the world.
func (t *TransactionHandler) NewHandler(path string, fn func(w *ecs.World) http.HandlerFunc) error {
	path = conformPath(path)
	if path == listEndpoint {
		return errors.New("endpoint 'list' is reserved by the cardinal system")
	}
	t.handlers = append(t.handlers, handler{
		fn:   fn(t.w),
		path: path,
	})
	return nil
}

// Serve sets up the endpoints passed in by the user, as well as a special "/list" endpoint, that informs consumers
// what endpoints the user set up in the TransactionHandler. Then, it serves the application, blocking the main thread.
// Please us `go txh.Serve(host,port)` if you do not want to block execution after calling this function.
func (t *TransactionHandler) Serve(host, port string) {
	paths := make([]string, len(t.handlers))
	for i, th := range t.handlers {
		paths[i] = th.path
		http.HandleFunc(th.path, th.fn)
	}
	http.HandleFunc(listEndpoint, func(w http.ResponseWriter, r *http.Request) {
		enc := json.NewEncoder(w)
		if err := enc.Encode(paths); err != nil {
			writeError(w, "cannot marshal list", err)
		}
	})
	err := http.ListenAndServe(fmt.Sprintf("%s:%s", host, port), nil)
	if err != nil {
		panic(err)
	}
}

func writeError(w http.ResponseWriter, msg string, err error) {
	w.WriteHeader(500)
	fmt.Fprintf(w, "%s: %v", msg, err)
}
