package ecs

import (
	"encoding/json"
	"reflect"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs/abi"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type Query interface {
	// Name returns the name of the query.
	Name() string
	// Group returns the group of the query.
	Group() string
	// HandleQuery handles queries with concrete types, rather than encoded bytes.
	HandleQuery(engine.Context, any) (any, error)
	// HandleQueryRaw is given a reference to the engine, json encoded bytes that represent a query request
	// and is expected to return a json encoded response struct.
	HandleQueryRaw(engine.Context, []byte) ([]byte, error)
	// DecodeEVMRequest decodes bytes originating from the evm into the request type, which will be ABI encoded.
	DecodeEVMRequest([]byte) (any, error)
	// EncodeEVMReply encodes the reply as an abi encoded struct.
	EncodeEVMReply(any) ([]byte, error)
	// DecodeEVMReply decodes EVM reply bytes, into the concrete go reply type.
	DecodeEVMReply([]byte) (any, error)
	// EncodeAsABI encodes a go struct in abi format. This is mostly used for testing.
	EncodeAsABI(any) ([]byte, error)
	// IsEVMCompatible reports if the query is able to be sent from the EVM.
	IsEVMCompatible() bool
}

type QueryType[Request any, Reply any] struct {
	name       string
	group      string
	handler    func(eCtx engine.Context, req *Request) (*Reply, error)
	requestABI *ethereumAbi.Type
	replyABI   *ethereumAbi.Type
}

func WithQueryEVMSupport[Request, Reply any]() QueryOption[Request, Reply] {
	return func(qt *QueryType[Request, Reply]) {
		if err := qt.generateABIBindings(); err != nil {
			panic(err)
		}
	}
}

// WithCustomQueryGroup sets a custom group for the query.
// By default, queries are registered under the "game" group which maps it to the /query/game/:queryType route.
// This option allows you to set a custom group, which allow you to register the query
// under /query/<custom_group>/:queryType.
func WithCustomQueryGroup[Request, Reply any](group string) QueryOption[Request, Reply] {
	return func(qt *QueryType[Request, Reply]) {
		qt.group = group
	}
}

type QueryOption[Request, Reply any] func(qt *QueryType[Request, Reply])

var _ Query = &QueryType[struct{}, struct{}]{}

func NewQueryType[Request any, Reply any](
	name string,
	handler func(eCtx engine.Context, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) (Query, error) {
	err := validateQuery[Request, Reply](name, handler)
	if err != nil {
		return nil, err
	}
	r := &QueryType[Request, Reply]{
		name:    name,
		group:   "game",
		handler: handler,
	}
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

func (r *QueryType[Request, Reply]) IsEVMCompatible() bool {
	return r.requestABI != nil && r.replyABI != nil
}

func (r *QueryType[Request, Reply]) generateABIBindings() error {
	var req Request
	reqABI, err := abi.GenerateABIType(req)
	if err != nil {
		return eris.Wrap(err, "error generating request ABI binding")
	}
	var rep Reply
	repABI, err := abi.GenerateABIType(rep)
	if err != nil {
		return eris.Wrap(err, "error generating reply ABI binding")
	}
	r.requestABI = reqABI
	r.replyABI = repABI
	return nil
}

func (r *QueryType[req, rep]) Name() string {
	return r.name
}

func (r *QueryType[req, rep]) Group() string {
	return r.group
}

func (r *QueryType[req, rep]) HandleQuery(eCtx engine.Context, a any) (any, error) {
	request, ok := a.(req)
	if !ok {
		return nil, eris.Errorf("cannot cast %T to this query request type %T", a, new(req))
	}
	reply, err := r.handler(eCtx, &request)
	return reply, err
}

func (r *QueryType[req, rep]) HandleQueryRaw(eCtx engine.Context, bz []byte) ([]byte, error) {
	request := new(req)
	err := json.Unmarshal(bz, request)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to unmarshal query request into type %T", *request)
	}
	res, err := r.handler(eCtx, request)
	if err != nil {
		return nil, err
	}
	bz, err = json.Marshal(res)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to marshal response %T", res)
	}
	return bz, nil
}

func (r *QueryType[req, rep]) DecodeEVMRequest(bz []byte) (any, error) {
	if r.requestABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "")
	}
	args := ethereumAbi.Arguments{{Type: *r.requestABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	if len(unpacked) < 1 {
		return nil, eris.New("error decoding EVM bytes: no values could be unpacked")
	}
	request, err := abi.SerdeInto[req](unpacked[0])
	if err != nil {
		return nil, err
	}
	return request, nil
}

func (r *QueryType[req, rep]) DecodeEVMReply(bz []byte) (any, error) {
	if r.replyABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "")
	}
	args := ethereumAbi.Arguments{{Type: *r.replyABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, eris.New("error decoding EVM bytes: no values could be unpacked")
	}
	reply, err := abi.SerdeInto[rep](unpacked[0])
	if err != nil {
		return nil, err
	}
	return reply, nil
}

func (r *QueryType[req, rep]) EncodeEVMReply(a any) ([]byte, error) {
	if r.replyABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "")
	}
	args := ethereumAbi.Arguments{{Type: *r.replyABI}}
	bz, err := args.Pack(a)
	return bz, eris.Wrap(err, "")
}

func (r *QueryType[Request, Reply]) EncodeAsABI(input any) ([]byte, error) {
	if r.requestABI == nil || r.replyABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "")
	}

	var args ethereumAbi.Arguments
	var in any
	//nolint:gocritic // its fine.
	switch ty := input.(type) {
	case Request:
		in = ty
		args = ethereumAbi.Arguments{{Type: *r.requestABI}}
	case Reply:
		in = ty
		args = ethereumAbi.Arguments{{Type: *r.replyABI}}
	default:
		return nil, eris.Errorf("expected the input struct to be either %T or %T, but got %T",
			new(Request), new(Reply), input)
	}

	bz, err := args.Pack(in)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bz, nil
}

func validateQuery[Request any, Reply any](
	name string,
	handler func(eCtx engine.Context, req *Request) (*Reply, error),
) error {
	if name == "" {
		return eris.New("cannot create query without name")
	}
	if handler == nil {
		return eris.New("cannot create query without handler")
	}

	var req Request
	var rep Reply
	reqType := reflect.TypeOf(req)
	reqKind := reqType.Kind()
	reqValid := false
	if (reqKind == reflect.Pointer && reqType.Elem().Kind() == reflect.Struct) ||
		reqKind == reflect.Struct {
		reqValid = true
	}
	repType := reflect.TypeOf(rep)
	repKind := reqType.Kind()
	repValid := false
	if (repKind == reflect.Pointer && repType.Elem().Kind() == reflect.Struct) ||
		repKind == reflect.Struct {
		repValid = true
	}

	if !repValid || !reqValid {
		return eris.Errorf(
			"invalid query: %s: the Request and Reply generics must be both structs",
			name,
		)
	}
	return nil
}
