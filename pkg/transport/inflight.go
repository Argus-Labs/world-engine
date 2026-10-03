package transport

import (
	"context"
	"sync"
)

// inflight counts running command handlers so Stop can wait for them. enter and close share mu, so no
// handler starts once close returns, and wait never races a late Add.
type inflight struct {
	mu      sync.Mutex
	closed  bool
	running sync.WaitGroup
}

// enter reports whether a handler may start; when it does, the caller must call exit after it returns.
func (f *inflight) enter() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	f.running.Add(1)
	return true
}

func (f *inflight) exit() {
	f.running.Done()
}

// close makes every later enter fail.
func (f *inflight) close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
}

// wait reports whether every running handler returned before ctx expired.
func (f *inflight) wait(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		f.running.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}
