package system

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"time"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type Manager struct {
	// registeredSystems is a list of all the registered system names in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredSystems []string

	// registeredInitSystems is a list of all the registered init system names in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredInitSystems []string

	// systemFn is a map of system names to system functions.
	systemFn map[string]System

	// currentSystem is the name of the system that is currently running.
	currentSystem *string
}

// NewManager creates a new system manager.
func NewManager() *Manager {
	return &Manager{
		registeredSystems: make([]string, 0),
		systemFn:          make(map[string]System),
		currentSystem:     nil,
	}
}

// RegisterSystems registers multiple systems with the system manager.
// There can only be one system with a given name, which is derived from the function name.
// If there is a duplicate system name, an error will be returned and none of the systems will be registered.
func (m *Manager) RegisterSystems(systems ...System) error {
	return m.registerSystems(false, systems...)
}

// RegisterInitSystems registers multiple init systems that is only executed once at tick 0 with the system manager.
// There can only be one system with a given name, which is derived from the function name.
// If there is a duplicate system name, an error will be returned and none of the systems will be registered.
func (m *Manager) RegisterInitSystems(systems ...System) error {
	return m.registerSystems(true, systems...)
}

func (m *Manager) registerSystems(isInit bool, systems ...System) error {
	// Iterate through all the systems and check if they are already registered.
	// This is done before registering any of the systems to ensure that all are registered or none of them are.
	systemNames := make([]string, 0, len(systems))
	for _, sys := range systems {
		// Obtain the name of the system function using reflection.
		systemName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(sys).Pointer()).Name())

		// Check for duplicate system names within the list of systems to be registered
		if slices.Contains(systemNames, systemName) {
			return eris.Errorf("duplicate system %q in slice", systemName)
		}

		// Checks if the system is already previously registered.
		// This will terminate the registration of all systems if any of them are already registered.
		if err := m.isSystemNameUnique(systemName); err != nil {
			return err
		}

		// If the system is not already registered, add it to the list of system names.
		systemNames = append(systemNames, systemName)
	}

	// Iterate through all the systems and register them one by one.
	for i, systemName := range systemNames {
		if isInit {
			m.registeredInitSystems = append(m.registeredInitSystems, systemName)
		} else {
			m.registeredSystems = append(m.registeredSystems, systemName)
		}
		m.systemFn[systemName] = systems[i]
	}

	return nil
}

// RunSystems runs all the registered system in the order that they were registered.
func (m *Manager) RunSystems(wCtx engine.Context) error {
	var systemsToRun []string
	if wCtx.CurrentTick() == 0 {
		//nolint:gocritic // We need to use the append function to concat
		systemsToRun = append(m.registeredInitSystems, m.registeredSystems...)
	} else {
		systemsToRun = m.registeredSystems
	}

	allSystemStartTime := time.Now()
	for _, systemName := range systemsToRun {
		// Explicit memory aliasing
		sysName := systemName
		m.currentSystem = &sysName

		// Inject the system name into the logger
		wCtx.SetLogger(wCtx.Logger().With().Str("system", systemName).Logger())

		// Executes the system function that the user registered
		systemStartTime := time.Now()
		err := m.systemFn[systemName](wCtx)
		if err != nil {
			m.currentSystem = nil
			return eris.Wrapf(err, "system %s generated an error", systemName)
		}

		// Emit the total time it took to run `systemName`
		statsd.EmitTickStat(systemStartTime, systemName)
	}

	// Set the current system to nil to indicate that no system is currently running
	m.currentSystem = nil

	// Emit the total time it took to run all systems
	statsd.EmitTickStat(allSystemStartTime, "all_systems")

	return nil
}

func (m *Manager) GetRegisteredSystemNames() []string {
	return m.registeredSystems
}

func (m *Manager) GetCurrentSystem() string {
	if m.currentSystem == nil {
		return "no_system"
	}
	return *m.currentSystem
}

// isSystemNameUnique checks if the system name already exists in the system map
func (m *Manager) isSystemNameUnique(systemName string) error {
	if _, ok := m.systemFn[systemName]; ok {
		return fmt.Errorf("system %q is already registered", systemName)
	}
	return nil
}
