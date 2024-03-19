package worldstage

import (
	"sync/atomic"
)

const (
	Init         Stage = "Init"         // The default stage of world
	Starting     Stage = "Starting"     // World is moved to this stage after StartGame() is called
	Recovering   Stage = "Recovering"   // World is moved to this stage when RecoverFromChain() is called
	Ready        Stage = "Ready"        // World is moved to this stage when it's ready to start ticking
	Running      Stage = "Running"      // World is moved to this stage when Tick() is first called
	ShuttingDown Stage = "ShuttingDown" // World is moved to this stage when it received a shutdown signal
	ShutDown     Stage = "ShutDown"     // World is moved to this stage when it has successfully shutdown
)

var allStages = []Stage{Init, Starting, Recovering, Ready, Running, ShuttingDown, ShutDown}

type Stage string

type Manager struct {
	current *atomic.Value
	// atStage contains a channel for each stage that will be closed when the stage is reached.
	// This will allow goroutines to block until a specified stage has been reached.
	atStage map[Stage]chan struct{}
}

func NewManager() *Manager {
	m := &Manager{
		current: &atomic.Value{},
		atStage: map[Stage]chan struct{}{},
	}
	for _, stage := range allStages {
		m.atStage[stage] = make(chan struct{})
	}
	m.Store(Init)
	return m
}

func (m *Manager) CompareAndSwap(oldStage, newStage Stage) (swapped bool) {
	ok := m.current.CompareAndSwap(oldStage, newStage)
	if ok {
		close(m.atStage[newStage])
	}
	return ok
}

func (m *Manager) Current() Stage {
	return m.current.Load().(Stage)
}

func (m *Manager) Store(val Stage) {
	m.current.Store(val)
	close(m.atStage[val])
}

// NotifyOnStage returns a channel that will be closed when the specified stage has been reached.
func (m *Manager) NotifyOnStage(stage Stage) <-chan struct{} {
	return m.atStage[stage]
}
