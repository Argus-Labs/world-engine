package cardinal_test

import (
	"bytes"
	"pkg.world.dev/world-engine/cardinal/testutils"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal"
	"pkg.world.dev/world-engine/cardinal/ecs/log"
)

const nameOfSystem = "SystemLogMyName"

func SystemLogMyName(worldCtx cardinal.WorldContext) error {
	worldCtx.Logger().Log().Msg("wanted system name: " + nameOfSystem)
	return nil
}

func TestSystemNamesAreCorrectInLogs(t *testing.T) {
	world, doTick := testutils.MakeWorldAndTicker(t)
	ecsWorld := cardinal.TestingWorldContextToECSWorld(cardinal.TestingWorldToWorldContext(world))
	var buf bytes.Buffer
	bufLogger := zerolog.New(&buf)
	cardinalLogger := log.Logger{
		&bufLogger,
	}
	ecsWorld.InjectLogger(&cardinalLogger)
	cardinal.RegisterSystems(world, SystemLogMyName)

	doTick()

	logLines := strings.Split(buf.String(), "\n")
	found := false
	for _, logLine := range logLines {
		// The one-and-only system emits a log line with "SystemLogMyName" in it. The log should
		// also automatically include the system name by default, so there should be at least 1 log line that has
		// the "SystemLogMyName" string twice.
		count := strings.Count(logLine, nameOfSystem)
		if count != 2 {
			continue
		}
		found = true
		break
	}
	assert.Check(t, found, "unable to find log line with %q twice", nameOfSystem)
}
