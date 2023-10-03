package read

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/invopop/jsonschema"
	"pkg.world.dev/world-engine/cardinal/ecs/ecs_abi"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/public"
)

type ReadType[Request any, Reply any] struct {
	name       string
	handler    func(world public.IWorld, req Request) (Reply, error)
	requestABI *abi.Type
	replyABI   *abi.Type
}

func WithReadEVMSupport[Request, Reply any]() func(transactionType *ReadType[Request, Reply]) {
	return func(read *ReadType[Request, Reply]) {
		var req Request
		var rep Reply
		reqABI, err := ecs_abi.GenerateABIType(req)
		if err != nil {
			panic(err)
		}
		repABI, err := ecs_abi.GenerateABIType(rep)
		if err != nil {
			panic(err)
		}
		read.requestABI = reqABI
		read.replyABI = repABI
	}
}

var _ public.IRead = NewReadType[struct{}, struct{}]("", nil)

func NewReadType[Request any, Reply any](
	name string,
	handler func(world public.IWorld, req Request) (Reply, error),
	opts ...func() func(readType *ReadType[Request, Reply]),
) *ReadType[Request, Reply] {
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
		panic(fmt.Sprintf("Invalid ReadType: %s: The Request and Reply must be both structs", name))
	}
	r := &ReadType[Request, Reply]{
		name:    name,
		handler: handler,
	}
	for _, opt := range opts {
		opt()(r)
	}
	return r
}

func (r *ReadType[Request, Reply]) generateABIBindings() error {
	var req Request
	reqABI, err := ecs_abi.GenerateABIType(req)
	if err != nil {
		return fmt.Errorf("error generating request ABI binding: %w", err)
	}
	var rep Reply
	repABI, err := ecs_abi.GenerateABIType(rep)
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

func (r *ReadType[req, rep]) Schema() (request, reply *jsonschema.Schema) {
	return jsonschema.Reflect(new(req)), jsonschema.Reflect(new(rep))
}

func (r *ReadType[req, rep]) HandleRead(world public.IWorld, a any) (any, error) {
	request, ok := a.(req)
	if !ok {
		return nil, fmt.Errorf("cannot cast %T to this reads request type %T", a, new(req))
	}
	reply, err := r.handler(world, request)
	return reply, err
}

func (r *ReadType[req, rep]) HandleReadRaw(w public.IWorld, bz []byte) ([]byte, error) {
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
		return nil, transaction.ErrEVMTypeNotSet
	}
	args := abi.Arguments{{Type: *r.requestABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, errors.New("error decoding EVM bytes: no values could be unpacked")
	}
	request, err := ecs_abi.SerdeInto[req](unpacked[0])
	if err != nil {
		return nil, err
	}
	return request, nil
}

func (r *ReadType[req, rep]) DecodeEVMReply(bz []byte) (any, error) {
	if r.replyABI == nil {
		return nil, transaction.ErrEVMTypeNotSet
	}
	args := abi.Arguments{{Type: *r.replyABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, errors.New("error decoding EVM bytes: no values could be unpacked")
	}
	reply, err := ecs_abi.SerdeInto[rep](unpacked[0])
	if err != nil {
		return nil, err
	}
	return reply, nil
}

func (r *ReadType[req, rep]) EncodeEVMReply(a any) ([]byte, error) {
	if r.replyABI == nil {
		return nil, transaction.ErrEVMTypeNotSet
	}
	args := abi.Arguments{{Type: *r.replyABI}}
	bz, err := args.Pack(a)
	return bz, err
}

func (r *ReadType[Request, Reply]) EncodeAsABI(input any) ([]byte, error) {
	if r.requestABI == nil || r.replyABI == nil {
		return nil, transaction.ErrEVMTypeNotSet
	}

	var args abi.Arguments
	var in any
	switch input.(type) {
	case Request:
		in = input.(Request)
		args = abi.Arguments{{Type: *r.requestABI}}
	case Reply:
		in = input.(Reply)
		args = abi.Arguments{{Type: *r.replyABI}}
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
