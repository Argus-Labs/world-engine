package message

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
)

func TestValidateIsStruct(t *testing.T) {
	type NotStruct []int
	type AStruct struct{}
	assert.True(t, isStruct[AStruct]())
	assert.True(t, isStruct[*AStruct]())
	assert.False(t, isStruct[NotStruct]())
}
