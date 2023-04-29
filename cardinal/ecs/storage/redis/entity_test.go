package redis

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestNewEntity(t *testing.T) {
	store := getTestStorage(t)
	ent, err := store.NewEntity()
	assert.NilError(t, err)

	ent2, err := store.NewEntity()
	assert.NilError(t, err)

	assert.Equal(t, ent+1, ent2)
}
