package cardinal_test

import (
	"testing"

	"github.com/franela/goblin"
	"github.com/golang/mock/gomock"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/router/iterator"
	iteratormocks "pkg.world.dev/world-engine/cardinal/router/iterator/mocks"
	"pkg.world.dev/world-engine/cardinal/router/mocks"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

// fooMessage To be used for registration of Messages.
type fooMessage struct {
	Bar string
}

// fooResponse To be used for registration of Messages.
type fooResponse struct {
}

func TestWorldRecovery(t *testing.T) {
	g := goblin.Goblin(t)
	g.Describe("WorldRecovery", func() {
		var tf *testutils.TestFixture
		var controller *gomock.Controller
		var router *mocks.MockRouter
		var world *cardinal.World
		var fooTx types.Message

		// Set CARDINAL_MODE to production so that RecoverFromChain() is called
		setEnvToCardinalProdMode(t)

		g.BeforeEach(func() {
			controller = gomock.NewController(t)
			router = mocks.NewMockRouter(controller)
			tf = testutils.NewTestFixture(t, nil, cardinal.WithCustomRouter(router))

			world = tf.World
			msgName := "foo"
			err := cardinal.RegisterMessage[
				fooMessage,
				fooResponse](
				world,
				msgName,
				message.WithMsgEVMSupport[fooMessage, fooResponse]())
			g.Assert(err).IsNil()
			var ok bool
			fooTx, ok = world.GetMessageByName(msgName)
			g.Assert(ok).IsTrue()
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
						_ ...uint64,
					) error {
						batch := []*iterator.TxBatch{
							{
								Tx:       &sign.Transaction{PersonaTag: "ty"},
								MsgID:    fooTx.ID(),
								MsgValue: fooMessage{Bar: "hello"},
							},
						}

						err := fn(batch, 0, timestamp)
						if err != nil {
							return err
						}

						return nil
					}).AnyTimes()

				// Mock router to return our mock iterator that carries the test recovery data
				router.EXPECT().TransactionIterator().Return(iter).Times(1)
				router.EXPECT().Start().Times(1)
				router.EXPECT().RegisterGameShard(gomock.Any()).Times(1)
				router.EXPECT().
					SubmitTxBlob(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				tf.StartWorld()

				// Check that tick 0 is run with the timestamp from recovery data
				g.Assert(cardinal.NewWorldContext(world).Timestamp()).Equal(timestamp)

				controller.Finish()
			})
		})
	})
}
