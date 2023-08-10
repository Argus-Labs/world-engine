package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/ecs/inmem"
	"gotest.tools/v3/assert"
	"testing"
)

func TestSetIDWithOption(t *testing.T) {
	id := "foo"
	w := inmem.NewECSWorldForTest(t, ecs.WithWorldID(id))
	assert.Equal(t, w.ID(), ecs.WorldId(id))
}
