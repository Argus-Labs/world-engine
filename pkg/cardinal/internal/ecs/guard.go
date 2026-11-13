package ecs

import "fmt"

// ReadOnly makes every write to the world panic until it is called again with "", and returns the
// holder it replaces so the caller can put that one back. holder names the code now running, for
// the panic. Cardinal holds it around code that may read the world and must not change it. The
// check runs before the world's lock is taken, so a refused write never deadlocks.
func (w *World) ReadOnly(holder string) string {
	previous := w.state.readOnly
	w.state.readOnly = holder
	return previous
}

// refuseWrite panics because op would change a world that is read-only.
func (ws *worldState) refuseWrite(op string) {
	panic(fmt.Sprintf("cardinal: %s must not change the world, and it called %s", ws.readOnly, op))
}
