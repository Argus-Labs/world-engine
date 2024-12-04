package world

import (
	"encoding/json"
	"reflect"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/v2/types"
)

var _ Query = &queryType[struct{}, struct{}]{}

var DefaultGroup = "game"

type Query interface {
	// Name returns the name of the query.
	Name() string
	// Group returns the group of the query.
	Group() string
	// GetRequestFieldInformation returns a map of the fields of the query's request type and their types.
	GetRequestFieldInformation() map[string]any

	// handleQuery handles queries with concrete struct types, rather than encoded bytes.
	handleQuery(WorldContextReadOnly, any) (any, error)
	// HandleQueryJSON handles json-encoded query request and return a json-encoded response.
	HandleQueryJSON(WorldContextReadOnly, []byte) ([]byte, error)
}

type QueryOption[Request, Reply any] func(qt *queryType[Request, Reply])

type queryType[Request any, Reply any] struct {
	name    string
	group   string
	handler func(wCtx WorldContextReadOnly, req *Request) (*Reply, error)
}

// WithGroup sets a custom group for the query.
// By default, queries are registered under the "game" group which maps it to the /query/game/:queryType route.
// This option allows you to set a custom group, which allow you to register the query
// under /query/<custom_group>/:queryType.
func WithGroup[Request, Reply any](group string) QueryOption[Request, Reply] {
	return func(qt *queryType[Request, Reply]) {
		qt.group = group
	}
}

func NewQueryType[Request any, Reply any](
	name string,
	handler func(wCtx WorldContextReadOnly, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) (Query, error) {
	err := validateQuery[Request, Reply](name, handler)
	if err != nil {
		return nil, err
	}
	r := &queryType[Request, Reply]{
		name:    name,
		group:   DefaultGroup,
		handler: handler,
	}
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

func (r *queryType[Request, Reply]) Name() string {
	return r.name
}

func (r *queryType[Request, Reply]) Group() string {
	return r.group
}

func (r *queryType[Request, Reply]) handleQuery(wCtx WorldContextReadOnly, a any) (any, error) {
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

func (r *queryType[Request, Reply]) HandleQueryJSON(wCtx WorldContextReadOnly, bz []byte) ([]byte, error) {
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

// GetRequestFieldInformation returns the field information for the request struct.
func (r *queryType[Request, Reply]) GetRequestFieldInformation() map[string]any {
	return types.GetFieldInformation(reflect.TypeOf(new(Request)).Elem())
}

func validateQuery[Request any, Reply any](
	name string,
	handler func(wCtx WorldContextReadOnly, req *Request) (*Reply, error),
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
