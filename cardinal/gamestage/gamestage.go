package gamestage

import "sync/atomic"

type Stage int32

type Atomic interface {
	CompareAndSwap(old, new Stage) (swapped bool)
	Load() Stage
	Store(val Stage)
	Swap(new Stage) (old Stage)
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

func (a *atomicStage) CompareAndSwap(old, new Stage) (swapped bool) {
	return a.value.CompareAndSwap(old, new)
}

func (a *atomicStage) Load() Stage {
	return a.value.Load().(Stage)
}

func (a *atomicStage) Store(val Stage) {
	a.value.Store(val)
}

func (a *atomicStage) Swap(new Stage) (old Stage) {
	return a.value.Swap(new).(Stage)
}
