package systems

import (
	"errors"
	"fmt"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"path/filepath"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"reflect"
	"runtime"
	"time"
)

type System func(ctx engine.Context) error

type Manager struct {
	// registeredSystems is a list of all the registered system names in the order that they were registered.
	// This is represented as a list as maps in Go are unordered.
	registeredSystems []string

	// systemFn is a map of system names to system functions.
	systemFn map[string]System

	// currentSystem is the name of the system that is currently running.
	currentSystem *string

	initSystem      System
	isInitSystemRan bool
}

// New creates a new system manager.
func New() *Manager {
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
	systemNames := make([]string, 0, len(systems))

	// Iterate through all the systems and check if they are already registered.
	// This is done before registering any of the systems to ensure that all systems are registered or none of them are.
	for _, system := range systems {
		// Obtain the name of the system function using reflection.
		systemName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name())

		// Checks if the system is already previously registered.
		// This will terminate the registration of all systems if any of them are already registered.
		if err := m.checkDuplicateSystemName(systemName); err != nil {
			return eris.Wrap(err, "failed to register system")
		}

		// If the system is not already registered, add it to the list of system names.
		systemNames = append(systemNames, systemName)
	}

	// Iterate through all the systems and register them one by one.
	for i, systemName := range systemNames {
		m.registeredSystems = append(m.registeredSystems, systemName)
		m.systemFn[systemName] = systems[i]
	}

	return nil
}

// RegisterInitSystem registers an init system with the system manager.
// The init system can only be run once.
func (m *Manager) RegisterInitSystem(system System) {
	m.initSystem = system
}

// RunSystems runs all the registered system in the order that they were registered.
func (m *Manager) RunSystems(eCtx engine.Context) error {
	allSystemStartTime := time.Now()

	for _, systemName := range m.registeredSystems {
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

// RunInitSystem runs the init system.
// The init system can only be run once.
func (m *Manager) RunInitSystem(eCtx engine.Context) error {
	systemName := "InitSystem"
	m.currentSystem = &systemName

	// Check if the init system has already been run
	if m.isInitSystemRan {
		return errors.New("init system already ran")
	}

	// If init system is not set, no need to do anything
	if m.initSystem == nil {
		return nil
	}

	// Inject the system name into the logger
	eCtx.SetLogger(eCtx.Logger().With().Str("system", "InitSystem").Logger())

	// Run the init system
	err := m.initSystem(eCtx)
	if err != nil {
		return eris.Wrap(err, "init system generated an error")
	}

	m.currentSystem = nil
	return nil
}

func (m *Manager) IsSystemsRegistered() bool {
	return len(m.registeredSystems) > 0
}

func (m *Manager) GetSystemNames() []string {
	return m.registeredSystems
}

func (m *Manager) GetCurrentSystem() string {
	if m.currentSystem == nil {
		return "no_system"
	}
	return *m.currentSystem
}

func (m *Manager) LogSystemsInfo(logEvent *zerolog.Event) *zerolog.Event {
	// Add the total number of systems to the log
	logEvent.Int("total_systems", len(m.registeredSystems))

	// Iterate through all the systems and add them to the log as an array
	logArray := zerolog.Arr()
	for _, systemName := range m.registeredSystems {
		logArray.Str(systemName)
	}

	// Return the log with the array of systems
	return logEvent.Array("systems", logArray)
}

// checkDuplicateSystemName checks if the system name already exists in the system map
func (m *Manager) checkDuplicateSystemName(systemName string) error {
	if _, ok := m.systemFn[systemName]; ok {
		return fmt.Errorf("system with name %s already exists", systemName)
	}
	return nil
}
