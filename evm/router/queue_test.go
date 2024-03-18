package router

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"gotest.tools/v3/assert"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

func TestQueue(t *testing.T) {
	q := newMsgQueue()
	sender := common.HexToAddress("0xeF68bBDa508adF1FC4589f8620DaD9EDBBFfA0B0")
	assert.Equal(t, q.IsSet(sender), false)
	err := q.Set(sender, "foo", &routerv1.SendMessageRequest{})
	assert.NilError(t, err)
	assert.Equal(t, q.IsSet(sender), true)
	q.Clear()
	assert.Equal(t, q.IsSet(sender), false)
}
