package cardinal_test

import (
	"context"
	"testing"

	. "github.com/franela/goblin"
	"github.com/golang/mock/gomock"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	iteratormocks "pkg.world.dev/world-engine/cardinal/router/iterator/mocks"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
)

// FooMessage To be used for registration of Messages.
type FooMessage struct {
	Bar string
}

// FooResponse To be used for registration of Messages.
type FooResponse struct {
}

func TestWorldRecovery(t *testing.T) {
	g := Goblin(t)
	g.Describe("WorldRecovery", func() {
		var tf *testutils.TestFixture
		var controller *gomock.Controller
		var router *mocks.MockRouter
		var world *cardinal.World
		var fooTx *message.MessageType[FooMessage, FooResponse]

		g.BeforeEach(func() {
			tf = testutils.NewTestFixture(t, nil)

			controller = gomock.NewController(t)
			router = mocks.NewMockRouter(controller)

			world = tf.World
			world.SetRouter(router)
			err := cardinal.RegisterMessage[
				FooMessage,
				FooResponse](
				world,
				"foo",
				message.WithMsgEVMSupport[FooMessage, FooResponse]())
			g.Assert(err).IsNil()
			fooTx, err = cardinal.GetMessageFromWorld[FooMessage, FooResponse](world)
			g.Assert(err).IsNil()
		})

		g.Describe("If there is recovery data", func() {
			g.It("tick 0 should recover with the timestamp from recovery data", func() {
				// This is the timestamp of the tick we will recover
				timestamp := uint64(1577883100)

				// Mock iterator to provide our test recovery data
				iter := iteratormocks.NewMockIterator(controller)
				iter.EXPECT().Each(gomock.Any(), gomock.Any()).DoAndReturn(
					func(
						fn func(batch []*iterator.TxBatch, tick, timestamp uint64) error,
						ranges ...uint64,
					) error {
						batch := []*iterator.TxBatch{
							{
								Tx:       &sign.Transaction{PersonaTag: "ty"},
								MsgID:    fooTx.ID(),
								MsgValue: FooMessage{Bar: "hello"},
							},
						}

						err := fn(batch, 0, timestamp)
						if err != nil {
							return err
						}

						return nil
					}).AnyTimes()

				// Mock router to return our mock iterator that carries the test recovery data
				router.EXPECT().TransactionIterator().Return(iter).AnyTimes()
				router.EXPECT().Start().AnyTimes()
				router.EXPECT().
					SubmitTxBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				tf.StartWorld()

				err := world.RecoverFromChain(context.Background())
				g.Assert(err).IsNil()

				// Check that tick 0 is run with the timestamp from recovery data
				g.Assert(cardinal.NewWorldContext(world).Timestamp()).Equal(timestamp)

				controller.Finish()
			})
		})
	})
}
