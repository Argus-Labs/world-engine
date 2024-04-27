package cardinal

import (
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"time"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

const (
	noActiveSystemName = ""
)

type SystemFunc func(ctx engine.Context) error

type System struct {
	Name string
	Fn   SystemFunc
}

type SystemManager interface {
	// GetRegisteredSystems returns a slice of all registered systems' name.
	GetRegisteredSystems() []string

	// GetCurrentSystem returns the name of the currently running system.
	// If no system is currently running, it returns an empty string.
	GetCurrentSystem() string

	// These methods are intentionally made private to avoid other
	// packages from trying to modify the System manager in the middle of a tick.
	registerSystems(isInit bool, systems ...SystemFunc) error
	runSystems(wCtx engine.Context) error
}

type systemManager struct {
	// Registered systems in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredSystems     []System
	registeredInitSystems []System

	// currentSystem is the name of the System that is currently running.
	currentSystem string
}

var _ SystemManager = &systemManager{}

func newSystemManager() SystemManager {
	var sm SystemManager = &systemManager{
		registeredSystems:     make([]System, 0),
		registeredInitSystems: make([]System, 0),
		currentSystem:         noActiveSystemName,
	}
	return sm
}

// RegisterSystems registers multiple systems with the System manager.
// There can only be one System with a given name, which is derived from the function name.
// If isInit is true, the System will only be executed once at tick 0.
// If there is a duplicate System name, an error will be returned and none of the systems will be registered.
func (m *systemManager) registerSystems(isInit bool, systemFuncs ...SystemFunc) error {
	// We create a list of System structs to register, and then register them in one go to ensure all or nothing.
	systemToRegister := make([]System, 0, len(systemFuncs))

	// Iterate throughs systemFuncs,
	// 1) Ensure that there is no duplicate System
	// 2) Create a System struct for each one.
	for _, systemFunc := range systemFuncs {
		// Obtain the name of the System function using reflection.
		systemName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(systemFunc).Pointer()).Name())

		// Check for duplicate System names within the list of systems to be registered
		if slices.ContainsFunc(
			systemToRegister,
			func(s System) bool { return s.Name == systemName },
		) {
			return eris.Errorf("duplicate System %q in slice", systemName)
		}

		// Checks if the System is already previously registered.
		// This will terminate the registration of all systems if any of them are already registered.
		if slices.ContainsFunc(
			slices.Concat(m.registeredSystems, m.registeredInitSystems),
			func(s System) bool { return s.Name == systemName },
		) {
			return eris.Errorf("System %q is already registered", systemName)
		}

		systemToRegister = append(systemToRegister, System{Name: systemName, Fn: systemFunc})
	}

	if isInit {
		m.registeredInitSystems = append(m.registeredInitSystems, systemToRegister...)
	} else {
		m.registeredSystems = append(m.registeredSystems, systemToRegister...)
	}

	return nil
}

// RunSystems runs all the registered System in the order that they were registered.
func (m *systemManager) runSystems(wCtx engine.Context) error {
	var systemsToRun []System
	if wCtx.CurrentTick() == 0 {
		systemsToRun = append(m.registeredInitSystems, m.registeredSystems...)
	} else {
		systemsToRun = m.registeredSystems
	}

	allSystemStartTime := time.Now()
	for _, sys := range systemsToRun {
		// Explicit memory aliasing
		m.currentSystem = sys.Name

		// Inject the System name into the logger
		wCtx.SetLogger(wCtx.Logger().With().Str("system", sys.Name).Logger())

		// Executes the System function that the user registered
		systemStartTime := time.Now()
		err := sys.Fn(wCtx)
		if err != nil {
			m.currentSystem = ""
			return eris.Wrapf(err, "System %s generated an error", sys.Name)
		}

		// Emit the total time it took to run `systemName`
		statsd.EmitTickStat(systemStartTime, sys.Name)
	}

	// Indicate that no System is currently running
	m.currentSystem = noActiveSystemName

	// Emit the total time it took to run all systems
	statsd.EmitTickStat(allSystemStartTime, "all_systems")

	return nil
}

func (m *systemManager) GetRegisteredSystems() []string {
	sys := append(m.registeredInitSystems, m.registeredSystems...)
	sysNames := make([]string, len(sys))
	for i, sys := range append(m.registeredInitSystems, m.registeredSystems...) {
		sysNames[i] = sys.Name
	}
	return sysNames
}

func (m *systemManager) GetCurrentSystem() string {
	return m.currentSystem
}
