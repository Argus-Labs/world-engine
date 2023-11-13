package router

import (
	"gotest.tools/v3/assert"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"testing"
)

func TestQueue(t *testing.T) {
	q := &msgQueue{}
	assert.Equal(t, q.IsSet(), false)
	q.Set("foo", &routerv1.SendMessageRequest{})
	assert.Equal(t, q.IsSet(), true)
	q.Clear()
	assert.Equal(t, q.IsSet(), false)
}
