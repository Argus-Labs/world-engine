package cardinal

import (
	"fmt"
	"github.com/rotisserie/eris"
	"path/filepath"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/system"
	"reflect"
	"runtime"
	"slices"
	"time"
)

type SystemManager struct {
	// registeredSystems is a list of all the registered system names in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredSystems []string

	// registeredInitSystems is a list of all the registered init system names in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredInitSystems []string

	// systemFn is a map of system names to system functions.
	systemFn map[string]system.System

	// currentSystem is the name of the system that is currently running.
	currentSystem *string
}

// NewSystemManager creates a new system manager.
func NewSystemManager() *SystemManager {
	return &SystemManager{
		registeredSystems: make([]string, 0),
		systemFn:          make(map[string]system.System),
		currentSystem:     nil,
	}
}

// RegisterSystems registers multiple systems with the system manager.
// There can only be one system with a given name, which is derived from the function name.
// If there is a duplicate system name, an error will be returned and none of the systems will be registered.
func (m *SystemManager) RegisterSystems(systems ...system.System) error {
	return m.registerSystems(&m.registeredSystems, systems...)
}

// RegisterInitSystems registers multiple init systems that is only executed once at tick 0 with the system manager.
// There can only be one system with a given name, which is derived from the function name.
// If there is a duplicate system name, an error will be returned and none of the systems will be registered.
func (m *SystemManager) RegisterInitSystems(systems ...system.System) error {
	return m.registerSystems(&m.registeredInitSystems, systems...)
}

func (m *SystemManager) registerSystems(registeredSystems *[]string, systems ...system.System) error {
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
		if err := m.isNotDuplicate(systemName); err != nil {
			return err
		}

		// If the system is not already registered, add it to the list of system names.
		systemNames = append(systemNames, systemName)
	}

	// Iterate through all the systems and register them one by one.
	for i, systemName := range systemNames {
		// The append() function creates a new slice copy, so we can't just pass registeredSystems normally.
		// Therefore, we need to pass a pointer to the slice so the changes are stored in the original slice.
		*registeredSystems = append(*registeredSystems, systemName)
		m.systemFn[systemName] = systems[i]
	}

	return nil
}

// RunSystems runs all the registered system in the order that they were registered.
func (m *SystemManager) RunSystems(eCtx engine.Context) error {
	var systemsToRun []string
	if eCtx.CurrentTick() == 0 {
		//nolint:gocritic,appendAssign // We need to use the append function to concat
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
		eCtx.SetLogger(eCtx.Logger().With().Str("system", systemName).Logger())

		// Executes the system function that the user registered
		systemStartTime := time.Now()
		err := m.systemFn[systemName](eCtx)
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

func (m *SystemManager) IsSystemsRegistered() bool {
	return len(m.registeredSystems) > 0
}

func (m *SystemManager) GetSystemNames() []string {
	return m.registeredSystems
}

func (m *SystemManager) GetCurrentSystem() string {
	if m.currentSystem == nil {
		return "no_system"
	}
	return *m.currentSystem
}

// isNotDuplicate checks if the system name already exists in the system map
func (m *SystemManager) isNotDuplicate(systemName string) error {
	if _, ok := m.systemFn[systemName]; ok {
		return fmt.Errorf("system %q is already registered", systemName)
	}
	return nil
}
