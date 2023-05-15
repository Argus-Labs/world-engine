package router

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestWithMap(t *testing.T) {
	m := NamespaceClients{}
	m["foo"] = nil

	r := NewRouter(WithNamespaces(m))

	ru, canCast := r.(*router)
	assert.Check(t, canCast == true)

	_, ok := ru.namespaces["foo"]
	assert.Equal(t, ok, true)
}
