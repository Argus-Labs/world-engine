package signer

import (
	"context"
	"strconv"
	"sync"

	"github.com/heroiclabs/nakama-common/runtime"
	"github.com/rotisserie/eris"
)

const (
	privateKeyNonce = "private_key_nonce"
)

var _ NonceManager = &nakamaNonceManager{}

type NonceManager interface {
	SetNonce(ctx context.Context, nonce uint64) error
	IncNonce(ctx context.Context) (nonce uint64, err error)
}

type nakamaNonceManager struct {
	sync.Mutex
	nk runtime.NakamaModule
}

func NewNakamaNonceManager(nk runtime.NakamaModule) NonceManager {
	return &nakamaNonceManager{
		nk: nk,
	}
}

func (n *nakamaNonceManager) SetNonce(ctx context.Context, nonce uint64) error {
	return setOnePKStorageObj(ctx, n.nk, privateKeyNonce, strconv.FormatUint(nonce, 10))
}

func (n *nakamaNonceManager) IncNonce(ctx context.Context) (nonce uint64, err error) {
	n.Lock()
	defer n.Unlock()
	nonce, err = getNonce(ctx, n.nk)
	if err != nil {
		return 0, err
	}
	newNonce := nonce + 1
	if err = n.SetNonce(ctx, newNonce); err != nil {
		return 0, err
	}
	return nonce, nil
}

func getNonce(ctx context.Context, nk runtime.NakamaModule) (uint64, error) {
	value, err := getOnePKStorageObj(ctx, nk, privateKeyNonce)
	if err != nil {
		return 0, err
	}
	res, err := strconv.ParseUint(value, 10, 64)
	return res, eris.Wrap(err, "")
}
