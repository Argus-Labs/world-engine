package ecs

type IQuery interface {
	Name() string
	// HandleQuery is given a reference to the world, a query request struct,
	// and is expected to return a JSON encoded response, or an error.
	HandleQuery(*World, []byte) ([]byte, error)
}

type Handler func(*World, []byte) ([]byte, error)

type QueryType struct {
	name    string
	handler Handler
}

func NewQueryType(name string, handler Handler) *QueryType {
	return &QueryType{name: name, handler: handler}
}

func (q *QueryType) Name() string {
	return q.name
}

func (q *QueryType) HandleQuery(w *World, req []byte) ([]byte, error) {
	return q.handler(w, req)
}
