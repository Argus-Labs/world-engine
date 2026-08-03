package systemevent_test

import (
	"testing"

	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlayerDeathWireRoundTrip(t *testing.T) {
	original := systemevent.PlayerDeath{Nickname: "player-one"}

	data, err := original.MarshalWire()
	require.NoError(t, err)

	decoded, err := (systemevent.PlayerDeath{}).UnmarshalWire(data)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}
