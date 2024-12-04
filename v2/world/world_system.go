package world

import (
	"context"
	"reflect"
	"slices"

	"github.com/rotisserie/eris"
	"go.opentelemetry.io/otel/codes"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/v2/tick"
)

// System is a user-defined function that is executed at every tick.
type System func(ctx WorldContext) error

// systemType is an internal entry used to track registered systems.
type systemType struct {
	Name string
	Fn   System
}

func (w *World) GetRegisteredSystems() []string {
	sys := slices.Concat(w.registeredInitSystems, w.registeredSystems)
	sysNames := make([]string, len(sys))
	for i, sys := range sys {
		sysNames[i] = sys.Name
	}
	return sysNames
}

// RegisterSystems registers multiple systems with the system manager.
// If isInit is true, the system will only be executed once at tick 0.
func (w *World) RegisterSystems(isInit bool, systemFuncs ...System) error {
	for _, systemFunc := range systemFuncs {
		if err := w.registerSystem(isInit, systemFunc); err != nil {
			return eris.Wrap(err, "failed to register system")
		}
	}
	return nil
}

// registerSystem is an internal function that allows us to register a system with a custom system name.
func (w *World) registerSystem(isInit bool, systemFunc System) error {
	sysName := reflect.TypeOf(systemFunc).Name()
	sys := systemType{Name: sysName, Fn: systemFunc}
	if isInit {
		w.registeredInitSystems = append(w.registeredInitSystems, sys)
	} else {
		w.registeredSystems = append(w.registeredSystems, sys)
	}
	return nil
}

// RunSystems runs all the registered system in the order that they were registered.
func (w *World) runSystems(ctx context.Context, proposal *tick.Proposal) (*tick.Tick, error) {
	ctx, span := w.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "system.run")
	defer span.End()

	t, err := tick.New(proposal)
	if err != nil {
		return nil, eris.Wrap(err, "failed to initialize tick")
	}

	wCtx := NewWorldContext(w.state, w.pm, t)

	var systemsToRun []systemType
	if t.ID == 0 {
		systemsToRun = slices.Concat(w.registeredInitSystems, w.registeredSystems)
	} else {
		systemsToRun = w.registeredSystems
	}

	for _, sys := range systemsToRun {
		wCtx.setSystemName(sys.Name)

		// Executes the system function that the user registered
		_, systemFnSpan := w.tracer.Start(ddotel.ContextWithStartOptions(ctx,
			ddtracer.Measured()),
			"system.run."+sys.Name)
		if err := sys.Fn(wCtx); err != nil {
			span.SetStatus(codes.Error, eris.ToString(err, true))
			span.RecordError(err)
			systemFnSpan.SetStatus(codes.Error, eris.ToString(err, true))
			systemFnSpan.RecordError(err)
			return nil, eris.Wrapf(err, "System %s generated an error", sys.Name)
		}
		systemFnSpan.End()
	}

	return t, nil
}
