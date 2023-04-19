package storage

import (
	"testing"

	"gotest.tools/v3/assert"
)

type fooBar struct{}

type barFoo struct{}

func TestComponentTypeMap(t *testing.T) {

	fb := NewMockComponentType(fooBar{}, fooBar{})
	bf := NewMockComponentType(barFoo{}, barFoo{})
	ctm := NewComponentTypeMap()

	ctm.Register(fb)
	ctm.Register(bf)

	efb := ctm.ComponentType(fb.ID())
	ebf := ctm.ComponentType(bf.ID())

	assert.Equal(t, fb.Name(), efb.Name())
	assert.Equal(t, bf.Name(), ebf.Name())
}
