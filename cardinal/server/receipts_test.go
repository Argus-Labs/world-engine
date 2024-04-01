package server_test

import (
	"encoding/json"
	"errors"
	"net/http"

	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/sign"
)

func (s *ServerTestSuite) TestReceiptsQuery() {
	s.setupWorld(cardinal.WithDisableSignatureVerification())
	world := s.world
	type fooIn struct{}
	type fooOut struct{ Y int }
	msgName := "foo"
	err := cardinal.RegisterMessage[fooIn, fooOut](world, msgName)
	s.Require().NoError(err)
	wantErrorMessage := "THIS_ERROR_MESSAGE_SHOULD_BE_IN_THE_RECEIPT"
	err = cardinal.RegisterSystems(world, func(ctx cardinal.WorldContext) error {
		return cardinal.EachMessage[fooIn, fooOut](ctx, func(message.TxData[fooIn]) (fooOut, error) {
			if ctx.CurrentTick()%2 == 0 {
				return fooOut{Y: 4}, nil
			}
			return fooOut{}, errors.New(wantErrorMessage)
		})
	})
	s.Require().NoError(err)

	fooMsg, ok := world.GetMessageByFullName("game." + msgName)
	s.Require().True(ok)
	_, txHash1, err := world.AddTransaction(fooMsg.ID(), fooIn{}, &sign.Transaction{PersonaTag: "alpha"})
	s.Require().NoError(err)
	s.fixture.DoTick()
	_, txHash2, err := world.AddTransaction(fooMsg.ID(), fooIn{}, &sign.Transaction{PersonaTag: "beta"})
	s.Require().NoError(err)
	s.fixture.DoTick()

	s.Require().NotEqual(txHash1, txHash2)

	res := s.fixture.Post("query/receipts/list", handler.ListTxReceiptsRequest{})
	s.Require().Equal(res.StatusCode, http.StatusOK)

	var reply handler.ListTxReceiptsResponse
	s.Require().NoError(json.NewDecoder(res.Body).Decode(&reply))

	s.Require().Equal(reply.StartTick, uint64(0))
	s.Require().Equal(reply.EndTick, world.CurrentTick())
	s.Require().Equal(len(reply.Receipts), 2)

	expectedReceipt1 := handler.ReceiptEntry{
		TxHash: string(txHash1),
		Tick:   0,
		Result: fooOut{Y: 4},
		Errors: nil,
	}
	expectedJSON1, err := json.Marshal(expectedReceipt1)
	s.Require().NoError(err)
	expectedReceipt2 := handler.ReceiptEntry{
		TxHash: string(txHash2),
		Tick:   1,
		Result: nil,
		Errors: []string{wantErrorMessage},
	}
	expectedJSON2, err := json.Marshal(expectedReceipt2)
	s.Require().NoError(err)

	// comparing via json since internally, eris is involved, and makes it a bit harder to compare.
	json1, err := json.Marshal(reply.Receipts[0])
	s.Require().NoError(err)
	json2, err := json.Marshal(reply.Receipts[1])
	s.Require().NoError(err)

	// Make sure the text of the error message actually ends up in the JSON
	s.Require().Contains(string(json2), wantErrorMessage)

	s.Require().Equal(string(expectedJSON1), string(json1))
	s.Require().Equal(string(expectedJSON2), string(json2))
}
