package lobby_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/lobby"
)

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		lobby.Register(w, lobby.Config{})
	})
}
