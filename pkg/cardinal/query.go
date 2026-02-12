package cardinal

import (
	"context"
	"sync"

	"buf.build/go/protovalidate"
	"github.com/goccy/go-json"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
)

// query is a struct that represents a query to the world.
// For now it has the same structure as ecs.SearchParam, but we might add more fields in the future
// so it's better to keep these as separate types.
type query struct {
	// List of component names to search for.
	find []string
	// Match type: "exact" or "contains".
	match ecs.SearchMatch
	// Optional expr language string to filter the results.
	// See https://expr-lang.org/ for documentation.
	where string
}

// reset resets the query object for reuse.
func (q *query) reset() {
	q.find = q.find[:0] // Reuse the underlying array
	q.match = ""
	q.where = ""
}

// handleQuery creates a new query handler for the world.
func (s *service) handleQuery(ctx context.Context, req *micro.Request) *micro.Response {
	// Check if world is shutting down.
	select {
	case <-ctx.Done():
		return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
	default:
		// Continue processing.
	}

	q, err := parseQuery(&s.queryPool, req)
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.Internal)
	}
	defer s.queryPool.Put(q)

	results, err := s.world.world.NewSearch(ecs.SearchParam{
		Find:  q.find,
		Match: q.match,
		Where: q.where,
	})
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to search entities"), codes.Internal)
	}

	res, err := serializeQueryResults(results)
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to serialize results"), codes.Internal)
	}

	return micro.NewSuccessResponse(req, res)
}

// parseQuery parses the query from the payload.
func parseQuery(pool *sync.Pool, req *micro.Request) (*query, error) {
	var payload iscv1.Query
	if err := req.Payload.UnmarshalTo(&payload); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal payload into query")
	}
	if err := protovalidate.Validate(&payload); err != nil {
		return nil, eris.Wrap(err, "failed to validate query")
	}

	q := pool.Get().(*query) //nolint:errcheck // we know the type
	q.reset()

	// Set query fields from the iscv1 query.
	q.find = payload.GetFind()
	q.match = ecs.SearchMatch(iscv1MatchToString(payload.GetMatch()))
	q.where = payload.GetWhere()

	return q, nil
}

// serializeQueryResults serializes the results into a protobuf message.
func serializeQueryResults(results []map[string]any) (*iscv1.QueryResult, error) {
	var entities []*structpb.Struct

	for _, result := range results {
		jsonBytes, err := json.Marshal(result)
		if err != nil {
			return nil, eris.Wrap(err, "failed to marshal results to JSON")
		}

		var protoCompatible map[string]any
		if err := json.Unmarshal(jsonBytes, &protoCompatible); err != nil {
			return nil, eris.Wrap(err, "failed to unmarshal results to protobuf-compatible format")
		}

		structValue, err := structpb.NewStruct(protoCompatible)
		if err != nil {
			return nil, eris.Wrap(err, "failed to create struct value")
		}

		entities = append(entities, structValue)
	}

	return &iscv1.QueryResult{
		Entities: entities,
	}, nil
}

func iscv1MatchToString(m iscv1.Query_Match) string {
	switch m {
	case iscv1.Query_MATCH_EXACT:
		return "exact"
	case iscv1.Query_MATCH_CONTAINS:
		return "contains"
	case iscv1.Query_MATCH_UNSPECIFIED:
		fallthrough
	default:
		return "" // This will be validated again in ecs.NewSearch
	}
}
