package cardinal

import (
	"context"
	"sync"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/shamaton/msgpack/v3"
	"google.golang.org/grpc/codes"
)

// query is a struct that represents a query to the world.
// For now it has the same structure as ecs.SearchParam, but we might add more fields in the future
// so it's better to keep these as separate types.
type Query struct {
	// List of component names to search for. Must be empty when Match is MatchAll.
	Find []string `json:"find"`
	// Match type: "exact", "contains", or "all".
	Match ecs.SearchMatch `json:"match"`
	// Optional expr language string to filter the results.
	// See https://expr-lang.org/ for documentation.
	Where string `json:"where,omitempty"`
	// Maximum number of results to return (default: unlimited, 0 = unlimited).
	Limit uint32 `json:"limit,omitempty"`
	// Number of results to skip before returning (default: 0).
	Offset uint32 `json:"offset,omitempty"`
}

// reset resets the Query object for reuse.
func (q *Query) reset() {
	q.Find = q.Find[:0] // Reuse the underlying array
	q.Match = ""
	q.Where = ""
	q.Limit = 0
	q.Offset = 0
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

	results, err := s.world.NewSearch(ecs.SearchParam{
		Find:   query.Find,
		Match:  query.Match,
		Where:  query.Where,
		Limit:  query.Limit,
		Offset: query.Offset,
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

	// Parse Limit and Offset
	// In proto3, unset uint32 fields return 0, which means unlimited for limit
	query.Limit = payload.GetLimit()
	query.Offset = payload.GetOffset()

	return query, nil
}

// serializeQueryResults serializes the results into a protobuf message.
// Each entity is serialized as MessagePack bytes to preserve uint64 precision.
func serializeQueryResults(results []map[string]any) (*iscv1.QueryResult, error) {
	entities := make([][]byte, 0, len(results))

	for _, result := range results {
		data, err := msgpack.Marshal(result)
		if err != nil {
			return nil, eris.Wrap(err, "failed to marshal entity to msgpack")
		}
		entities = append(entities, data)
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
	case iscv1.Query_MATCH_ALL:
		return "all"
	case iscv1.Query_MATCH_UNSPECIFIED:
		fallthrough
	default:
		return "" // This will be validated again in ecs.NewSearch
	}
}
