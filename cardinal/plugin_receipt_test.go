package cardinal_test

import (
	"encoding/json"
	"errors"
	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
	"testing"
)

func TestReceiptsQuery(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	type fooIn struct{}
	type fooOut struct{ Y int }
	fooMsg := message.NewMessageType[fooIn, fooOut]("foo")
	err := cardinal.RegisterMessages(world, fooMsg)
	assert.NilError(t, err)
	err = cardinal.RegisterSystems(world, func(ctx cardinal.WorldContext) error {
		fooMsg.Each(ctx, func(t message.TxData[fooIn]) (fooOut, error) {
			if ctx.CurrentTick()%2 == 0 {
				return fooOut{Y: 4}, nil
			}

			return fooOut{}, errors.New("omg")
		})
		return nil
	})
	assert.NilError(t, err)

	_, txHash1 := world.AddTransaction(fooMsg.ID(), fooIn{}, &sign.Transaction{PersonaTag: "ty"})
	tf.DoTick()
	_, txHash2 := world.AddTransaction(fooMsg.ID(), fooIn{}, &sign.Transaction{PersonaTag: "ty"})
	tf.DoTick()

	qry, err := world.GetQueryByName("list")
	assert.NilError(t, err)

	res, err := qry.HandleQuery(cardinal.NewReadOnlyWorldContext(world), &cardinal.ListTxReceiptsRequest{})
	assert.NilError(t, err)
	reply, ok := res.(*cardinal.ListTxReceiptsResponse)
	assert.True(t, ok)

	assert.Equal(t, reply.StartTick, uint64(0))
	assert.Equal(t, reply.EndTick, world.CurrentTick())
	assert.Len(t, reply.Receipts, 2)

	expectedReceipt1 := cardinal.ReceiptEntry{
		TxHash: string(txHash1),
		Tick:   0,
		Result: fooOut{Y: 4},
		Errors: nil,
	}
	expectedJSON1, err := json.Marshal(expectedReceipt1)
	assert.NilError(t, err)
	expectedReceipt2 := cardinal.ReceiptEntry{
		TxHash: string(txHash2),
		Tick:   1,
		Result: nil,
		Errors: []error{errors.New("omg")},
	}
	expectedJSON2, err := json.Marshal(expectedReceipt2)
	assert.NilError(t, err)

	// comparing via json since internally, eris is involved, and makes it a bit harder to compare.
	json1, err := json.Marshal(reply.Receipts[0])
	assert.NilError(t, err)
	json2, err := json.Marshal(reply.Receipts[1])
	assert.NilError(t, err)

	assert.Equal(t, string(expectedJSON1), string(json1))
	assert.Equal(t, string(expectedJSON2), string(json2))
}
