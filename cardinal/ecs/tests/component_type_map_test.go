package tests

import (
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"testing"

	"gotest.tools/v3/assert"
)

type fooBar struct{}

type barFoo struct{}

func TestComponentTypeMap(t *testing.T) {

	fb := storage.NewMockComponentType(fooBar{}, fooBar{})
	bf := storage.NewMockComponentType(barFoo{}, barFoo{})
	ctm := storage.NewComponentTypeMap()

	ctm.Register(fb)
	ctm.Register(bf)

	efb := ctm.ComponentType(fb.ID())
	ebf := ctm.ComponentType(bf.ID())

	assert.Equal(t, fb.Name(), efb.Name())
	assert.Equal(t, bf.Name(), ebf.Name())
}
