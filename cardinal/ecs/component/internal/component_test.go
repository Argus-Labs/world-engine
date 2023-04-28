package internal

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
)

func TestComponentID(t *testing.T) {
	comp := EnergyComponent{}
	anyComp, err := anypb.New(&comp)
	assert.NilError(t, err)
	testCases := []struct {
		name       string
		msg        proto.Message
		expectedID string
	}{
		{
			name:       "regular msg",
			msg:        &EnergyComponent{Amount: 10},
			expectedID: string(comp.ProtoReflect().Descriptor().FullName()),
		},
		{
			name:       "with any",
			msg:        anyComp,
			expectedID: string(comp.ProtoReflect().Descriptor().FullName()),
		},
	}

	for _, tc := range testCases {
		gotID := component.ID(tc.msg)
		assert.Equal(t, gotID, tc.expectedID, "got %s expected %s", gotID, tc.expectedID)
	}
}
