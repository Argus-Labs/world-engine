package migration

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs/ecstest/fullrelease/component"
)

// DroppedV1ToNothing discards a component the current build no longer declares.
//
// It declares an input and no output, which is the whole point: the entity ends up without the
// component. A migration that produces nothing may look pointless, but without it the restore
// refuses, because the alternative is for the engine to silently discard data on its own. This is
// the developer saying the data is genuinely finished with.
//
// A drop that needs to keep the value somewhere is case 2d, not this: it would declare an Out for
// whichever component takes it over.
type DroppedV1ToNothing struct {
	Old ecs.In[component.DroppedV1]

	// Calls counts entities converted. See [RenamedV1ToCurrent].
	Calls int
}

func (m *DroppedV1ToNothing) Migrate() { m.Calls++ }
