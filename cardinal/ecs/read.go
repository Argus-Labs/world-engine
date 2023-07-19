package ecs

import "encoding/json"

type IRead interface {
	// Name returns the name of the read.
	Name() string
	// HandleRead is given a reference to the world, json encoded bytes that represent a read request
	// and is expected to return a json encoded response struct.
	HandleRead(*World, []byte) ([]byte, error)
	// Schema returns the json schema of the read request.
	Schema() string
}

// Handler represent a function that handles a read request, and returns a read response.
type Handler func(*World, []byte) ([]byte, error)

type ReadType[T any] struct {
	name    string
	schema  string
	handler Handler
}

var _ IRead = NewReadType[struct{}]("", *new(Handler))

func NewReadType[T any](name string, handler Handler) *ReadType[T] {
	jsonSchema, err := json.Marshal(new(T))
	if err != nil {
		panic(err)
	}

	return &ReadType[T]{
		name:    name,
		schema:  string(jsonSchema),
		handler: handler,
	}
}

func (r *ReadType[T]) Name() string {
	return r.name
}

func (r *ReadType[T]) Schema() string {
	return r.schema
}

func (r *ReadType[T]) HandleRead(w *World, req []byte) ([]byte, error) {
	return r.handler(w, req)
}
