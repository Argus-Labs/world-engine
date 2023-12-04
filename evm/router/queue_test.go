package router

import (
	"github.com/ethereum/go-ethereum/common"
	"gotest.tools/v3/assert"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"testing"
)

func TestQueue(t *testing.T) {
	q := newMsgQueue()
	sender := common.HexToAddress("0xeF68bBDa508adF1FC4589f8620DaD9EDBBFfA0B0")
	assert.Equal(t, q.IsSet(sender), false)
	q.Set(sender, "foo", &routerv1.SendMessageRequest{})
	assert.Equal(t, q.IsSet(sender), true)
	q.Clear()
	assert.Equal(t, q.IsSet(sender), false)
}
