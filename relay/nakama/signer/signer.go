package signer

// signer.go manages the creation and loading of the Nakama private key used to sign
// all transactions.

import (
	"context"

	"pkg.world.dev/world-engine/sign"
)

const (
	AdminAccountID = "00000000-0000-0000-0000-000000000000"
)

type Signer interface {
	SignTx(ctx context.Context, personaTag string, namespace string, data any) (*sign.Transaction, error)
	SignSystemTx(ctx context.Context, namespace string, data any) (*sign.Transaction, error)
	SignerAddress() string
}
