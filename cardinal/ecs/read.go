package ecs

type IRead interface {
	// Name returns the name of the read.
	Name() string
	// HandleRead is given a reference to the world, json encoded bytes that represent a read request
	// and is expected to return a json encoded response struct.
	HandleRead(*World, []byte) ([]byte, error)
}

// Handler represent a function that handles a read request, and returns a read response.
type Handler func(*World, []byte) ([]byte, error)

type ReadType struct {
	name    string
	handler Handler
}

var _ IRead = &ReadType{}

func NewReadType(name string, handler Handler) IRead {
	return &ReadType{name: name, handler: handler}
}

func (r *ReadType) Name() string {
	return r.name
}

func (r *ReadType) HandleRead(w *World, req []byte) ([]byte, error) {
	return r.handler(w, req)
}
