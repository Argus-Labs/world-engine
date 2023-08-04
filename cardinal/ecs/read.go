package ecs

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/invopop/jsonschema"
)

type IRead interface {
	// Name returns the name of the read.
	Name() string
	// HandleRead is given a reference to the world, json encoded bytes that represent a read request
	// and is expected to return a json encoded response struct.
	HandleRead(*World, []byte) ([]byte, error)
	// Schema returns the json schema of the read request.
	Schema() *jsonschema.Schema
}

type ReadType[Request any, Reply any] struct {
	name       string
	handler    func(world *World, req Request) (Reply, error)
	requestABI *abi.Type
	replyABI   *abi.Type
}

var _ IRead = NewReadType[struct{}, struct{}]("", nil, false)

func NewReadType[Request any, Reply any](
	name string,
	handler func(world *World, req Request) (Reply, error),
	supportEvm bool,
) *ReadType[Request, Reply] {
	r := &ReadType[Request, Reply]{
		name:    name,
		handler: handler,
	}
	if supportEvm {
		err := r.generateABIBindings()
		if err != nil {
			panic(err)
		}
	}
	return r
}

func (r *ReadType[Request, Reply]) generateABIBindings() error {
	var req Request
	reqABI, err := GenerateABIType(req)
	if err != nil {
		return fmt.Errorf("error generating request ABI binding: %w", err)
	}
	var rep Reply
	repABI, err := GenerateABIType(rep)
	if err != nil {
		return fmt.Errorf("error generating reply ABI binding: %w", err)
	}
	r.requestABI = reqABI
	r.replyABI = repABI
	return nil
}

func (r *ReadType[req, rep]) Name() string {
	return r.name
}

func (r *ReadType[req, rep]) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(new(req))
}

func (r *ReadType[req, rep]) HandleRead(w *World, bz []byte) ([]byte, error) {
	t := new(req)
	err := json.Unmarshal(bz, t)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal read request into type %T: %w", *t, err)
	}
	res, err := r.handler(w, *t)
	if err != nil {
		return nil, err
	}
	bz, err = json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal response %T: %w", res, err)
	}
	return bz, nil
}
