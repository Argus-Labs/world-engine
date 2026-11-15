package cardinal

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_CanceledCommandsAreNotEnqueued(t *testing.T) {
	t.Parallel()

	for _, method := range []string{"SendCommand", "SendCommandWithReply"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()

			for _, expired := range []bool{false, true} {
				name := "canceled"
				if expired {
					name = "expired deadline"
				}

				t.Run(name, func(t *testing.T) {
					t.Parallel()

					fixture := newServiceFixture(t, testutils.NewRand(t), false)
					payload := testutils.SimpleCommand{Value: 42}
					cmd := &iscv1.Command{
						Name: payload.Name(), Address: fixture.world.address, Payload: payload.MarshalWire(),
					}

					var (
						ctx    context.Context
						cancel context.CancelFunc
					)

					if expired {
						ctx, cancel = context.WithDeadline(serviceTestContext("alice"), time.Unix(1, 0))
						require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
					} else {
						ctx, cancel = context.WithCancel(serviceTestContext("alice"))
						cancel()
						require.ErrorIs(t, ctx.Err(), context.Canceled)
					}

					defer cancel()

					var err error
					if method == "SendCommand" {
						_, err = fixture.svc.SendCommand(
							ctx,
							connect.NewRequest(&cardinalv1.SendCommandRequest{Command: cmd}),
						)
					} else {
						_, err = fixture.svc.SendCommandWithReply(
							ctx,
							connect.NewRequest(
								&cardinalv1.SendCommandWithReplyRequest{Command: cmd, EventName: "result"},
							),
						)
					}

					require.Error(t, err)
					assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))

					fixture.world.commands.Drain()
					commands, err := fixture.world.commands.Get(fixture.commandID)
					require.NoError(t, err)
					assert.Empty(t, commands, "a request canceled before handler entry must not reach game systems")
				})
			}
		})
	}
}
