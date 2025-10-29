package service

import (
	"context"
	"sync"

	"github.com/goccy/go-json"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/structpb"
)

// Query is a struct that represents a query to the world.
// For now it has the same structure as ecs.SearchParam, but we might add more fields in the future
// so it's better to keep these as separate types.
type Query struct {
	// List of component names to search for.
	Find []string `json:"find"`
	// Match type: "exact" or "contains".
	Match ecs.SearchMatch `json:"match"`
	// Optional expr language string to filter the results.
	// See https://expr-lang.org/ for documentation.
	Where string `json:"where,omitempty"`
}

// reset resets the Query object for reuse.
func (q *Query) reset() {
	q.Find = q.Find[:0] // Reuse the underlying array
	q.Match = ""
	q.Where = ""
}

// handleQuery creates a new query handler for the world.
func (s *ShardService) handleQuery(ctx context.Context, req *micro.Request) *micro.Response {
	// Check if world is shutting down.
	select {
	case <-ctx.Done():
		return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), 0)
	default:
		// Continue processing.
	}

	query, err := parseQuery(&s.queryPool, req)
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), 0)
	}
	defer s.queryPool.Put(query)

	results, err := s.world.NewSearch(ecs.SearchParam{
		Find:  query.Find,
		Match: query.Match,
		Where: query.Where,
	})
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to search entities"), 0)
	}

	res, err := serializeQueryResults(results)
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to serialize results"), 0)
	}

	return micro.NewSuccessResponse(req, res)
}

// parseQuery parses the query from the payload.
func parseQuery(pool *sync.Pool, req *micro.Request) (*Query, error) {
	var payload iscv1.Query
	if err := req.Payload.UnmarshalTo(&payload); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal payload into query")
	}
	if err := protovalidate.Validate(&payload); err != nil {
		return nil, eris.Wrap(err, "failed to validate query")
	}

	query := pool.Get().(*Query) //nolint:errcheck // we know the type
	query.reset()

	// Set query fields from the iscv1 query.
	query.Find = payload.GetFind()
	query.Match = ecs.SearchMatch(iscv1MatchToString(payload.GetMatch()))
	query.Where = payload.GetWhere()

	return query, nil
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
