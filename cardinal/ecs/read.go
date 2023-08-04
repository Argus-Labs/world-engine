package ecs

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/invopop/jsonschema"
	"reflect"
)

type IRead interface {
	// Name returns the name of the read.
	Name() string
	// HandleRead handles reads with concrete types, rather than encoded bytes.
	HandleRead(*World, any) (any, error)
	// HandleReadRaw is given a reference to the world, json encoded bytes that represent a read request
	// and is expected to return a json encoded response struct.
	HandleReadRaw(*World, []byte) ([]byte, error)
	// Schema returns the json schema of the read request.
	Schema() *jsonschema.Schema
	// DecodeEVMRequest decodes bytes originating from the evm into the request type, which will be ABI encoded.
	DecodeEVMRequest([]byte) (any, error)
	// EncodeEVMReply encodes the reply as an abi encoded struct.
	EncodeEVMReply(any) ([]byte, error)
	// DecodeEVMReply decodes EVM reply bytes, into the concrete go reply type.
	DecodeEVMReply([]byte) (any, error)
	// EncodeRequestAsABI encodes a go struct in abi format. This is mostly used for testing.
	EncodeRequestAsABI(any) ([]byte, error)
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
	var request Request
	r.requestABI.TupleType = reflect.TypeOf(request)
	r.replyABI = repABI
	var reply Reply
	r.replyABI.TupleType = reflect.TypeOf(reply)
	return nil
}

func (r *ReadType[req, rep]) Name() string {
	return r.name
}

func (r *ReadType[req, rep]) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(new(req))
}

func (r *ReadType[req, rep]) HandleRead(world *World, a any) (any, error) {
	request, ok := a.(req)
	if !ok {
		return nil, fmt.Errorf("cannot cast %T to this reads request type %T", a, new(req))
	}
	reply, err := r.handler(world, request)
	return reply, err
}

func (r *ReadType[req, rep]) HandleReadRaw(w *World, bz []byte) ([]byte, error) {
	request := new(req)
	err := json.Unmarshal(bz, request)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal read request into type %T: %w", *request, err)
	}
	res, err := r.handler(w, *request)
	if err != nil {
		return nil, err
	}
	bz, err = json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal response %T: %w", res, err)
	}
	return bz, nil
}

func (r *ReadType[req, rep]) DecodeEVMRequest(bz []byte) (any, error) {
	if r.requestABI == nil {
		return nil, errors.New("cannot call DecodeEVMRequest without setting supportEVM to true when " +
			"creating the read")
	}
	args := abi.Arguments{{Type: *r.requestABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, errors.New("error decoding EVM bytes: no values could be unpacked")
	}
	underlying, ok := unpacked[0].(req)
	if !ok {
		return nil, fmt.Errorf("error decoding EVM bytes: cannot cast %T to %T", unpacked[0], new(req))
	}
	return underlying, nil
}

func (r *ReadType[req, rep]) DecodeEVMReply(bz []byte) (any, error) {
	if r.replyABI == nil {
		return nil, errors.New("cannot call DecodeEVMReply without setting supportEVM to true when " +
			"creating the read")
	}
	args := abi.Arguments{{Type: *r.replyABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, errors.New("error decoding EVM bytes: no values could be unpacked")
	}
	underlying, ok := unpacked[0].(rep)
	if !ok {
		return nil, fmt.Errorf("error decoding EVM bytes: cannot cast %T to %T", unpacked[0], new(req))
	}
	return underlying, nil
}

func (r *ReadType[req, rep]) EncodeEVMReply(a any) ([]byte, error) {
	if r.replyABI == nil {
		return nil, errors.New("cannot call EncodeEVMReply without setting supportEVM to true when " +
			"creating the read")
	}
	args := abi.Arguments{{Type: *r.replyABI}}
	bz, err := args.Pack(a)
	return bz, err
}

func (r *ReadType[Request, Reply]) EncodeRequestAsABI(req any) ([]byte, error) {
	if r.requestABI == nil {
		return nil, errors.New("cannot call EncodeRequestAsABI without setting supportEVM to true when " +
			"creating the read")
	}
	request, ok := req.(Request)
	if !ok {
		return nil, fmt.Errorf("expected the request struct %T, got %T", request, req)
	}
	args := abi.Arguments{{Type: *r.requestABI}}
	bz, err := args.Pack(request)
	if err != nil {
		return nil, err
	}
	return bz, nil
}
