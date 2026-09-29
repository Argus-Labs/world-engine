package docker

import "sync"

// State is a machine-readable phase of a long-running docker operation.
type State string

// Progress states.
const (
	StatePulling  State = "pulling"
	StatePulled   State = "pulled"
	StateBuilding State = "building"
	StateBuilt    State = "built"
	StateStarted  State = "started"
	StateStopped  State = "stopped"
)

// Progress reports the state of a long-running operation.
type Progress struct {
	Name    string // resource identifier: container name, image name, etc.
	State   State  // current phase of the operation
	Current int64  // 0–100 percent, only meaningful when State == StatePulling
	Total   int64  // 100, only meaningful when State == StatePulling
	Err     error  // non-nil on failure
}

// synchronized wraps a progress callback with a mutex so that it is safe
// to call from multiple goroutines. Returns nil if fn is nil.
func synchronized(fn func(Progress)) func(Progress) {
	if fn == nil {
		return nil
	}
	var mu sync.Mutex
	return func(p Progress) {
		mu.Lock()
		defer mu.Unlock()
		fn(p)
	}
}

// notify calls fn with p if fn is non-nil. Nil-safe fire-and-forget helper.
func notify(fn func(Progress), p Progress) {
	if fn == nil {
		return
	}
	fn(p)
}
