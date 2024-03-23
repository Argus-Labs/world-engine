package cardinal_test

import (
	"encoding/json"
	"errors"
	"testing"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"pkg.world.dev/world-engine/sign"
)

func TestReceiptsQuery(t *testing.T) {
	tf := testutils.NewTestFixture(t, nil)
	world := tf.World
	type fooIn struct{}
	type fooOut struct{ Y int }
	msgName := "foo"
	err := cardinal.RegisterMessage[fooIn, fooOut](world, msgName)
	assert.NilError(t, err)
	wantErrorMessage := "THIS_ERROR_MESSAGE_SHOULD_BE_IN_THE_RECEIPT"
	err = cardinal.RegisterSystems(world, func(ctx cardinal.WorldContext) error {
		return cardinal.EachMessage[fooIn, fooOut](ctx, func(message.TxData[fooIn]) (fooOut, error) {
			if ctx.CurrentTick()%2 == 0 {
				return fooOut{Y: 4}, nil
			}
			return fooOut{}, errors.New(wantErrorMessage)
		})
	})
	assert.NilError(t, err)
	fooMsg, ok := world.GetMessageByFullName("game." + msgName)
	assert.Assert(t, ok)
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
		Errors: []string{wantErrorMessage},
	}
	expectedJSON2, err := json.Marshal(expectedReceipt2)
	assert.NilError(t, err)

	// comparing via json since internally, eris is involved, and makes it a bit harder to compare.
	json1, err := json.Marshal(reply.Receipts[0])
	assert.NilError(t, err)
	json2, err := json.Marshal(reply.Receipts[1])
	assert.NilError(t, err)
	// Make sure the text of the error message actually ends up in the JSON
	assert.Contains(t, string(json2), wantErrorMessage)

	assert.Equal(t, string(expectedJSON1), string(json1))
	assert.Equal(t, string(expectedJSON2), string(json2))
}
