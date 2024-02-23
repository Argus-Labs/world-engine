package worldstage

import (
	"sync/atomic"
)

type Stage string

const (
	Init         Stage = "Init"         // The default stage of world
	Starting     Stage = "Starting"     // World is moved to this stage after StartGame() is called
	Recovering   Stage = "Recovering"   // World is moved to this stage when RecoverFromChain() is called
	Ready        Stage = "Ready"        // World is moved to this stage when it's ready to start ticking
	Running      Stage = "Running"      // World is moved to this stage when Tick() is first called
	ShuttingDown Stage = "ShuttingDown" // World is moved to this stage when it received a shutdown signal
	ShutDown     Stage = "ShutDown"     // World is moved to this stage when it has successfully shutdown
)

type Manager struct {
	current *atomic.Value
}

func NewManager() *Manager {
	m := &Manager{
		current: &atomic.Value{},
	}
	m.Store(Init)
	return m
}

func (m *Manager) CompareAndSwap(oldStage, newStage Stage) (swapped bool) {
	return m.current.CompareAndSwap(oldStage, newStage)
}

func (m *Manager) Current() Stage {
	return m.current.Load().(Stage)
}

func (m *Manager) Store(val Stage) {
	m.current.Store(val)
}

func (m *Manager) Swap(newStage Stage) (oldStage Stage) {
	return m.current.Swap(newStage).(Stage)
}
