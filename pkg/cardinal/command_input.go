package cardinal

import (
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
)

// validateCommand checks transport-independent command invariants without mutating queue state.
func (w *World) validateCommand(cmd *iscv1.Command) error {
	if err := w.commands.Validate(cmd); err != nil {
		return err
	}
	if micro.String(w.address) != micro.String(cmd.GetAddress()) {
		return eris.New("address doesn't match shard address")
	}
	return nil
}

// enqueueCommand shares command validation between transports and deterministic tests.
func (w *World) enqueueCommand(cmd *iscv1.Command) error {
	if err := w.validateCommand(cmd); err != nil {
		return err
	}
	return w.commands.Enqueue(cmd)
}
