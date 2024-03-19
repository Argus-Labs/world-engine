package sys

import (
	"errors"

	"github.com/argus-labs/world-engine/example/tester/game/msg"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
)

// Error is a system that will produce an error for any incoming Error messages. It's
// used to test receipt errors.
func Error(ctx cardinal.WorldContext) error {
	return cardinal.EachMessage[msg.ErrorInput, msg.ErrorOutput](
		ctx, func(m message.TxData[msg.ErrorInput]) (msg.ErrorOutput, error) {
			err := errors.New(m.Msg.ErrorMsg)
			return msg.ErrorOutput{}, err
		})
}
