package router

import (
	"google.golang.org/protobuf/proto"
	"gotest.tools/v3/assert"
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"testing"
	"time"
)

func TestResultStorage(t *testing.T) {
	t.Parallel()
	rs := NewMemoryResultStorage(1 * time.Second)
	hash := "baz"
	res := &routerv1.SendMessageResponse{
		Errs:      "foo",
		Result:    []byte("bar"),
		EvmTxHash: hash,
		Code:      4,
	}
	rs.SetResult(res)
	gotRes, ok := rs.Result(hash)
	assert.Equal(t, ok, true)
	assert.Check(t, proto.Equal(gotRes, res))
	time.Sleep(1 * time.Second)

	// get the Result again, which will now trigger its expiry and delete it.
	_, ok = rs.Result(hash)
	assert.Equal(t, ok, true)

	// should no longer have the Result
	_, ok = rs.Result(hash)
	assert.Equal(t, ok, false)
}
