package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
)

// -------------------------------------------------------------------------------------------------
// Test types and helpers
// -------------------------------------------------------------------------------------------------
// These types are used exclusively for introspect testing. They register components, commands, and
// events via the system state pattern to populate the world's type registry.
// -------------------------------------------------------------------------------------------------

type IntrospectTestComponent struct {
	Value int `json:"value"`
}

func (IntrospectTestComponent) Name() string { return "IntrospectTestComponent" }

type NestedTestComponent struct {
	Inner struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"inner"`
	Tags []string `json:"tags"`
}

func (NestedTestComponent) Name() string { return "NestedTestComponent" }

type IntrospectTestCommand struct {
	Action string `json:"action"`
	Target int    `json:"target"`
}

func (IntrospectTestCommand) Name() string { return "IntrospectTestCommand" }

type IntrospectTestEvent struct {
	Message string `json:"message"`
}

func (IntrospectTestEvent) Name() string { return "IntrospectTestEvent" }

type IntrospectInitSystemState struct {
	ecs.BaseSystemState
	Query ecs.Contains[struct {
		Comp ecs.Ref[IntrospectTestComponent]
	}]
	Commands ecs.WithCommand[IntrospectTestCommand]
	Events   ecs.WithEvent[IntrospectTestEvent]
}

func introspectInitSystem(_ *IntrospectInitSystemState) error {
	return nil
}

func createIntrospectTestWorld(t *testing.T) *ecs.World {
	t.Helper()
	world := ecs.NewWorld()

	ecs.RegisterSystem(world, introspectInitSystem, ecs.WithHook(ecs.Init))

	world.Init()

	_, err := world.Tick(nil)
	require.NoError(t, err, "world tick failed")

	return world
}

func createIntrospectTestRequest() *micro.Request {
	return &micro.Request{
		ServiceAddress: &microv1.ServiceAddress{},
	}
}

// -------------------------------------------------------------------------------------------------
// handleIntrospect endpoint tests
// -------------------------------------------------------------------------------------------------
// These tests verify the handleIntrospect HTTP handler behavior including success paths, caching
// via sync.Once, and proper error handling when context is cancelled.
// -------------------------------------------------------------------------------------------------

func TestHandleIntrospect_Success(t *testing.T) {
	t.Parallel()
	world := createIntrospectTestWorld(t)
	svc := &ShardService{world: world}

	resp := svc.handleIntrospect(context.Background(), createIntrospectTestRequest())

	require.NotNil(t, resp)
	assert.Equal(t, codes.OK, codes.Code(resp.Status.GetCode()))
	assert.NotNil(t, resp.Payload)
}

func TestHandleIntrospect_CachesResult(t *testing.T) {
	t.Parallel()
	world := createIntrospectTestWorld(t)
	svc := &ShardService{world: world}

	resp1 := svc.handleIntrospect(context.Background(), createIntrospectTestRequest())
	require.Equal(t, codes.OK, codes.Code(resp1.Status.GetCode()))

	cacheAfterFirst := svc.introspect.Cache

	resp2 := svc.handleIntrospect(context.Background(), createIntrospectTestRequest())
	require.Equal(t, codes.OK, codes.Code(resp2.Status.GetCode()))

	assert.Equal(t, cacheAfterFirst, svc.introspect.Cache, "cache should be reused")
	assert.NotNil(t, svc.introspect.Cache)
}

func TestHandleIntrospect_ContextCancelled(t *testing.T) {
	t.Parallel()
	world := createIntrospectTestWorld(t)
	svc := &ShardService{world: world}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp := svc.handleIntrospect(ctx, createIntrospectTestRequest())

	require.NotNil(t, resp)
	assert.NotEqual(t, codes.OK, codes.Code(resp.Status.GetCode()))
}

// -------------------------------------------------------------------------------------------------
// buildIntrospectCache tests
// -------------------------------------------------------------------------------------------------
// These tests verify that the cache builder correctly collects all registered types (commands,
// components, events) from the world and structures them appropriately for the response.
// -------------------------------------------------------------------------------------------------

func TestBuildIntrospectCache_ContainsAllTypes(t *testing.T) {
	t.Parallel()
	world := createIntrospectTestWorld(t)
	svc := &ShardService{world: world}

	cache, err := svc.buildIntrospectCache()
	require.NoError(t, err)

	assert.Contains(t, cache, "commands")
	assert.Contains(t, cache, "components")
	assert.Contains(t, cache, "events")

	commands, ok := cache["commands"].([]any)
	require.True(t, ok)
	assertContainsTypeName(t, commands, "IntrospectTestCommand")

	events, ok := cache["events"].([]any)
	require.True(t, ok)
	assertContainsTypeName(t, events, "IntrospectTestEvent")

	components, ok := cache["components"].([]any)
	require.True(t, ok)
	assertContainsTypeName(t, components, "IntrospectTestComponent")
}

func TestBuildIntrospectCache_EmptyWorld(t *testing.T) {
	t.Parallel()
	world := ecs.NewWorld()
	world.Init()

	svc := &ShardService{world: world}

	cache, err := svc.buildIntrospectCache()
	require.NoError(t, err)

	commands, ok := cache["commands"].([]any)
	require.True(t, ok)
	assert.Empty(t, commands)

	events, ok := cache["events"].([]any)
	require.True(t, ok)
	assert.Empty(t, events)

	components, ok := cache["components"].([]any)
	require.True(t, ok)
	assert.Empty(t, components)
}

// -------------------------------------------------------------------------------------------------
// JSON schema generation tests
// -------------------------------------------------------------------------------------------------
// These tests verify the JSON schema generation helpers produce valid schemas and correctly remove
// redundant fields ($schema, type, additionalProperties) that are always the same for structs.
// -------------------------------------------------------------------------------------------------

func TestGetJSONSchemas_GeneratesValidSchemas(t *testing.T) {
	t.Parallel()
	world := createIntrospectTestWorld(t)

	schemas, err := getJSONSchemas(world.CommandTypes())
	require.NoError(t, err)
	require.NotEmpty(t, schemas)

	for _, item := range schemas {
		schemaMap, ok := item.(map[string]any)
		require.True(t, ok, "schema item should be a map")
		assert.Contains(t, schemaMap, "name")
		assert.Contains(t, schemaMap, "schema")

		schema, ok := schemaMap["schema"].(map[string]any)
		require.True(t, ok, "schema should be a map")
		assert.NotContains(t, schema, "$schema", "redundant $schema should be removed")
		assert.NotContains(t, schema, "type", "redundant type should be removed")
	}
}

func TestSchemaToMap_RemovesRedundantFields(t *testing.T) {
	t.Parallel()
	type SimpleStruct struct {
		Field string `json:"field"`
	}

	schema := reflectSchema(reflect.TypeOf(SimpleStruct{}))
	result, err := schemaToMap(schema)

	require.NoError(t, err)
	assert.NotContains(t, result, "$schema")
	assert.NotContains(t, result, "type")
	assert.NotContains(t, result, "additionalProperties")
}

func assertContainsTypeName(t *testing.T, items []any, typeName string) {
	t.Helper()
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name, valid := m["name"].(string); valid && name == typeName {
			return
		}
	}
	t.Errorf("expected to find type %q in schemas", typeName)
}
