package protoutil_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/protoutil"
	"github.com/argus-labs/world-engine/pkg/cardinal/testutils"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		command   ecs.Command
		checkFunc func(t *testing.T, result *iscv1.CommandBody)
	}{
		{
			name:    "marshal command",
			command: testutils.NewSimpleCommand("test-command", "test-payload"),
			checkFunc: func(t *testing.T, result *iscv1.CommandBody) {
				assert.Equal(t, "test-command", result.GetName())
				assert.NotNil(t, result.GetPayload())
				assert.Equal(t, "test-command", result.GetPayload().GetFields()["name"].GetStringValue())
				assert.Equal(t, "test-payload", result.GetPayload().GetFields()["payload"].GetStringValue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dst := micro.GetAddress("test", micro.RealmWorld, "test", "project", "destination")
			result, err := protoutil.MarshalCommand(tt.command, dst, "test-persona")

			require.NoError(t, err)
			require.NotNil(t, result)
			tt.checkFunc(t, result)
		})
	}
}

func TestMarshalEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		event     ecs.Event
		checkFunc func(t *testing.T, result *iscv1.Event)
	}{
		{
			name:  "marshal event",
			event: testutils.NewSimpleEvent("test-event", "test-payload"),
			checkFunc: func(t *testing.T, result *iscv1.Event) {
				assert.Equal(t, "test-event", result.GetName())
				assert.NotNil(t, result.GetPayload())
				assert.Equal(t, "test-event", result.GetPayload().GetFields()["name"].GetStringValue())
				assert.Equal(t, "test-payload", result.GetPayload().GetFields()["payload"].GetStringValue())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := protoutil.MarshalEvent(tt.event)

			require.NoError(t, err)
			require.NotNil(t, result)
			tt.checkFunc(t, result)
		})
	}
}

func BenchmarkMarshalCommand(b *testing.B) {
	command := testutils.NewSimpleCommand("benchmark-command", "benchmark-payload")
	dst := micro.GetAddress("test", micro.RealmWorld, "test", "project", "destination")

	for i := 0; i < b.N; i++ {
		_, err := protoutil.MarshalCommand(command, dst, "test-persona")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalEvent(b *testing.B) {
	event := testutils.NewSimpleEvent("benchmark-event", "benchmark-payload")

	for i := 0; i < b.N; i++ {
		_, err := protoutil.MarshalEvent(event)
		if err != nil {
			b.Fatal(err)
		}
	}
}
