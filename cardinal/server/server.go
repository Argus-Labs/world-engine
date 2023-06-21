package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/ecs"
)

const (
	listEndpoint = "/list"
)

type handler struct {
	fn   http.HandlerFunc
	path string
}

type TransactionHandler struct {
	w        *ecs.World
	handlers []handler
}

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
