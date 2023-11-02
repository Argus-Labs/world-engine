package cardinal

import (
	"context"
	"testing"

	"pkg.world.dev/world-engine/chain/x/shard/types"
	"pkg.world.dev/world-engine/sign"
)

type DummyAdapter struct{}

func (d *DummyAdapter) Submit(_ context.Context, p *sign.SignedPayload, txID, tick uint64) error {
	return nil
}

func (d *DummyAdapter) QueryTransactions(_ context.Context, request *types.QueryTransactionsRequest,
) (*types.QueryTransactionsResponse, error) {
	return nil, nil
}

func TestOptionFunctionSignatures(t *testing.T) {
	//This test is designed to keep API compatability. If a compile error happens here it means a function signature to
	//public facing functions was changed.
	WithAdapter(&DummyAdapter{})
	WithReceiptHistorySize(1)
	WithNamespace("blah")
	WithPort("4040")
	WithDisableSignatureVerification()
	WithPrettyLog()
}
