package cardinal

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/goccy/go-json"
	"github.com/invopop/jsonschema"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
)

// debugModule provides introspection and debugging capabilities for a World instance.
type debugModule struct {
	world      *World
	server     *http.Server
	control    *tickControl
	reflector  *jsonschema.Reflector
	commands   map[string]*structpb.Struct
	events     map[string]*structpb.Struct
	components map[string]*structpb.Struct
}

var _ cardinalv1connect.DebugServiceHandler = (*debugModule)(nil)

// newDebugModule creates a new debugModule bound to the given World.
func newDebugModule(world *World) debugModule {
	return debugModule{
		world:      world,
		control:    newTickControl(),
		commands:   make(map[string]*structpb.Struct),
		events:     make(map[string]*structpb.Struct),
		components: make(map[string]*structpb.Struct),
		reflector: &jsonschema.Reflector{
			Anonymous:      true, // Don't add $id based on package path
			ExpandedStruct: true, // Inline the struct fields directly
		},
	}
}

// Init initializes and starts the connect server for the debug service.
func (d *debugModule) Init(addr string) {
	if d == nil {
		return
	}

	logger := d.world.tel.GetLogger("debug")

	mux := http.NewServeMux()
	mux.Handle(cardinalv1connect.NewDebugServiceHandler(d, connect.WithInterceptors(validate.NewInterceptor())))

	d.server = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info().Str("addr", addr).Msg("Debug service initialized")

	go func() {
		_ = d.server.ListenAndServe()
	}()
}

// Shutdown gracefully shuts down the debug server.
func (d *debugModule) Shutdown(ctx context.Context) error {
	if d == nil || d.server == nil {
		return nil
	}
	return d.server.Shutdown(ctx)
}

// -------------------------------------------------------------------------------------------------
// Introspect
// -------------------------------------------------------------------------------------------------

// register records the JSON schema of a command, event, or component type for introspection.
func (d *debugModule) register(kind string, value schema.Serializable) error {
	if d == nil {
		return nil
	}

	var catalog map[string]*structpb.Struct
	switch kind {
	case "command":
		catalog = d.commands
	case "event":
		catalog = d.events
	case "component":
		catalog = d.components
	default:
		panic("this is an internal function, this should never panic")
	}

	name := value.Name()
	if _, exists := catalog[name]; exists {
		return nil
	}

	jsonSchema := d.reflector.Reflect(value)
	data, err := json.Marshal(jsonSchema)
	if err != nil {
		return eris.Wrap(err, "failed to marshal json schema")
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		return eris.Wrap(err, "failed to unmarshal json schema")
	}

	// Remove redundant fields.
	delete(schemaMap, "$schema")
	delete(schemaMap, "type")
	delete(schemaMap, "additionalProperties")

	schemaStruct, err := structpb.NewStruct(schemaMap)
	if err != nil {
		return eris.Wrap(err, "failed to create struct from schema")
	}

	catalog[name] = schemaStruct
	return nil
}

// Introspect returns metadata about the registered types in the world.
func (d *debugModule) Introspect(
	_ context.Context,
	_ *connect.Request[cardinalv1.IntrospectRequest],
) (*connect.Response[cardinalv1.IntrospectResponse], error) {
	return connect.NewResponse(&cardinalv1.IntrospectResponse{
		Commands:   d.buildTypeSchemas(d.commands),
		Components: d.buildTypeSchemas(d.components),
		Events:     d.buildTypeSchemas(d.events),
	}), nil
}

// buildTypeSchemas converts the internal schema cache to proto TypeSchema messages.
func (d *debugModule) buildTypeSchemas(cache map[string]*structpb.Struct) []*cardinalv1.TypeSchema {
	schemas := make([]*cardinalv1.TypeSchema, 0, len(cache))
	for name, schemaStruct := range cache {
		schemas = append(schemas, &cardinalv1.TypeSchema{
			Name:   name,
			Schema: schemaStruct,
		})
	}
	return schemas
}

// -------------------------------------------------------------------------------------------------
// Debugger
// -------------------------------------------------------------------------------------------------

// tickControl coordinates pause, resume, step, and reset signaling for the tick loop.
type tickControl struct {
	pauseCh   chan chan uint64   // Request pause, receives tick height when paused
	resumeCh  chan struct{}      // Signal to resume
	stepCh    chan chan uint64   // Request step, receives tick height after step
	resetCh   chan chan struct{} // Request reset
	isPaused  atomic.Bool        // Current pause state
	stepReady chan struct{}      // Signals that step result is ready to be read
}

// newTickControl creates a tickControl with initialized channels.
func newTickControl() *tickControl {
	return &tickControl{
		pauseCh:   make(chan chan uint64),
		resumeCh:  make(chan struct{}),
		stepCh:    make(chan chan uint64),
		resetCh:   make(chan chan struct{}),
		stepReady: make(chan struct{}),
	}
}

// Pause stops tick execution and returns the current tick height.
func (d *debugModule) Pause(
	_ context.Context,
	_ *connect.Request[cardinalv1.PauseRequest],
) (*connect.Response[cardinalv1.PauseResponse], error) {
	if d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world is already paused"))
	}

	replyCh := make(chan uint64, 1)
	d.control.pauseCh <- replyCh
	tickHeight := <-replyCh

	return connect.NewResponse(&cardinalv1.PauseResponse{
		TickHeight: tickHeight,
	}), nil
}

// Resume continues tick execution after a pause.
func (d *debugModule) Resume(
	_ context.Context,
	_ *connect.Request[cardinalv1.ResumeRequest],
) (*connect.Response[cardinalv1.ResumeResponse], error) {
	if !d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world is not paused"))
	}

	d.control.resumeCh <- struct{}{}

	return connect.NewResponse(&cardinalv1.ResumeResponse{}), nil
}

// Step executes a single tick. Only works when paused.
func (d *debugModule) Step(
	_ context.Context,
	_ *connect.Request[cardinalv1.StepRequest],
) (*connect.Response[cardinalv1.StepResponse], error) {
	if !d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world must be paused to step"))
	}

	replyCh := make(chan uint64, 1)
	d.control.stepCh <- replyCh
	tickHeight := <-replyCh

	return connect.NewResponse(&cardinalv1.StepResponse{
		TickHeight: tickHeight,
	}), nil
}

// Reset restores the world to its initial state (before tick 0).
func (d *debugModule) Reset(
	_ context.Context,
	_ *connect.Request[cardinalv1.ResetRequest],
) (*connect.Response[cardinalv1.ResetResponse], error) {
	if !d.control.isPaused.Load() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, eris.New("world must be paused to reset"))
	}

	replyCh := make(chan struct{}, 1)
	d.control.resetCh <- replyCh
	<-replyCh

	return connect.NewResponse(&cardinalv1.ResetResponse{}), nil
}

// GetState returns the current world state snapshot.
func (d *debugModule) GetState(
	_ context.Context,
	_ *connect.Request[cardinalv1.GetStateRequest],
) (*connect.Response[cardinalv1.GetStateResponse], error) {
	worldState, err := d.world.world.ToProto()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, eris.Wrap(err, "failed to serialize world state"))
	}

	return connect.NewResponse(&cardinalv1.GetStateResponse{
		IsPaused: d.control.isPaused.Load(),
		Snapshot: &cardinalv1.Snapshot{
			TickHeight: d.world.currentTick.height,
			Timestamp:  timestamppb.New(d.world.currentTick.timestamp),
			WorldState: worldState,
		},
	}), nil
}
