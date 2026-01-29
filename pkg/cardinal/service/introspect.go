package service

import (
	"context"
	"reflect"
	"sync"

	"github.com/argus-labs/world-engine/pkg/micro"

	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
)

type Introspect struct {
	Cache      map[string]any // lazily built on first request, immutable after init
	Once       sync.Once
	BuildError error // BuildError persists the first cache-build failure (sync.Once won’t retry).
}

// handleIntrospect returns metadata about the registered types in the world.
// The result is cached on first call since type registrations are immutable after init.
// This endpoint is intended for dev tooling (e.g., AI agents, debugging tools).
func (s *ShardService) handleIntrospect(ctx context.Context, req *micro.Request) *micro.Response {
	// Check if world is shutting down.
	select {
	case <-ctx.Done():
		return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
	default:
		// Continue processing.
	}

	// Build cache on first call (thread-safe via sync.Once)
	s.introspect.Once.Do(func() {
		s.introspect.Cache, s.introspect.BuildError = s.buildIntrospectCache()
	})
	if s.introspect.BuildError != nil {
		return micro.NewErrorResponse(
			req, eris.Wrap(s.introspect.BuildError, "failed to build introspect cache"), codes.Internal)
	}

	result, err := structpb.NewStruct(s.introspect.Cache)
	if err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to build introspect result"), codes.Internal)
	}

	return micro.NewSuccessResponse(req, result)
}

// buildIntrospectCache builds the introspection metadata for commands, components, and events.
// This is called once and cached for subsequent requests.
func (s *ShardService) buildIntrospectCache() (map[string]any, error) {
	return nil, nil
	// componentsSchemas, err := getJSONSchemas(s.world.ComponentTypes())
	// if err != nil {
	// 	return nil, eris.Wrap(err, "failed to get components JSON schemas")
	// }
	//
	// commandsSchemas, err := getJSONSchemas(s.world.CommandTypes())
	// if err != nil {
	// 	return nil, eris.Wrap(err, "failed to get commands JSON schemas")
	// }
	//
	// eventsSchemas, err := getJSONSchemas(s.world.EventTypes())
	// if err != nil {
	// 	return nil, eris.Wrap(err, "failed to get events JSON schemas")
	// }
	//
	// return map[string]any{
	// 	"commands":   commandsSchemas,
	// 	"components": componentsSchemas,
	// 	"events":     eventsSchemas,
	// }, nil
}

// getJSONSchemas is a generic helper that converts a map of type names to reflect.Type
// into a list of JSON schema objects.
func getJSONSchemas(types map[string]reflect.Type) ([]any, error) {
	result := make([]any, 0, len(types))
	for name, typ := range types {
		schema := reflectSchema(typ)
		schemaMap, err := schemaToMap(schema)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to convert schema")
		}
		result = append(result, map[string]any{
			"name":   name,
			"schema": schemaMap,
		})
	}
	return result, nil
}

// reflectSchema generates a JSON Schema from a reflect.Type using invopop/jsonschema.
func reflectSchema(t reflect.Type) *jsonschema.Schema {
	r := &jsonschema.Reflector{
		Anonymous:      true, // Don't add $id based on package path
		ExpandedStruct: true, // Inline the struct fields directly
	}
	return r.ReflectFromType(t)
}

// schemaToMap converts a jsonschema.Schema to a map[string]any.
func schemaToMap(schema any) (map[string]any, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal schema")
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal schema")
	}
	// Remove redundant fields that are always the same for structs
	delete(result, "$schema")
	delete(result, "type")                 // Always "object" for structs
	delete(result, "additionalProperties") // Always false for structs
	return result, nil
}
