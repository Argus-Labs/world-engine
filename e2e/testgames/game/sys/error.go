package sys

import (
	"errors"

	"github.com/argus-labs/world-engine/example/tester/game/msg"

	"pkg.world.dev/world-engine/cardinal/world"
)

// Error is a system that will produce an error for any incoming Error messages. It's
// used to test receipt errors.
func Error(ctx world.WorldContext) error {
	return world.EachMessage[msg.ErrorInput](
		ctx, func(m world.Tx[msg.ErrorInput]) (any, error) {
			err := errors.New(m.Msg.ErrorMsg)
			return nil, err
		})
}
