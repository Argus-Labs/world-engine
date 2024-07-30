package cardinal

import (
	"encoding/json"
	"reflect"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/abi"
	"pkg.world.dev/world-engine/cardinal/types"
)

var _ query = &queryType[struct{}, struct{}]{}

var DefaultQueryGroup = "game"

type query interface {
	// Name returns the name of the query.
	Name() string
	// Group returns the group of the query.
	Group() string
	// IsEVMCompatible reports if the query is able to be sent from the EVM.
	IsEVMCompatible() bool
	// GetRequestFieldInformation returns a map of the fields of the query's request type and their types.
	GetRequestFieldInformation() map[string]any

	// handleQuery handles queries with concrete struct types, rather than encoded bytes.
	handleQuery(WorldContext, any) (any, error)
	// handleQueryJSON handles json-encoded query request and return a json-encoded response.
	handleQueryJSON(WorldContext, []byte) ([]byte, error)
	// handleQueryEVM handles ABI-encoded query request and return a ABI-encoded response.
	handleQueryEVM(WorldContext, []byte) ([]byte, error)
	// encodeEVMRequest encodes a go struct in ABI format. Used for testing.
	encodeEVMRequest(any) ([]byte, error)
	// decodeEVMReply decodes EVM reply bytes, into the concrete go reply type.
	decodeEVMReply([]byte) (any, error)
}

type QueryOption[Request, Reply any] func(qt *queryType[Request, Reply])

type queryType[Request any, Reply any] struct {
	name       string
	group      string
	handler    func(wCtx WorldContext, req *Request) (*Reply, error)
	requestABI *ethereumAbi.Type
	replyABI   *ethereumAbi.Type
}

func WithQueryEVMSupport[Request, Reply any]() QueryOption[Request, Reply] {
	return func(qt *queryType[Request, Reply]) {
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
	return func(qt *queryType[Request, Reply]) {
		qt.group = group
	}
}

func newQueryType[Request any, Reply any](
	name string,
	handler func(wCtx WorldContext, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) (query, error) {
	err := validateQuery[Request, Reply](name, handler)
	if err != nil {
		return nil, err
	}
	r := &queryType[Request, Reply]{
		name:    name,
		group:   DefaultQueryGroup,
		handler: handler,
	}
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

func (r *queryType[Request, Reply]) IsEVMCompatible() bool {
	return r.requestABI != nil && r.replyABI != nil
}

// generateABIBindings generates the ABI bindings used for encoding/decoding requests and replies.
func (r *queryType[Request, Reply]) generateABIBindings() error {
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

func (r *queryType[Request, Reply]) Name() string {
	return r.name
}

func (r *queryType[Request, Reply]) Group() string {
	return r.group
}

func (r *queryType[Request, Reply]) handleQuery(wCtx WorldContext, a any) (any, error) {
	var request *Request
	if reflect.TypeOf(a).Kind() == reflect.Pointer {
		ptrRequest, ok := a.(*Request)
		if !ok {
			return nil, eris.Errorf("cannot cast %T to this query request type %T", a, new(Request))
		}
		request = ptrRequest
	} else {
		valueReq, ok := a.(Request)
		if !ok {
			return nil, eris.Errorf("cannot cast %T to this query request type %T", a, new(Request))
		}
		request = &valueReq
	}
	reply, err := r.handler(wCtx, request)
	return reply, err
}

func (r *queryType[Request, Reply]) handleQueryJSON(wCtx WorldContext, bz []byte) ([]byte, error) {
	request := new(Request)
	err := json.Unmarshal(bz, request)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to unmarshal query request into type %T", *request)
	}

	res, err := r.handler(wCtx, request)
	if err != nil {
		return nil, err
	}

	bz, err = json.Marshal(res)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to marshal response %T", res)
	}

	return bz, nil
}

func (r *queryType[Request, Reply]) handleQueryEVM(wCtx WorldContext, bz []byte) ([]byte, error) {
	if !r.IsEVMCompatible() {
		return nil, eris.Errorf("query %s/%s is not EVM-compatible", r.Group(), r.Name())
	}

	req, err := r.decodeEVMRequest(bz)
	if err != nil {
		return nil, err
	}

	res, err := r.handler(wCtx, req)
	if err != nil {
		return nil, err
	}

	bz, err = r.encodeEVMReply(res)
	if err != nil {
		return nil, err
	}

	return bz, nil
}

func (r *queryType[Request, Reply]) encodeEVMRequest(req any) ([]byte, error) {
	if r.requestABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "failed to ABI encode request")
	}

	args := ethereumAbi.Arguments{{Type: *r.requestABI}}
	bz, err := args.Pack(req)
	if err != nil {
		return nil, eris.Wrap(err, "failed to ABI encode request")
	}

	return bz, nil
}

func (r *queryType[Request, Reply]) decodeEVMRequest(bz []byte) (*Request, error) {
	if r.requestABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "failed to ABI decode request")
	}

	args := ethereumAbi.Arguments{{Type: *r.requestABI}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, eris.Wrap(err, "failed to ABI decode request")
	}

	if len(unpacked) < 1 {
		return nil, eris.New("error decoding EVM bytes: no values could be unpacked")
	}

	request, err := abi.SerdeInto[Request](unpacked[0])
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (r *queryType[Request, Reply]) encodeEVMReply(a any) ([]byte, error) {
	if r.replyABI == nil {
		return nil, eris.Wrap(ErrEVMTypeNotSet, "failed to ABI encode reply")
	}

	args := ethereumAbi.Arguments{{Type: *r.replyABI}}
	bz, err := args.Pack(a)
	if err != nil {
		return nil, eris.Wrap(err, "failed to ABI encode reply")
	}

	return bz, nil
}

func (r *queryType[Request, Reply]) decodeEVMReply(bz []byte) (any, error) {
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

	reply, err := abi.SerdeInto[Reply](unpacked[0])
	if err != nil {
		return nil, err
	}

	return reply, nil
}

// GetRequestFieldInformation returns the field information for the request struct.
func (r *queryType[Request, Reply]) GetRequestFieldInformation() map[string]any {
	return types.GetFieldInformation(reflect.TypeOf(new(Request)).Elem())
}

func validateQuery[Request any, Reply any](
	name string,
	handler func(wCtx WorldContext, req *Request) (*Reply, error),
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
