package gamestage

import "sync/atomic"

type Stage int32

type Atomic interface {
	CompareAndSwap(oldStage, newStage Stage) (swapped bool)
	Load() Stage
	Store(val Stage)
	Swap(newStage Stage) (oldStage Stage)
}

const (
	StagePreStart Stage = iota
	StageStarting
	StageRunning
	StageShuttingDown
	StageShutDown
)

type atomicStage struct {
	value *atomic.Value
}

func NewAtomic() Atomic {
	a := &atomicStage{
		value: &atomic.Value{},
	}
	a.Store(StagePreStart)
	return a
}

func (a *atomicStage) CompareAndSwap(oldStage, newStage Stage) (swapped bool) {
	return a.value.CompareAndSwap(oldStage, newStage)
}

func (a *atomicStage) Load() Stage {
	return a.value.Load().(Stage)
}

func (a *atomicStage) Store(val Stage) {
	a.value.Store(val)
}

func (a *atomicStage) Swap(newStage Stage) (oldStage Stage) {
	return a.value.Swap(newStage).(Stage)
}
