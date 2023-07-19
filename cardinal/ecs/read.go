package ecs

import "github.com/invopop/jsonschema"

type IRead interface {
	// Name returns the name of the read.
	Name() string
	// HandleRead is given a reference to the world, json encoded bytes that represent a read request
	// and is expected to return a json encoded response struct.
	HandleRead(*World, []byte) ([]byte, error)
	// Schema returns the json schema of the read request.
	Schema() *jsonschema.Schema
}

// Handler represent a function that handles a read request, and returns a read response.
type Handler func(*World, []byte) ([]byte, error)

type ReadType[T any] struct {
	name    string
	handler Handler
}

var _ IRead = NewReadType[struct{}]("", nil)

func NewReadType[T any](name string, handler Handler) *ReadType[T] {
	return &ReadType[T]{
		name:    name,
		handler: handler,
	}
}

func (r *ReadType[T]) Name() string {
	return r.name
}

func (r *ReadType[T]) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(new(T))
}

func (r *ReadType[T]) HandleRead(w *World, req []byte) ([]byte, error) {
	return r.handler(w, req)
}
