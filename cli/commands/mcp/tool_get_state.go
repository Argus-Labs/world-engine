package mcp

import (
	"context"
	"maps"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rotisserie/eris"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// defaultMaxStateEntities caps how many entities get_state returns by default.
// GetState takes no arguments — the shard always hands back its whole world — so
// find/match/where are applied here and this caps whatever survives them.
const defaultMaxStateEntities = 100

// Match modes for the find filter, mirroring the engine's SearchMatch.
const (
	matchContains = "contains"
	matchExact    = "exact"
)

// GetStateInput is the structured input for the get_state tool.
type GetStateInput struct {
	ShardID      string   `json:"shard_id"                jsonschema_description:"ID of the shard to read state from"`
	InstanceName string   `json:"instance_name,omitempty" jsonschema_description:"Specific pool instance; accepts variants like 'game-5', 'game 5', 'game5', or '5'. Defaults to the shard's first instance."`
	Find         []string `json:"find,omitempty"          jsonschema_description:"Component names to filter on (e.g. ['Health','Position']). Names are the component's registered Name(), not the Go type. Omit to return every entity."`
	Match        string   `json:"match,omitempty"         jsonschema_description:"How find is applied: 'contains' (entity has at least these components — the default) or 'exact' (entity has exactly these and nothing else)."`
	Where        string   `json:"where,omitempty"         jsonschema_description:"expr-lang boolean expression filtering entities, e.g. 'Health.HP > 50'. Component names are the variables and '_id' is the entity ID. Fields are the component's protobuf field names, which match its Go struct field names; every field is readable, unset ones included. An entity missing a component the expression reads fields from doesn't match; a bare comparison like 'Gravestone == nil' still evaluates for every entity."`
	MaxEntities  int      `json:"max_entities,omitempty"  jsonschema_description:"Maximum entities to return (default 100). Use a negative value to return every match."`
	Organization string   `json:"organization,omitempty"  jsonschema_description:"Organization (auto-derived from the cluster's ShardPool for this shard if omitted)"`
	Project      string   `json:"project,omitempty"       jsonschema_description:"Project (auto-derived from the cluster's ShardPool for this shard if omitted)"`
	ShardURL     string   `json:"shard_url,omitempty"     jsonschema_description:"Cardinal shard API URL; auto-resolved per instance from the cluster when omitted. When set, pair it with instance_name; pass organization/project too to skip the cluster lookup entirely."`
	OperatorURL  string   `json:"operator_url,omitempty"  jsonschema_description:"cardinal-operator URL used to resolve instance_name (defaults to http://localhost:8090 for local dev)"`
}

// GetStateOutput is the structured output for the get_state tool.
type GetStateOutput struct {
	ShardID      string        `json:"shard_id"            jsonschema_description:"Shard the snapshot came from"`
	InstanceName string        `json:"instance_name"       jsonschema_description:"Pool instance the snapshot was actually read from (resolved, even when instance_name was omitted)"`
	IsPaused     bool          `json:"is_paused"           jsonschema_description:"Whether tick execution is currently paused"`
	TickHeight   uint64        `json:"tick_height"         jsonschema_description:"Tick the snapshot was taken at"`
	Timestamp    string        `json:"timestamp,omitempty" jsonschema_description:"When the shard captured the snapshot (RFC 3339)"`
	Version      uint32        `json:"version,omitempty"   jsonschema_description:"Snapshot format version"`
	EntityCount  int           `json:"entity_count"        jsonschema_description:"Total entities in the world, before filtering"`
	Matched      int           `json:"matched"             jsonschema_description:"Entities that passed the filter, before truncation"`
	Truncated    bool          `json:"truncated"           jsonschema_description:"True when entities was cut short by max_entities"`
	Entities     []EntityState `json:"entities"            jsonschema_description:"Matching entities with their component data"`
}

// EntityState is one entity from the snapshot with its decoded components.
type EntityState struct {
	ID         uint32         `json:"id"         jsonschema_description:"Entity ID"`
	Components map[string]any `json:"components" jsonschema_description:"Component data keyed by component name"`
}

// registerGetStateTool registers the get_state tool. It reads a shard's current
// world snapshot from its DebugService, using the same addressing as introspect.
func registerGetStateTool(srv *server.MCPServer) {
	getStateTool := mcp.NewTool(
		"get_state",
		mcp.WithDescription(
			"Read and query a Cardinal shard's world state: tick height, whether the world is paused, and "+
				"the entities with their component data. Filter with find/match (by component set) and "+
				"where (an expr-lang predicate such as 'Health.HP > 50'). The shard hands over a whole "+
				"snapshot and filtering happens here, so narrowing a query costs the shard nothing. "+
				"State is the last snapshot the tick loop published. Requires a running cluster.",
		),
		mcp.WithInputSchema[GetStateInput](),
		mcp.WithOutputSchema[GetStateOutput](),
	)
	srv.AddTool(getStateTool, strictToolHandler(getStateHandler))
}

// getStateHandler fetches a shard's world snapshot, filters it, and flattens the
// survivors per entity.
func getStateHandler(
	ctx context.Context,
	_ mcp.CallToolRequest,
	args GetStateInput,
) (GetStateOutput, error) {
	filter, err := compileStateFilter(args)
	if err != nil {
		return GetStateOutput{}, err
	}

	ctx, cancel := ensureDeadline(ctx, defaultCommandTimeout)
	defer cancel()

	client, target, err := dialShardDebugService(ctx, debugTargetArgs{
		shardID:      args.ShardID,
		instanceName: args.InstanceName,
		organization: args.Organization,
		project:      args.Project,
		shardURL:     args.ShardURL,
		operatorURL:  args.OperatorURL,
	})
	if err != nil {
		return GetStateOutput{}, err
	}

	// Component columns are protobuf wire bytes, so the shard's descriptors are needed to read them.
	descriptors, err := fetchComponentDescriptors(ctx, client)
	if err != nil {
		return GetStateOutput{}, err
	}

	res, err := client.GetState(ctx, connect.NewRequest(&cardinalv1.GetStateRequest{}))
	if err != nil {
		return GetStateOutput{}, eris.Wrapf(err, "failed to read state from shard %q", args.ShardID)
	}

	limit := args.MaxEntities
	if limit == 0 {
		limit = defaultMaxStateEntities
	}

	snapshot := res.Msg.GetSnapshot()
	result, err := flattenWorldState(snapshot.GetWorldState(), limit, filter, descriptors)
	if err != nil {
		return GetStateOutput{}, err
	}

	out := GetStateOutput{
		ShardID:      args.ShardID,
		InstanceName: target.instanceName,
		IsPaused:     res.Msg.GetIsPaused(),
		TickHeight:   snapshot.GetTickHeight(),
		Version:      snapshot.GetVersion(),
		EntityCount:  result.total,
		Matched:      result.matched,
		Truncated:    len(result.entities) < result.matched,
		Entities:     result.entities,
	}
	if ts := snapshot.GetTimestamp(); ts != nil {
		out.Timestamp = ts.AsTime().Format(time.RFC3339)
	}
	return out, nil
}

// stateFilter is the compiled form of the find/match/where inputs.
type stateFilter struct {
	find  map[string]struct{} // requested component names; empty matches every entity
	match string
	where *vm.Program // nil when no where expression was given
	// Components the where expression reads fields from (e.g. "Health" in 'Health.HP > 50'). An
	// entity missing one can't satisfy the expression, so it's rejected instead of evaluated —
	// evaluating would fail on the nil lookup. Bare references ('Health == nil') aren't collected,
	// so absence checks still evaluate.
	whereNeeds []string
}

// compileStateFilter validates the filter inputs and compiles the where clause
// once, before the snapshot is walked.
func compileStateFilter(args GetStateInput) (stateFilter, error) {
	f := stateFilter{match: args.Match}

	if len(args.Find) > 0 {
		f.find = make(map[string]struct{}, len(args.Find))
		for _, name := range args.Find {
			f.find[name] = struct{}{}
		}
	}

	if f.match == "" {
		f.match = matchContains
	}
	if f.match != matchContains && f.match != matchExact {
		return stateFilter{}, eris.Errorf("match must be %q or %q, got %q", matchContains, matchExact, f.match)
	}

	if args.Where != "" {
		// Compiled as a bool expression, same as the engine's search filter. The
		// component types aren't known until an entity is in hand, so a non-bool
		// result can still slip through to evaluation time.
		program, err := expr.Compile(args.Where, expr.AsBool())
		if err != nil {
			return stateFilter{}, eris.Wrap(err, "failed to parse where clause")
		}
		f.where = program
		f.whereNeeds = whereFieldReads(args.Where)
	}
	return f, nil
}

// whereFieldReads returns the component names a where expression reads fields from — the roots of
// member accesses like 'Health.HP' or 'Health["HP"]'. Parse errors return nothing; expr.Compile has
// already accepted the expression, so a failure here just skips the presence pre-check.
func whereFieldReads(where string) []string {
	tree, err := parser.Parse(where)
	if err != nil {
		return nil
	}
	v := &fieldReadVisitor{names: map[string]struct{}{}}
	ast.Walk(&tree.Node, v)

	delete(v.names, "_id") // the entity ID is always present
	names := slices.Collect(maps.Keys(v.names))
	slices.Sort(names) // deterministic evaluation order
	return names
}

// fieldReadVisitor collects identifiers that member accesses read from.
type fieldReadVisitor struct {
	names map[string]struct{}
}

func (v *fieldReadVisitor) Visit(node *ast.Node) {
	member, ok := (*node).(*ast.MemberNode)
	if !ok {
		return
	}
	if ident, ok := member.Node.(*ast.IdentifierNode); ok {
		v.names[ident.Value] = struct{}{}
	}
}

// matchesComponents reports whether an entity's component set satisfies find/match.
func (f stateFilter) matchesComponents(names []string) bool {
	if len(f.find) == 0 {
		return true
	}
	found := 0
	for _, name := range names {
		if _, ok := f.find[name]; ok {
			found++
		}
	}
	if found != len(f.find) {
		return false // entity is missing at least one requested component
	}
	// "contains" is now satisfied; exact additionally forbids extra components.
	return f.match != matchExact || len(names) == len(f.find)
}

// matchesWhere evaluates the where expression against one entity. The
// environment mirrors the engine's: component names as variables plus "_id".
func (f stateFilter) matchesWhere(id uint32, components map[string]any) (bool, error) {
	if f.where == nil {
		return true, nil
	}
	for _, name := range f.whereNeeds {
		if _, ok := components[name]; !ok {
			return false, nil // can't satisfy a field read on a component it doesn't have
		}
	}
	env := make(map[string]any, len(components)+1)
	maps.Copy(env, components)
	env["_id"] = id

	output, err := expr.Run(f.where, env)
	if err != nil {
		return false, eris.Wrap(err, "failed to run where clause")
	}
	matched, ok := output.(bool)
	if !ok {
		return false, eris.New("where clause must evaluate to a boolean")
	}
	return matched, nil
}

// flattenResult is what one pass over the snapshot produced.
type flattenResult struct {
	entities []EntityState
	matched  int // passed the filter, before truncation
	total    int // entities in the world, before filtering
}

// flattenWorldState turns the snapshot into a per-entity view, keeping only
// entities the filter accepts and at most limit of them. Each entity lists its
// components as indices into the snapshot's name table, with one payload per
// component in the same order. A negative limit keeps every match.
func flattenWorldState(
	ws *cardinalv1.WorldState,
	limit int,
	filter stateFilter,
	descriptors componentDescriptors,
) (flattenResult, error) {
	result := flattenResult{entities: make([]EntityState, 0)}
	table := ws.GetComponents()
	entities := ws.GetEntities()
	result.total = len(entities)

	for _, entity := range entities {
		names, err := componentNames(table, entity.GetComponents())
		if err != nil {
			return flattenResult{}, err
		}
		if !filter.matchesComponents(names) {
			continue
		}

		atLimit := limit >= 0 && len(result.entities) >= limit

		// Past the limit with no where clause every remaining entity that passed
		// find/match matches, so it can only move the count — decoding its
		// components would be thrown away. On a large world this is the difference
		// between decoding the whole snapshot and decoding one page of it.
		if atLimit && filter.where == nil {
			result.matched++
			continue
		}

		components := entityComponents(names, entity.GetPayloads(), descriptors)

		ok, err := filter.matchesWhere(entity.GetId(), components)
		if err != nil {
			return flattenResult{}, err
		}
		if !ok {
			continue
		}

		result.matched++
		if atLimit {
			continue // keep counting so the caller learns the real match count
		}
		result.entities = append(result.entities, EntityState{ID: entity.GetId(), Components: components})
	}
	return result, nil
}

// componentNames resolves an entity's component indices through the snapshot's name table.
func componentNames(table []string, indices []uint32) ([]string, error) {
	names := make([]string, len(indices))
	for i, idx := range indices {
		if int(idx) >= len(table) {
			return nil, eris.Errorf("snapshot entity references component %d, but the name table has %d entries",
				idx, len(table))
		}
		names[i] = table[idx]
	}
	return names, nil
}

// entityComponents pairs an entity's component names with its payloads. Payloads are protobuf wire
// bytes, decoded against the component's registered descriptor. A component with no descriptor, or a
// payload that won't decode, is passed through as raw bytes rather than failing the whole read.
func entityComponents(names []string, payloads [][]byte, descriptors componentDescriptors) map[string]any {
	components := make(map[string]any, len(names))
	for i, name := range names {
		if i >= len(payloads) {
			break // fewer payloads than components; nothing stored for the rest
		}
		var value any = payloads[i]
		if md, ok := descriptors[name]; ok {
			if decoded, err := decodeMessage(md, payloads[i]); err == nil {
				value = decoded
			}
		}
		components[name] = value
	}
	return components
}
