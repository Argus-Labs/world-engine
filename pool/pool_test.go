package pool

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/bank/types"
	"gotest.tools/assert"
)

func TestPoolSimple(t *testing.T) {
	pool := NewMsgPool(10)
	msgs := []sdk.Msg{
		&types.MsgSend{
			FromAddress: "foo",
			ToAddress:   "bar",
			Amount:      sdk.Coins{sdk.NewInt64Coin("foo", 10)},
		}, &types.MsgSend{
			FromAddress: "bar",
			ToAddress:   "foo",
			Amount:      sdk.Coins{sdk.NewInt64Coin("bar", 10)},
		},
	}
	pool.Send(msgs...)

	drainedMsgs := pool.Drain()
	for i := 0; i < len(msgs); i++ {
		gotMsg := drainedMsgs[i]
		expected := msgs[i]
		assert.Equal(t, gotMsg, expected)
	}
}
