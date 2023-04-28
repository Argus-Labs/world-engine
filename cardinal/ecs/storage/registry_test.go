package storage

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gotest.tools/v3/assert"

	testtypes "github.com/argus-labs/world-engine/cardinal/ecs/storage/testtypes/v1"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

func TestAny(t *testing.T) {
	ec := &testtypes.EnergyComponent{
		Amount: 300,
		Cap:    12_000,
	}
	a, err := anypb.New(ec)
	assert.NilError(t, err)
	arch := &types.Archetype{
		ArchetypeIndex: 15,
		EntityIds:      []uint64{1, 2, 3},
		Components:     []*anypb.Any{a},
	}

	bz, err := proto.Marshal(arch)
	assert.NilError(t, err)

	newArch := new(types.Archetype)
	err = proto.Unmarshal(bz, newArch)
	assert.NilError(t, err)

	fmt.Println(newArch.String())
	energy := new(testtypes.EnergyComponent)

	reg := NewTypeRegistry()
	reg.Register(energy)
	dst, err := anypb.UnmarshalNew(a, proto.UnmarshalOptions{
		NoUnkeyedLiterals: struct {
		}{},
		Merge:          false,
		AllowPartial:   false,
		DiscardUnknown: false,
		Resolver:       reg,
		RecursionLimit: 0,
	})
	assert.NilError(t, err)
	actual, ok := dst.(*testtypes.EnergyComponent)
	assert.Equal(t, ok, true)
	fmt.Println("actual: ", actual)
}
