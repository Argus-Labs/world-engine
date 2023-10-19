package ecs

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/invopop/jsonschema"
	"pkg.world.dev/world-engine/cardinal/ecs/abi"
)

type IQuery interface {
	// Name returns the name of the query.
	Name() string
	// HandleQuery handles queries with concrete types, rather than encoded bytes.
	HandleQuery(WorldContext, any) (any, error)
	// HandleQueryRaw is given a reference to the world, json encoded bytes that represent a query request
	// and is expected to return a json encoded response struct.
	HandleQueryRaw(WorldContext, []byte) ([]byte, error)
	// Schema returns the json schema of the query request.
	Schema() (request, reply *jsonschema.Schema)
	// DecodeEVMRequest decodes bytes originating from the evm into the request type, which will be ABI encoded.
	DecodeEVMRequest([]byte) (any, error)
	// EncodeEVMReply encodes the reply as an abi encoded struct.
	EncodeEVMReply(any) ([]byte, error)
	// DecodeEVMReply decodes EVM reply bytes, into the concrete go reply type.
	DecodeEVMReply([]byte) (any, error)
	// EncodeAsABI encodes a go struct in abi format. This is mostly used for testing.
	EncodeAsABI(any) ([]byte, error)
}

type QueryType[Request any, Reply any] struct {
	name       string
	handler    func(wCtx WorldContext, req Request) (Reply, error)
	requestABI *ethereumAbi.Type
	replyABI   *ethereumAbi.Type
}

func WithQueryEVMSupport[Request, Reply any]() func(transactionType *QueryType[Request, Reply]) {
	return func(query *QueryType[Request, Reply]) {
		var req Request
		var rep Reply
		reqABI, err := abi.GenerateABIType(req)
		if err != nil {
			panic(err)
		}
		repABI, err := abi.GenerateABIType(rep)
		if err != nil {
			panic(err)
		}
		query.requestABI = reqABI
		query.replyABI = repABI
	}
}

var _ IQuery = NewQueryType[struct{}, struct{}]("", nil)

func NewQueryType[Request any, Reply any](
	name string,
	handler func(wCtx WorldContext, req Request) (Reply, error),
	opts ...func() func(queryType *QueryType[Request, Reply]),
) *QueryType[Request, Reply] {
	var req Request
	var rep Reply
	reqType := reflect.TypeOf(req)
	reqKind := reqType.Kind()
	reqValid := false
	if (reqKind == reflect.Pointer && reqType.Elem().Kind() == reflect.Struct) || reqKind == reflect.Struct {
		reqValid = true
	}
	repType := reflect.TypeOf(rep)
	repKind := reqType.Kind()
	repValid := false
	if (repKind == reflect.Pointer && repType.Elem().Kind() == reflect.Struct) || repKind == reflect.Struct {
		repValid = true
	}

	if !repValid || !reqValid {
		panic(fmt.Sprintf("Invalid QueryType: %s: The Request and Reply must be both structs", name))
	}
	r := &QueryType[Request, Reply]{
		name:    name,
		handler: handler,
	}
	for _, opt := range opts {
		opt()(r)
	}
	return r
}

func (r *QueryType[Request, Reply]) generateABIBindings() error {
	var req Request
	reqABI, err := abi.GenerateABIType(req)
	if err != nil {
		return fmt.Errorf("error generating request ABI binding: %w", err)
	}
	var rep Reply
	repABI, err := abi.GenerateABIType(rep)
	if err != nil {
		return fmt.Errorf("error generating reply ABI binding: %w", err)
	}
	r.requestABI = reqABI
	r.replyABI = repABI
	return nil
}

func (r *QueryType[req, rep]) Name() string {
	return r.name
}

func (r *QueryType[req, rep]) Schema() (request, reply *jsonschema.Schema) {
	return jsonschema.Reflect(new(req)), jsonschema.Reflect(new(rep))
}

func (r *QueryType[req, rep]) HandleQuery(wCtx WorldContext, a any) (any, error) {
	request, ok := a.(req)
	if !ok {
		return nil, fmt.Errorf("cannot cast %T to this query request type %T", a, new(req))
	}
	reply, err := r.handler(wCtx, request)
	return reply, err
}

func (r *QueryType[req, rep]) HandleQueryRaw(wCtx WorldContext, bz []byte) ([]byte, error) {
	request := new(req)
	err := json.Unmarshal(bz, request)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal query request into type %T: %w", *request, err)
	}
	res, err := r.handler(wCtx, *request)
	if err != nil {
		return nil, err
	}
	bz, err = json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal response %T: %w", res, err)
	}
	return bz, nil
}

func (r *QueryType[req, rep]) DecodeEVMRequest(bz []byte) (any, error) {
	if r.requestABI == nil {
		return nil, ErrEVMTypeNotSet
	}
	args := ethereumAbi.Arguments{{Type: *r.requestABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, errors.New("error decoding EVM bytes: no values could be unpacked")
	}
	request, err := abi.SerdeInto[req](unpacked[0])
	if err != nil {
		return nil, err
	}
	return request, nil
}

func (r *QueryType[req, rep]) DecodeEVMReply(bz []byte) (any, error) {
	if r.replyABI == nil {
		return nil, ErrEVMTypeNotSet
	}
	args := ethereumAbi.Arguments{{Type: *r.replyABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, errors.New("error decoding EVM bytes: no values could be unpacked")
	}
	reply, err := abi.SerdeInto[rep](unpacked[0])
	if err != nil {
		return nil, err
	}
	return reply, nil
}

func (r *QueryType[req, rep]) EncodeEVMReply(a any) ([]byte, error) {
	if r.replyABI == nil {
		return nil, ErrEVMTypeNotSet
	}
	args := ethereumAbi.Arguments{{Type: *r.replyABI}}
	bz, err := args.Pack(a)
	return bz, err
}

func (r *QueryType[Request, Reply]) EncodeAsABI(input any) ([]byte, error) {
	if r.requestABI == nil || r.replyABI == nil {
		return nil, ErrEVMTypeNotSet
	}

	var args ethereumAbi.Arguments
	var in any
	switch input.(type) {
	case Request:
		in = input.(Request)
		args = ethereumAbi.Arguments{{Type: *r.requestABI}}
	case Reply:
		in = input.(Reply)
		args = ethereumAbi.Arguments{{Type: *r.replyABI}}
	default:
		return nil, fmt.Errorf("expected the input struct to be either %T or %T, but got %T",
			new(Request), new(Reply), input)
	}

	bz, err := args.Pack(in)
	if err != nil {
		return nil, err
	}
	return bz, nil
}
